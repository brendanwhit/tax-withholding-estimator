package handler

import (
	"fmt"
	"html/template"
	"io"
	"log"
"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/brendanwhit/tax-withholding-estimator/internal/calc"
	"github.com/brendanwhit/tax-withholding-estimator/internal/db"
	pdfparse "github.com/brendanwhit/tax-withholding-estimator/internal/pdf"
	"github.com/brendanwhit/tax-withholding-estimator/internal/tax"
)

// Server holds dependencies for HTTP handlers.
type Server struct {
	Store    *db.Store
	TaxStore *tax.Store
	Tmpl     *template.Template
}

// NewServer creates a new Server with parsed templates.
func NewServer(store *db.Store, tmplDir string) (*Server, error) {
	taxStore := tax.NewStore(store.DB)

	funcMap := template.FuncMap{
		"formatMoney": func(f float64) string {
			neg := ""
			if f < 0 {
				neg = "-"
				f = -f
			}
			return fmt.Sprintf("%s$%s", neg, formatWithCommas(f))
		},
		"formatPercent": func(f float64) string {
			return fmt.Sprintf("%.1f%%", f*100)
		},
		"formatStatus": func(s string) string {
			switch s {
			case "single":
				return "Single"
			case "married_filing_jointly":
				return "Married Filing Jointly"
			case "married_filing_separately":
				return "Married Filing Separately"
			case "head_of_household":
				return "Head of Household"
			default:
				return s
			}
		},
	}

	pattern := filepath.Join(tmplDir, "*.html")
	tmpl, err := template.New("").Funcs(funcMap).ParseGlob(pattern)
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	return &Server{
		Store:    store,
		TaxStore: taxStore,
		Tmpl:     tmpl,
	}, nil
}

func formatWithCommas(f float64) string {
	s := fmt.Sprintf("%.2f", f)
	parts := strings.Split(s, ".")
	intPart := parts[0]
	decPart := parts[1]

	var result []byte
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result) + "." + decPart
}

// RegisterRoutes sets up all HTTP routes.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/upload", s.handleUpload)
	mux.HandleFunc("/filing-status", s.handleFilingStatus)
	mux.HandleFunc("/brackets", s.handleBrackets)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	year := currentTaxYear()

	filingStatus, err := s.Store.GetFilingStatus(year)
	if err != nil {
		http.Error(w, "failed to get filing status", http.StatusInternalServerError)
		return
	}
	if filingStatus == "" {
		filingStatus = string(tax.MarriedFilingJointly)
	}

	paystubs, err := s.Store.GetAllPaystubsByYear(year)
	if err != nil {
		http.Error(w, "failed to get paystubs", http.StatusInternalServerError)
		return
	}

	schedule, err := s.TaxStore.GetBrackets(year, tax.FilingStatus(filingStatus))
	if err != nil {
		http.Error(w, "failed to get tax brackets", http.StatusInternalServerError)
		return
	}

	earners := buildEarnerSummaries(paystubs)

	result := calc.CalculateWithholding(schedule, earners, 0, 26, time.Now())

	data := map[string]interface{}{
		"Year":          year,
		"FilingStatus":  filingStatus,
		"Result":        result,
		"Paystubs":      paystubs,
		"HasPaystubs":   len(paystubs) > 0,
		"AllStatuses":   tax.AllFilingStatuses(),
	}

	if err := s.Tmpl.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if err := s.Tmpl.ExecuteTemplate(w, "upload.html", nil); err != nil {
			log.Printf("template error: %v", err)
		}
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		renderUploadError(w, s.Tmpl, "Failed to parse form: "+err.Error())
		return
	}

	file, header, err := r.FormFile("paystub")
	if err != nil {
		renderUploadError(w, s.Tmpl, "No file uploaded")
		return
	}
	defer func() { _ = file.Close() }()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".pdf") {
		renderUploadError(w, s.Tmpl, "Only PDF files are accepted. Please upload a PDF paystub.")
		return
	}

	tmpDir := os.TempDir()
	tmpFile, err := os.CreateTemp(tmpDir, "paystub-*.pdf")
	if err != nil {
		renderUploadError(w, s.Tmpl, "Failed to create temp file")
		return
	}
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	}()

	size, err := io.Copy(tmpFile, file)
	if err != nil {
		renderUploadError(w, s.Tmpl, "Failed to save uploaded file")
		return
	}

	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		renderUploadError(w, s.Tmpl, "Failed to read uploaded file")
		return
	}

	data, err := pdfparse.ParsePaystub(tmpFile, size)
	if err != nil {
		renderUploadError(w, s.Tmpl, "Failed to parse paystub: "+err.Error())
		return
	}

	year := data.PayPeriodStart.Year()
	paystub := &db.Paystub{
		PersonName:         data.FirstName,
		TaxYear:            year,
		PayPeriodStart:     data.PayPeriodStart,
		PayPeriodEnd:       data.PayPeriodEnd,
		GrossPay:           data.GrossPay,
		FederalTaxWithheld: data.FederalTaxWithheld,
	}
	if data.YTDGrossPay > 0 {
		paystub.YTDGrossPay = &data.YTDGrossPay
	}
	if data.YTDFederalTaxWithheld > 0 {
		paystub.YTDFederalTaxWithheld = &data.YTDFederalTaxWithheld
	}

	if _, err := s.Store.SavePaystub(paystub); err != nil {
		renderUploadError(w, s.Tmpl, "Failed to save paystub: "+err.Error())
		return
	}

	tmplData := map[string]interface{}{
		"Success": true,
		"Data":    data,
		"Year":    year,
	}

	if isHTMX(r) {
		if err := s.Tmpl.ExecuteTemplate(w, "upload-result.html", tmplData); err != nil {
			log.Printf("template error: %v", err)
		}
		return
	}
	if err := s.Tmpl.ExecuteTemplate(w, "upload.html", tmplData); err != nil {
		log.Printf("template error: %v", err)
	}
}

