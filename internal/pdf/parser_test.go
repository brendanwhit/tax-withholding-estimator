package pdf_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-pdf/fpdf"

	pdfparse "github.com/brendanwhit/tax-withholding-estimator/internal/pdf"
)

// createTestPDF generates a simple text-based PDF with paystub content.
func createTestPDF(t *testing.T, content string) (string, int64) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "paystub.pdf")

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

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat test PDF: %v", err)
	}

	return path, info.Size()
}

func TestParsePaystubBasic(t *testing.T) {
	content := `EARNINGS STATEMENT
Employee: John Smith
SSN: 123-45-6789
Pay Period: 01/01/2025 - 01/15/2025

Gross Pay: $5,384.62
Federal Income Tax: $807.69
State Tax: $269.23

YTD Gross: $5,384.62
YTD Federal Tax: $807.69
`
	path, size := createTestPDF(t, content)

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()

	data, err := pdfparse.ParsePaystub(f, size)
	if err != nil {
		t.Fatalf("ParsePaystub: %v", err)
	}

	if data.FirstName != "John" {
		t.Errorf("FirstName = %q, want %q", data.FirstName, "John")
	}
	if data.GrossPay != 5384.62 {
		t.Errorf("GrossPay = %v, want 5384.62", data.GrossPay)
	}
	if data.FederalTaxWithheld != 807.69 {
		t.Errorf("FederalTaxWithheld = %v, want 807.69", data.FederalTaxWithheld)
	}
	if data.PayPeriodStart.Format("2006-01-02") != "2025-01-01" {
		t.Errorf("PayPeriodStart = %v, want 2025-01-01", data.PayPeriodStart)
	}
	if data.PayPeriodEnd.Format("2006-01-02") != "2025-01-15" {
		t.Errorf("PayPeriodEnd = %v, want 2025-01-15", data.PayPeriodEnd)
	}
	if data.YTDGrossPay != 5384.62 {
		t.Errorf("YTDGrossPay = %v, want 5384.62", data.YTDGrossPay)
	}
	if data.YTDFederalTaxWithheld != 807.69 {
		t.Errorf("YTDFederalTaxWithheld = %v, want 807.69", data.YTDFederalTaxWithheld)
	}
}

func TestParsePaystubStripsSensitiveData(t *testing.T) {
	text := `SSN: 123-45-6789
EIN: 12-3456789
Account: 1234567890
Routing: 021000021
Employee: Jane Doe`

	sanitized := pdfparse.StripSensitiveData(text)

	if strings.Contains(sanitized, "123-45-6789") {
		t.Error("SSN not stripped")
	}
	if strings.Contains(sanitized, "12-3456789") {
		t.Error("EIN not stripped")
	}
	if strings.Contains(sanitized, "1234567890") {
		t.Error("Account number not stripped")
	}
	// First name should still be present.
	if !strings.Contains(sanitized, "Jane") {
		t.Error("first name was stripped — should be retained")
	}
}

func TestParsePaystubRejectsNonPDF(t *testing.T) {
	// A plain text file is not a valid PDF.
	content := []byte("This is not a PDF file")
	r := bytes.NewReader(content)

	_, err := pdfparse.ParsePaystub(r, int64(len(content)))
	if err == nil {
		t.Error("expected error for non-PDF file")
	}

	var parseErr *pdfparse.ParseError
	if !isParseError(err, &parseErr) {
		t.Errorf("expected ParseError, got %T: %v", err, err)
	}
}

func TestParsePaystubPayPeriodDates(t *testing.T) {
	content := `Employee: Alice Johnson
Pay Period: 03/01/2025 - 03/15/2025
Gross Pay: $4,000.00
Federal Tax: $600.00
`
	path, size := createTestPDF(t, content)

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()

	data, err := pdfparse.ParsePaystub(f, size)
	if err != nil {
		t.Fatalf("ParsePaystub: %v", err)
	}

	if data.PayPeriodStart.Month() != 3 || data.PayPeriodStart.Day() != 1 {
		t.Errorf("unexpected start: %v", data.PayPeriodStart)
	}
	if data.PayPeriodEnd.Month() != 3 || data.PayPeriodEnd.Day() != 15 {
		t.Errorf("unexpected end: %v", data.PayPeriodEnd)
	}
}

func TestParsePaystubYTDTotals(t *testing.T) {
	content := `Employee: Bob Williams
Pay Period: 06/01/2025 - 06/15/2025
Gross Pay: $5,000.00
Federal Income Tax: $750.00
YTD Gross: $60,000.00
YTD Federal Tax: $9,000.00
`
	path, size := createTestPDF(t, content)

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()

	data, err := pdfparse.ParsePaystub(f, size)
	if err != nil {
		t.Fatalf("ParsePaystub: %v", err)
	}

	if data.YTDGrossPay != 60000.00 {
		t.Errorf("YTDGrossPay = %v, want 60000.00", data.YTDGrossPay)
	}
	if data.YTDFederalTaxWithheld != 9000.00 {
		t.Errorf("YTDFederalTaxWithheld = %v, want 9000.00", data.YTDFederalTaxWithheld)
	}
}

func TestStripSensitiveDataOnlyFirstNameRetained(t *testing.T) {
	text := `Employee Name: Sarah Connor
Address: 123 Main St, Los Angeles, CA 90001
SSN: 987-65-4321
Account #12345678901234`

	sanitized := pdfparse.StripSensitiveData(text)

	if strings.Contains(sanitized, "987-65-4321") {
		t.Error("SSN not stripped")
	}
	if !strings.Contains(sanitized, "Sarah") {
		t.Error("first name should be retained")
	}
}

// isParseError checks if the error is a ParseError.
func isParseError(err error, target **pdfparse.ParseError) bool {
	if pe, ok := err.(*pdfparse.ParseError); ok {
		*target = pe
		return true
	}
	return false
}
