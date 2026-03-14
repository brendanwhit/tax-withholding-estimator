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

// TestParsePaystubColumnarFormat tests parsing of paystubs with columnar layouts
// where labels and values appear on separate lines (e.g., labels in one column,
// values in another). This simulates the text extraction pattern from real
// columnar PDFs (like those from small payroll providers).
func TestParsePaystubColumnarFormat(t *testing.T) {
	// Simulates extracted text from a columnar PDF where:
	// - dates appear before their labels (columnar interleave)
	// - "Statement of Earnings For:" label with name on next line
	// - "FEDERAL WH" with current/YTD on next lines
	// - Earnings total row with 4 values (hrs, current$, ytd_hrs, ytd$)
	// - 403b deduction with current/YTD on next lines
	content := `Statement of Earnings For:
Emily

1/1/2026
1/14/2026

Period Begin:
Period End:

Gross Pay
$3,200.00

FEDERAL WH
98.50
257.30

Total:
65.00
3,200.00
195.00
9,600.00

403b
80.00
480.00
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

	if data.FirstName != "Emily" {
		t.Errorf("FirstName = %q, want %q", data.FirstName, "Emily")
	}
	if data.GrossPay != 3200.00 {
		t.Errorf("GrossPay = %v, want 3200.00", data.GrossPay)
	}
	if data.FederalTaxWithheld != 98.50 {
		t.Errorf("FederalTaxWithheld = %v, want 98.50", data.FederalTaxWithheld)
	}
	if data.PayPeriodStart.Format("2006-01-02") != "2026-01-01" {
		t.Errorf("PayPeriodStart = %v, want 2026-01-01", data.PayPeriodStart)
	}
	if data.PayPeriodEnd.Format("2006-01-02") != "2026-01-14" {
		t.Errorf("PayPeriodEnd = %v, want 2026-01-14", data.PayPeriodEnd)
	}
	if data.YTDGrossPay != 9600.00 {
		t.Errorf("YTDGrossPay = %v, want 9600.00", data.YTDGrossPay)
	}
	if data.YTDFederalTaxWithheld != 257.30 {
		t.Errorf("YTDFederalTaxWithheld = %v, want 257.30", data.YTDFederalTaxWithheld)
	}

	// Check 403b deduction.
	found403b := false
	for _, d := range data.Deductions {
		if d.Type == "403b" {
			found403b = true
			if d.Amount != 80.00 {
				t.Errorf("403b Amount = %v, want 80.00", d.Amount)
			}
			if d.YTDAmount != 480.00 {
				t.Errorf("403b YTDAmount = %v, want 480.00", d.YTDAmount)
			}
		}
	}
	if !found403b {
		t.Error("expected 403b deduction to be extracted")
	}
}

func TestParsePaystubADPFormat(t *testing.T) {
	content := `ADP EARNINGS STATEMENT
Employee Name: David Chen
Pay Period: 01/01/2025 - 01/15/2025

Total Earnings  $6,250.00  $12,500.00
FWT             $937.50    $1,875.00
State Tax       $312.50    $625.00
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

	if data.FirstName != "David" {
		t.Errorf("FirstName = %q, want %q", data.FirstName, "David")
	}
	if data.GrossPay != 6250.00 {
		t.Errorf("GrossPay = %v, want 6250.00", data.GrossPay)
	}
	if data.FederalTaxWithheld != 937.50 {
		t.Errorf("FederalTaxWithheld = %v, want 937.50", data.FederalTaxWithheld)
	}
	if data.YTDGrossPay != 12500.00 {
		t.Errorf("YTDGrossPay = %v, want 12500.00", data.YTDGrossPay)
	}
	if data.YTDFederalTaxWithheld != 1875.00 {
		t.Errorf("YTDFederalTaxWithheld = %v, want 1875.00", data.YTDFederalTaxWithheld)
	}
}