func (s *Server) handleFilingStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	yearStr := r.FormValue("year")
	status := r.FormValue("filing_status")

	year, err := strconv.Atoi(yearStr)
	if err != nil {
		year = currentTaxYear()
	}

	if err := s.Store.SaveFilingStatus(year, status); err != nil {
		http.Error(w, "failed to save filing status", http.StatusInternalServerError)
		return
	}

	if isHTMX(r) {
		w.Header().Set("HX-Redirect", "/")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleBrackets(w http.ResponseWriter, r *http.Request) {
	year := currentTaxYear()
	if y := r.URL.Query().Get("year"); y != "" {
		if parsed, err := strconv.Atoi(y); err == nil {
			year = parsed
		}
	}

	statusStr := r.URL.Query().Get("status")
	if statusStr == "" {
		statusStr = string(tax.Single)
	}

	schedule, err := s.TaxStore.GetBrackets(year, tax.FilingStatus(statusStr))
	if err != nil {
		http.Error(w, "failed to get brackets: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Year":        year,
		"Status":      statusStr,
		"Schedule":    schedule,
		"AllStatuses": tax.AllFilingStatuses(),
	}

	if err := s.Tmpl.ExecuteTemplate(w, "brackets.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func renderUploadError(w http.ResponseWriter, tmpl *template.Template, msg string) {
	data := map[string]interface{}{
		"Error": msg,
	}
	w.WriteHeader(http.StatusBadRequest)
	if err := tmpl.ExecuteTemplate(w, "upload.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func currentTaxYear() int {
	return time.Now().Year()
}

func buildEarnerSummaries(paystubs []db.Paystub) []calc.EarnerSummary {
	byPerson := make(map[string][]db.Paystub)
	for _, p := range paystubs {
		byPerson[p.PersonName] = append(byPerson[p.PersonName], p)
	}

	var earners []calc.EarnerSummary
	for name, stubs := range byPerson {
		e := calc.EarnerSummary{
			Name:               name,
			PayPeriodsUploaded: len(stubs),
		}

		for _, s := range stubs {
			e.TotalGrossPay += s.GrossPay
			e.TotalFederalWithheld += s.FederalTaxWithheld
		}

		// Use latest stub for YTD values.
		latest := stubs[len(stubs)-1]
		if latest.YTDGrossPay != nil {
			e.LatestYTDGross = *latest.YTDGrossPay
		} else {
			e.LatestYTDGross = e.TotalGrossPay
		}
		if latest.YTDFederalTaxWithheld != nil {
			e.LatestYTDFedWithheld = *latest.YTDFederalTaxWithheld
		} else {
			e.LatestYTDFedWithheld = e.TotalFederalWithheld
		}

		if e.PayPeriodsUploaded > 0 {
			e.AvgGrossPerPeriod = e.TotalGrossPay / float64(e.PayPeriodsUploaded)
		}

		earners = append(earners, e)
	}

	return earners
}
