package handler_test

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-pdf/fpdf"

	"github.com/brendanwhit/tax-withholding-estimator/internal/db"
	"github.com/brendanwhit/tax-withholding-estimator/internal/handler"
)

func setupTestServer(t *testing.T) (*handler.Server, *http.ServeMux) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Find template directory relative to this test file.
	tmplDir := findTemplateDir(t)

	srv, err := handler.NewServer(store, tmplDir)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	return srv, mux
}

func findTemplateDir(t *testing.T) string {
	t.Helper()
	// Walk up from test location to find templates directory.
	candidates := []string{
		"../../templates",
		"../../../templates",
		filepath.Join(os.Getenv("PWD"), "templates"),
	}
	for _, c := range candidates {
		abs, _ := filepath.Abs(c)
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}
	t.Fatal("cannot find templates directory")
	return ""
}

func TestDashboardReturns200(t *testing.T) {
	_, mux := setupTestServer(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET / = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Tax Withholding Estimator") {
		t.Error("dashboard should contain title")
	}
}

func TestUploadPageReturns200(t *testing.T) {
	_, mux := setupTestServer(t)

	req := httptest.NewRequest("GET", "/upload", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /upload = %d, want 200", w.Code)
	}
}

func TestUploadRejectsNonPDF(t *testing.T) {
	_, mux := setupTestServer(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("paystub", "data.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("not a pdf"))
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /upload with .txt = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "PDF") {
		t.Error("error should mention PDF")
	}
}

func TestUploadAcceptsPDFAndReturnsData(t *testing.T) {
	_, mux := setupTestServer(t)

	pdfPath := createTestPaystubPDF(t)

	pdfFile, err := os.Open(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pdfFile.Close() }()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("paystub", "paystub.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(part, pdfFile); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST /upload with PDF = %d, want 200. Body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "successfully") {
		t.Errorf("response should contain success message, got: %s", w.Body.String())
	}
}

func TestFilingStatusCanBeSetAndPersisted(t *testing.T) {
	srv, mux := setupTestServer(t)

	form := url.Values{}
	form.Set("year", "2025")
	form.Set("filing_status", "single")

	req := httptest.NewRequest("POST", "/filing-status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther && w.Code != http.StatusOK {
		t.Errorf("POST /filing-status = %d, want 303 or 200", w.Code)
	}

	// Verify it was persisted.
	status, err := srv.Store.GetFilingStatus(2025)
	if err != nil {
		t.Fatalf("GetFilingStatus: %v", err)
	}
	if status != "single" {
		t.Errorf("persisted status = %q, want %q", status, "single")
	}
}

func TestBracketsPageReturns200(t *testing.T) {
	_, mux := setupTestServer(t)

	req := httptest.NewRequest("GET", "/brackets?year=2025&status=single", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /brackets = %d, want 200", w.Code)
	}

	body := w.Body.String()
	// The template uses formatPercent which outputs "10.0%"
	if !strings.Contains(body, "10.0") {
		// Dump last 2000 chars to see what rendered.
		tail := body
		if len(tail) > 2000 {
			tail = tail[len(tail)-2000:]
		}
		t.Errorf("brackets page should show 10.0%% bracket rate.\nTail of body:\n%s", tail)
	}
}

func TestBracketsCachedInSQLite(t *testing.T) {
	srv, mux := setupTestServer(t)

	req := httptest.NewRequest("GET", "/brackets?year=2025&status=single", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /brackets = %d", w.Code)
	}

	// Verify brackets were cached by loading from DB.
	var count int
	err := srv.Store.DB.QueryRow("SELECT COUNT(*) FROM tax_brackets WHERE tax_year = 2025 AND filing_status = 'single'").Scan(&count)
	if err != nil {
		t.Fatalf("query brackets: %v", err)
	}
	if count == 0 {
		t.Error("expected brackets to be cached in SQLite")
	}
}

func createTestPaystubPDF(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "paystub.pdf")

	content := fmt.Sprintf(`EARNINGS STATEMENT
Employee: TestUser Smith
Pay Period: 01/01/%d - 01/15/%d

Gross Pay: $5,384.62
Federal Income Tax: $807.69

YTD Gross: $5,384.62
YTD Federal Tax: $807.69
`, currentYear(), currentYear())

	doc := fpdf.New("P", "mm", "Letter", "")
	doc.AddPage()
	doc.SetFont("Courier", "", 10)

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		doc.Cell(0, 5, line)
		doc.Ln(5)
	}

	if err := doc.OutputFileAndClose(path); err != nil {
		t.Fatalf("create test PDF: %v", err)
	}
	return path
}

func currentYear() int {
	return 2025 // use fixed year for reproducible tests
}