func TestParsePaystubGustoFormat(t *testing.T) {
	content := `Gusto Payroll
Employee: Maria Garcia
Pay Period: 02/01/2025 - 02/15/2025
Gross Wages: $4,500.00
FIT: $675.00
Year-to-Date Gross: $9,000.00
Year-to-Date Tax: $1,350.00
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

	if data.FirstName != "Maria" {
		t.Errorf("FirstName = %q, want %q", data.FirstName, "Maria")
	}
	if data.GrossPay != 4500.00 {
		t.Errorf("GrossPay = %v, want 4500.00", data.GrossPay)
	}
	if data.FederalTaxWithheld != 675.00 {
		t.Errorf("FederalTaxWithheld = %v, want 675.00", data.FederalTaxWithheld)
	}
	if data.YTDGrossPay != 9000.00 {
		t.Errorf("YTDGrossPay = %v, want 9000.00", data.YTDGrossPay)
	}
	if data.YTDFederalTaxWithheld != 1350.00 {
		t.Errorf("YTDFederalTaxWithheld = %v, want 1350.00", data.YTDFederalTaxWithheld)
	}
}

func TestParsePaystubWorkdayFormat(t *testing.T) {
	content := `Workday Pay Statement
Employee: James Wilson
Period Ending: 03/15/2025
Gross Pay: $5,000.00
Federal Withholding: $750.00
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

	if data.FederalTaxWithheld != 750.00 {
		t.Errorf("FederalTaxWithheld = %v, want 750.00", data.FederalTaxWithheld)
	}
	// Period Ending infers start as end - 14 days.
	if data.PayPeriodEnd.Format("2006-01-02") != "2025-03-15" {
		t.Errorf("PayPeriodEnd = %v, want 2025-03-15", data.PayPeriodEnd)
	}
	if data.PayPeriodStart.Format("2006-01-02") != "2025-03-01" {
		t.Errorf("PayPeriodStart = %v, want 2025-03-01", data.PayPeriodStart)
	}
}

func TestParsePaystubPaychexFormat(t *testing.T) {
	content := `Paychex Payroll
Employee: Karen Brown
Pay Period: 04/01/2025 - 04/15/2025
Gross Earnings: $5,500.00
FWT: $825.00
Gross YTD: $22,000.00
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

	if data.GrossPay != 5500.00 {
		t.Errorf("GrossPay = %v, want 5500.00", data.GrossPay)
	}
	if data.FederalTaxWithheld != 825.00 {
		t.Errorf("FederalTaxWithheld = %v, want 825.00", data.FederalTaxWithheld)
	}
	if data.YTDGrossPay != 22000.00 {
		t.Errorf("YTDGrossPay = %v, want 22000.00", data.YTDGrossPay)
	}
}

func TestParsePaystubGovernmentFormat(t *testing.T) {
	content := `LEAVE AND EARNINGS STATEMENT
Employee: Robert Taylor
Pay Period: 05/01/2025 - 05/15/2025
Total Gross: $4,800.00
FITW: $720.00
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

	if data.GrossPay != 4800.00 {
		t.Errorf("GrossPay = %v, want 4800.00", data.GrossPay)
	}
	if data.FederalTaxWithheld != 720.00 {
		t.Errorf("FederalTaxWithheld = %v, want 720.00", data.FederalTaxWithheld)
	}
}

func TestParsePaystubStatementOfEarnings(t *testing.T) {
	content := `Statement of Earnings For: Patricia
Pay Period: 06/01/2025 - 06/15/2025
Gross Pay: $3,200.00
Federal Tax: $480.00
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

	if data.FirstName != "Patricia" {
		t.Errorf("FirstName = %q, want %q", data.FirstName, "Patricia")
	}
}

func TestParsePaystubEmployeeNameBugfix(t *testing.T) {
	content := `EARNINGS STATEMENT
Employee Name: Susan Miller
Pay Period: 07/01/2025 - 07/15/2025
Gross Pay: $4,000.00
Federal Income Tax: $600.00
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

	if data.FirstName != "Susan" {
		t.Errorf("FirstName = %q, want %q (should not capture 'Name')", data.FirstName, "Susan")
	}
}

