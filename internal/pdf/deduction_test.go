package pdf_test

import (
	"os"
	"testing"

	pdfparse "github.com/brendanwhit/tax-withholding-estimator/internal/pdf"
)

func TestParsePaystub401kAndHSA(t *testing.T) {
	content := `EARNINGS STATEMENT
Employee: Sarah Johnson
Pay Period: 01/01/2025 - 01/15/2025

Gross Pay: $5,384.62
Federal Income Tax: $807.69
401(k): $500.00
HSA: $150.00

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

	if len(data.Deductions) == 0 {
		t.Fatal("expected deductions to be extracted")
	}

	found401k := false
	foundHSA := false
	for _, d := range data.Deductions {
		switch d.Type {
		case "401k":
			found401k = true
			if d.Amount != 500.00 {
				t.Errorf("401k amount = %v, want 500.00", d.Amount)
			}
		case "hsa":
			foundHSA = true
			if d.Amount != 150.00 {
				t.Errorf("HSA amount = %v, want 150.00", d.Amount)
			}
		}
	}
	if !found401k {
		t.Error("expected 401k deduction to be extracted")
	}
	if !foundHSA {
		t.Error("expected HSA deduction to be extracted")
	}
}

func TestParsePaystubFSAAndCommuter(t *testing.T) {
	content := `EARNINGS STATEMENT
Employee: Mike Williams
Pay Period: 02/01/2025 - 02/15/2025

Gross Pay: $4,000.00
Federal Income Tax: $600.00
FSA: $125.00
Dependent Care FSA: $200.00
Commuter: $100.00
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

	types := make(map[string]float64)
	for _, d := range data.Deductions {
		types[d.Type] = d.Amount
	}

	if amt, ok := types["fsa_health"]; !ok || amt != 125.00 {
		t.Errorf("fsa_health = %v, want 125.00 (found=%v)", amt, ok)
	}
	if amt, ok := types["fsa_dependent"]; !ok || amt != 200.00 {
		t.Errorf("fsa_dependent = %v, want 200.00 (found=%v)", amt, ok)
	}
	if amt, ok := types["commuter"]; !ok || amt != 100.00 {
		t.Errorf("commuter = %v, want 100.00 (found=%v)", amt, ok)
	}
}

func TestParsePaystubNoDeductions(t *testing.T) {
	content := `EARNINGS STATEMENT
Employee: Jane Doe
Pay Period: 03/01/2025 - 03/15/2025
Gross Pay: $3,000.00
Federal Income Tax: $450.00
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

	if len(data.Deductions) != 0 {
		t.Errorf("expected no deductions, got %d", len(data.Deductions))
	}
}
