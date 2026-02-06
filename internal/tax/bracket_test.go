package tax_test

import (
	"math"
	"testing"

	"github.com/brendanwhit/tax-withholding-estimator/internal/tax"
)

// almostEqual checks if two floats are within a tolerance.
func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) < tol
}

func TestCalculateTaxSingleFiler(t *testing.T) {
	schedule := tax.HardcodedBrackets(2025, tax.Single)
	if schedule == nil {
		t.Fatal("expected 2025 Single brackets to exist")
	}

	tests := []struct {
		name     string
		income   float64
		wantTax  float64
		tolerance float64
	}{
		{
			name:      "income below standard deduction",
			income:    10000,
			wantTax:   0,
			tolerance: 0.01,
		},
		{
			name:      "income at standard deduction",
			income:    15000,
			wantTax:   0,
			tolerance: 0.01,
		},
		{
			name:      "income in 10% bracket only",
			income:    25000,
			wantTax:   1000.00, // (25000 - 15000) * 0.10
			tolerance: 0.01,
		},
		{
			name:      "income spanning 10% and 12% brackets",
			income:    50000,
			wantTax:   1192.50 + (50000 - 15000 - 11925) * 0.12,
			tolerance: 0.01,
		},
		{
			name:      "income of 100k",
			income:    100000,
			wantTax:   0, // placeholder, calculate below
			tolerance: 1.00,
		},
		{
			name:      "high income in 37% bracket",
			income:    700000,
			wantTax:   0, // placeholder, calculate below
			tolerance: 1.00,
		},
	}

	// Calculate expected taxes manually for the 100k and 700k cases.
	// 2025 Single: standard deduction 15000
	// Taxable 85000:
	// 10%: 11925 * 0.10 = 1192.50
	// 12%: (48475-11925) * 0.12 = 36550 * 0.12 = 4386.00
	// 22%: (85000-48475) * 0.22 = 36525 * 0.22 = 8035.50
	// Total: 13614.00
	tests[4].wantTax = 13614.00

	// Taxable 685000:
	// 10%: 11925 * 0.10 = 1192.50
	// 12%: 36550 * 0.12 = 4386.00
	// 22%: 54875 * 0.22 = 12072.50
	// 24%: 93950 * 0.24 = 22548.00
	// 32%: 53225 * 0.32 = 17032.00
	// 35%: 375825 * 0.35 = 131538.75
	// 37%: (685000-626350) * 0.37 = 58650 * 0.37 = 21700.50
	// Total: 210470.25
	tests[5].wantTax = 210470.25

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := schedule.CalculateTax(tt.income)
			if !almostEqual(got, tt.wantTax, tt.tolerance) {
				t.Errorf("CalculateTax(%v) = %v, want %v", tt.income, got, tt.wantTax)
			}
		})
	}
}

func TestCalculateTaxMarriedFilingJointly(t *testing.T) {
	schedule := tax.HardcodedBrackets(2025, tax.MarriedFilingJointly)
	if schedule == nil {
		t.Fatal("expected 2025 MFJ brackets to exist")
	}

	tests := []struct {
		name     string
		income   float64
		wantTax  float64
		tolerance float64
	}{
		{
			name:      "income below standard deduction",
			income:    25000,
			wantTax:   0,
			tolerance: 0.01,
		},
		{
			name:      "income in 10% bracket",
			income:    50000,
			wantTax:   (50000 - 30000) * 0.10, // 2000
			tolerance: 0.01,
		},
		{
			name:      "combined income 200k",
			income:    200000,
			wantTax:   0,
			tolerance: 1.00,
		},
	}

	// 200k MFJ: taxable = 170000
	// 10%: 23850 * 0.10 = 2385.00
	// 12%: 73100 * 0.12 = 8772.00
	// 22%: (170000 - 96950) * 0.22 = 73050 * 0.22 = 16071.00
	// Total: 27228.00
	tests[2].wantTax = 27228.00

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := schedule.CalculateTax(tt.income)
			if !almostEqual(got, tt.wantTax, tt.tolerance) {
				t.Errorf("CalculateTax(%v) = %v, want %v", tt.income, got, tt.wantTax)
			}
		})
	}
}

func TestBracketBoundaries(t *testing.T) {
	schedule := tax.HardcodedBrackets(2025, tax.Single)
	if schedule == nil {
		t.Fatal("expected 2025 Single brackets to exist")
	}

	// Income exactly at the standard deduction boundary.
	got := schedule.CalculateTax(15000)
	if got != 0 {
		t.Errorf("tax at standard deduction = %v, want 0", got)
	}

	// Income exactly at the first bracket boundary: taxable = 11925.
	got = schedule.CalculateTax(15000 + 11925)
	want := 11925 * 0.10
	if !almostEqual(got, want, 0.01) {
		t.Errorf("tax at first bracket edge = %v, want %v", got, want)
	}

	// One dollar over the first bracket boundary.
	got = schedule.CalculateTax(15000 + 11925 + 1)
	want = 11925*0.10 + 1*0.12
	if !almostEqual(got, want, 0.01) {
		t.Errorf("tax at first bracket edge + 1 = %v, want %v", got, want)
	}
}

func TestStandardDeductionPerFilingStatus(t *testing.T) {
	tests := []struct {
		status            tax.FilingStatus
		wantDeduction     float64
	}{
		{tax.Single, 15000},
		{tax.MarriedFilingJointly, 30000},
		{tax.MarriedFilingSeparately, 15000},
		{tax.HeadOfHousehold, 22500},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			schedule := tax.HardcodedBrackets(2025, tt.status)
			if schedule == nil {
				t.Fatalf("no 2025 brackets for %s", tt.status)
			}
			if schedule.StandardDeduction != tt.wantDeduction {
				t.Errorf("standard deduction = %v, want %v", schedule.StandardDeduction, tt.wantDeduction)
			}
		})
	}
}

func TestHardcodedBracketsExistForBothYears(t *testing.T) {
	for _, year := range []int{2025, 2026} {
		for _, status := range tax.AllFilingStatuses() {
			schedule := tax.HardcodedBrackets(year, status)
			if schedule == nil {
				t.Errorf("missing brackets for year=%d status=%s", year, status)
				continue
			}
			if len(schedule.Brackets) == 0 {
				t.Errorf("empty brackets for year=%d status=%s", year, status)
			}
			if schedule.StandardDeduction <= 0 {
				t.Errorf("invalid standard deduction for year=%d status=%s: %v", year, status, schedule.StandardDeduction)
			}
		}
	}
}

func TestHardcodedBracketsUnknownYear(t *testing.T) {
	schedule := tax.HardcodedBrackets(2020, tax.Single)
	if schedule != nil {
		t.Error("expected nil for unsupported year")
	}
}

func TestZeroIncome(t *testing.T) {
	schedule := tax.HardcodedBrackets(2025, tax.Single)
	if schedule == nil {
		t.Fatal("expected brackets")
	}
	got := schedule.CalculateTax(0)
	if got != 0 {
		t.Errorf("tax on zero income = %v, want 0", got)
	}
}
