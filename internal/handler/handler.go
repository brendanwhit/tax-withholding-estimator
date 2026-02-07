package handler

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
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

// uploadResult holds the result of processing a single uploaded file.
type uploadResult struct {
	Filename  string
	Success   bool
	Error     string
	Data      *pdfparse.PaystubData
	Year      int
}

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
		"formatDeductionType": func(s string) string {
			switch s {
			case "401k":
				return "401(k)"
			case "403b":
				return "403(b)"
			case "hsa":
				return "HSA"
			case "fsa_health":
				return "Healthcare FSA"
			case "fsa_dependent":
				return "Dependent Care FSA"
			case "commuter":
				return "Commuter"
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

// devRoute is used by build-tag-gated files to register additional routes.
type devRoute struct {
	pattern string
	handler func(s *Server) http.HandlerFunc
}

var devRoutes []devRoute

// RegisterRoutes sets up all HTTP routes.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/upload", s.handleUpload)
	mux.HandleFunc("/filing-status", s.handleFilingStatus)
	mux.HandleFunc("/brackets", s.handleBrackets)

	for _, r := range devRoutes {
		mux.HandleFunc(r.pattern, r.handler(s))
	}
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

	// Load deduction data for the dashboard.
	deductionSummaries, err := s.Store.GetDeductionSummaryByYear(year)
	if err != nil {
		log.Printf("failed to get deduction summaries: %v", err)
	}
	limits, err := s.Store.GetOrCacheContributionLimits(year)
	if err != nil {
		log.Printf("failed to get contribution limits: %v", err)
	}

	// Build limit lookup and warning info.
	limitMap := make(map[string]float64)
	for _, l := range limits {
		limitMap[l.DeductionType] = l.AnnualLimit
	}

	type deductionWarning struct {
		PersonName    string
		DeductionType string
		YTDTotal      float64
		Limit         float64
		Pct           float64
	}
	var warnings []deductionWarning
	for _, ds := range deductionSummaries {
		limit, ok := limitMap[ds.DeductionType]
		if !ok || limit == 0 {
			continue
		}
		pct := ds.TotalAmount / limit
		if pct >= 0.9 {
			warnings = append(warnings, deductionWarning{
				PersonName:    ds.PersonName,
				DeductionType: ds.DeductionType,
				YTDTotal:      ds.TotalAmount,
				Limit:         limit,
				Pct:           pct * 100,
			})
		}
	}

	// Calculate total pre-tax deductions for tax calculation adjustment.
	totalPreTaxDeductions, err := s.Store.GetTotalPreTaxDeductionsByYear(year)
	if err != nil {
		log.Printf("failed to get total deductions: %v", err)
	}

	result := calc.CalculateWithholding(schedule, earners, 0, 26, time.Now())

	// Build EOY projection per person.
	paystubsByPerson := make(map[string][]db.Paystub)
	for _, p := range paystubs {
		paystubsByPerson[p.PersonName] = append(paystubsByPerson[p.PersonName], p)
	}
	eoyProjection := calc.ProjectEOYWithholding(paystubsByPerson, time.Now(), year)

	data := map[string]interface{}{
		"Year":                 year,
		"FilingStatus":         filingStatus,
		"Result":               result,
		"Paystubs":             paystubs,
		"HasPaystubs":          len(paystubs) > 0,
		"AllStatuses":          tax.AllFilingStatuses(),
		"DeductionSummaries":   deductionSummaries,
		"DeductionWarnings":    warnings,
		"TotalPreTaxDeductions": totalPreTaxDeductions,
		"EOYProjection":        eoyProjection,
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
		renderUploadError(w, r, s.Tmpl, "Failed to parse form: "+err.Error())
		return
	}

	files := r.MultipartForm.File["paystub"]
	if len(files) == 0 {
		renderUploadError(w, r, s.Tmpl, "No file uploaded")
		return
	}

	var results []uploadResult
	for _, header := range files {
		result := s.processUploadedFile(header)
		results = append(results, result)
	}

	// Check if all files failed
	allFailed := true
	for _, r := range results {
		if r.Success {
			allFailed = false
			break
		}
	}

	tmplData := map[string]interface{}{
		"Results": results,
	}

	if allFailed {
		w.WriteHeader(http.StatusBadRequest)
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

func (s *Server) processUploadedFile(header *multipart.FileHeader) uploadResult {
	result := uploadResult{Filename: header.Filename}

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".pdf") {
		result.Error = "Only PDF files are accepted"
		return result
	}

	file, err := header.Open()
	if err != nil {
		result.Error = "Failed to open file"
		return result
	}
	defer func() { _ = file.Close() }()

	tmpFile, err := os.CreateTemp("", "paystub-*.pdf")
	if err != nil {
		result.Error = "Failed to create temp file"
		return result
	}
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	}()

	size, err := io.Copy(tmpFile, file)
	if err != nil {
		result.Error = "Failed to save uploaded file"
		return result
	}

	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		result.Error = "Failed to read uploaded file"
		return result
	}

	data, err := pdfparse.ParsePaystub(tmpFile, size)
	if err != nil {
		result.Error = "Failed to parse paystub: " + err.Error()
		return result
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

	paystubID, err := s.Store.SavePaystub(paystub)
	if err != nil {
		result.Error = "Failed to save paystub: " + err.Error()
		return result
	}

	// Save pre-tax deductions if extracted.
	for _, d := range data.Deductions {
		ded := &db.PreTaxDeduction{
			PaystubID:     paystubID,
			DeductionType: d.Type,
			Amount:        d.Amount,
		}
		if d.YTDAmount > 0 {
			ded.YTDAmount = &d.YTDAmount
		}
		if saveErr := s.Store.SavePreTaxDeduction(ded); saveErr != nil {
			log.Printf("failed to save deduction %s for paystub %d: %v", d.Type, paystubID, saveErr)
		}
	}

	result.Success = true
	result.Data = data
	result.Year = year
	return result
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

func renderUploadError(w http.ResponseWriter, r *http.Request, tmpl *template.Template, msg string) {
	data := map[string]interface{}{
		"Error": msg,
	}
	w.WriteHeader(http.StatusBadRequest)
	templateName := "upload.html"
	if isHTMX(r) {
		templateName = "upload-result.html"
	}
	if err := tmpl.ExecuteTemplate(w, templateName, data); err != nil {
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