func TestParsePaystubDashDates(t *testing.T) {
	content := `EARNINGS STATEMENT
Employee: Tom Anderson
Period: 01-01-2025 - 01-15-2025
Gross Pay: $3,500.00
Federal Tax: $525.00
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

	if data.PayPeriodStart.Format("2006-01-02") != "2025-01-01" {
		t.Errorf("PayPeriodStart = %v, want 2025-01-01", data.PayPeriodStart)
	}
	if data.PayPeriodEnd.Format("2006-01-02") != "2025-01-15" {
		t.Errorf("PayPeriodEnd = %v, want 2025-01-15", data.PayPeriodEnd)
	}
}

func TestParsePaystubFederalWH(t *testing.T) {
	content := `EARNINGS STATEMENT
Employee: Amy Clark
Pay Period: 08/01/2025 - 08/15/2025
Gross Pay: $4,200.00
FEDERAL WH: $630.00
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

	if data.FederalTaxWithheld != 630.00 {
		t.Errorf("FederalTaxWithheld = %v, want 630.00", data.FederalTaxWithheld)
	}
}

func TestParsePaystubPeriodBeginEnd(t *testing.T) {
	content := `EARNINGS STATEMENT
Employee: Chris Evans
Period Begin: 09/01/2025
Period End: 09/15/2025
Gross Pay: $4,500.00
Federal Tax: $675.00
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

	if data.PayPeriodStart.Format("2006-01-02") != "2025-09-01" {
		t.Errorf("PayPeriodStart = %v, want 2025-09-01", data.PayPeriodStart)
	}
	if data.PayPeriodEnd.Format("2006-01-02") != "2025-09-15" {
		t.Errorf("PayPeriodEnd = %v, want 2025-09-15", data.PayPeriodEnd)
	}
}

// TestParsePaystubRipplingFormat tests parsing of Rippling-style paystubs where
// a SUMMARY section has "Gross Pay\n$current\n$ytd" on separate lines, and
// the EARNINGS STATEMENT section has "PAID TO:" with the name on the next line.
func TestParsePaystubRipplingFormat(t *testing.T) {
	content := `SUMMARY
CURRENT
YTD
Gross Pay
$5,500.00
$11,000.00
Deductions
$1,200.00
$2,400.00
Taxes
$900.00
$1,800.00
Net Pay
$3,398.00
$6,796.00

TAXES WITHHELD
CURRENT
YTD
Federal Income Tax
$500.00
$1,000.00
Medicare
$79.75
$159.50
Social Security
$341.00
$682.00

DEDUCTIONS
CURRENT EMP.
DEDUCTION
YTD EMP.
DEDUCTION
HSA
$200.00
$0.00
$400.00
$0.00
401K
$500.00
$65.00
$1,000.00
$130.00

EARNINGS STATEMENT
ACME CORP.
PAID TO:
Alex
PAY PERIOD:
02/01/2026 - 02/15/2026
PAY DATE:
02/20/2026
GROSS PAY:
$5,500.00
NET PAY:
$3,398.00
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

	if data.FirstName != "Alex" {
		t.Errorf("FirstName = %q, want %q", data.FirstName, "Alex")
	}
	if data.GrossPay != 5500.00 {
		t.Errorf("GrossPay = %v, want 5500.00", data.GrossPay)
	}
	if data.FederalTaxWithheld != 500.00 {
		t.Errorf("FederalTaxWithheld = %v, want 500.00", data.FederalTaxWithheld)
	}
	if data.PayPeriodStart.Format("2006-01-02") != "2026-02-01" {
		t.Errorf("PayPeriodStart = %v, want 2026-02-01", data.PayPeriodStart)
	}
	if data.PayPeriodEnd.Format("2006-01-02") != "2026-02-15" {
		t.Errorf("PayPeriodEnd = %v, want 2026-02-15", data.PayPeriodEnd)
	}
	if data.YTDGrossPay != 11000.00 {
		t.Errorf("YTDGrossPay = %v, want 11000.00", data.YTDGrossPay)
	}
	if data.YTDFederalTaxWithheld != 1000.00 {
		t.Errorf("YTDFederalTaxWithheld = %v, want 1000.00", data.YTDFederalTaxWithheld)
	}

	// Check deductions.
	types := make(map[string]float64)
	for _, d := range data.Deductions {
		types[d.Type] = d.Amount
	}
	if amt, ok := types["401k"]; !ok || amt != 500.00 {
		t.Errorf("401k = %v, want 500.00 (found=%v)", amt, ok)
	}
	if amt, ok := types["hsa"]; !ok || amt != 200.00 {
		t.Errorf("HSA = %v, want 200.00 (found=%v)", amt, ok)
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
