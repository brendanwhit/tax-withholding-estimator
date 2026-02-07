package calc_test

import (
	"testing"
	"time"

	"github.com/brendanwhit/tax-withholding-estimator/internal/calc"
	"github.com/brendanwhit/tax-withholding-estimator/internal/db"
)

func makeStubDates(person string, year int, startMonth, startDay, endMonth, endDay int, gross, fedTax float64) db.Paystub {
	return db.Paystub{
		PersonName:         person,
		TaxYear:            year,
		PayPeriodStart:     time.Date(year, time.Month(startMonth), startDay, 0, 0, 0, 0, time.UTC),
		PayPeriodEnd:       time.Date(year, time.Month(endMonth), endDay, 0, 0, 0, 0, time.UTC),
		GrossPay:           gross,
		FederalTaxWithheld: fedTax,
	}
}

func TestInferPayFrequencyBiweekly(t *testing.T) {
	stubs := []db.Paystub{
		makeStubDates("A", 2025, 1, 1, 1, 14, 5000, 800),
		makeStubDates("A", 2025, 1, 15, 1, 28, 5000, 800),
		makeStubDates("A", 2025, 1, 29, 2, 11, 5000, 800),
		makeStubDates("A", 2025, 2, 12, 2, 25, 5000, 800),
	}
	freq := calc.InferPayFrequency(stubs)
	if freq != calc.FrequencyBiweekly {
		t.Errorf("expected Bi-weekly, got %s", freq)
	}
}

func TestInferPayFrequencySemiMonthly(t *testing.T) {
	stubs := []db.Paystub{
		makeStubDates("A", 2025, 1, 1, 1, 15, 4000, 600),
		makeStubDates("A", 2025, 1, 16, 1, 31, 4000, 600),
		makeStubDates("A", 2025, 2, 1, 2, 15, 4000, 600),
		makeStubDates("A", 2025, 2, 16, 2, 28, 4000, 600),
	}
	freq := calc.InferPayFrequency(stubs)
	if freq != calc.FrequencySemiMonthly {
		t.Errorf("expected Semi-monthly, got %s", freq)
	}
}

func TestInferPayFrequencyMonthly(t *testing.T) {
	stubs := []db.Paystub{
		makeStubDates("A", 2025, 1, 1, 1, 31, 8000, 1200),
		makeStubDates("A", 2025, 2, 1, 2, 28, 8000, 1200),
		makeStubDates("A", 2025, 3, 1, 3, 31, 8000, 1200),
	}
	freq := calc.InferPayFrequency(stubs)
	if freq != calc.FrequencyMonthly {
		t.Errorf("expected Monthly, got %s", freq)
	}
}

func TestProjectEOYWithholding(t *testing.T) {
	// 3 biweekly paystubs for Alice (14-day gaps).
	stubs := map[string][]db.Paystub{
		"Alice": {
			makeStubDates("Alice", 2025, 1, 1, 1, 14, 5000, 800),
			makeStubDates("Alice", 2025, 1, 15, 1, 28, 5000, 800),
			makeStubDates("Alice", 2025, 1, 29, 2, 11, 5000, 800),
		},
	}

	refDate := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	proj := calc.ProjectEOYWithholding(stubs, refDate, 2025)

	if len(proj.Earners) != 1 {
		t.Fatalf("expected 1 earner, got %d", len(proj.Earners))
	}

	alice := proj.Earners[0]
	if alice.PayFrequency != calc.FrequencyBiweekly {
		t.Errorf("expected Bi-weekly, got %s", alice.PayFrequency)
	}

	// 3 periods done, 23 remaining (26-3).
	if alice.RemainingPayPeriods != 23 {
		t.Errorf("remaining periods = %d, want 23", alice.RemainingPayPeriods)
	}

	// Total withheld so far: 3 * 800 = 2400.
	if alice.TotalWithheldToDate != 2400 {
		t.Errorf("total withheld = %v, want 2400", alice.TotalWithheldToDate)
	}

	// Projected remaining: 23 * 800 = 18400.
	if alice.ProjectedRemaining != 18400 {
		t.Errorf("projected remaining = %v, want 18400", alice.ProjectedRemaining)
	}

	// EOY total: 2400 + 18400 = 20800.
	if alice.ProjectedEOYTotal != 20800 {
		t.Errorf("EOY total = %v, want 20800", alice.ProjectedEOYTotal)
	}
}

func TestBonusDetection(t *testing.T) {
	// Regular biweekly paystubs plus one bonus.
	stubs := map[string][]db.Paystub{
		"Bob": {
			makeStubDates("Bob", 2025, 1, 1, 1, 14, 5000, 800),
			makeStubDates("Bob", 2025, 1, 15, 1, 28, 5000, 800),
			// Bonus: off-cycle (single-day), high amount, ~22% withholding.
			{
				PersonName:         "Bob",
				TaxYear:            2025,
				PayPeriodStart:     time.Date(2025, 2, 5, 0, 0, 0, 0, time.UTC),
				PayPeriodEnd:       time.Date(2025, 2, 5, 0, 0, 0, 0, time.UTC),
				GrossPay:           15000,
				FederalTaxWithheld: 3300, // 22% supplemental rate
			},
			makeStubDates("Bob", 2025, 1, 29, 2, 11, 5000, 800),
			makeStubDates("Bob", 2025, 2, 12, 2, 25, 5000, 800),
		},
	}

	refDate := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	proj := calc.ProjectEOYWithholding(stubs, refDate, 2025)

	bob := proj.Earners[0]

	// Should detect the bonus.
	if bob.BonusCount == 0 {
		t.Error("expected at least 1 bonus detected")
	}

	// Bonus withholding should be included in total but not in projection.
	if bob.BonusWithheldToDate != 3300 {
		t.Errorf("bonus withheld = %v, want 3300", bob.BonusWithheldToDate)
	}

	// Projection should use regular paycheck ($800), not the bonus.
	// 4 regular stubs + 1 bonus = 5 total. Remaining = 26 - 5 = 21.
	// Projected remaining should be 21 * 800 = 16800.
	if bob.ProjectedRemaining != 21*800 {
		t.Errorf("projected remaining = %v, want %v", bob.ProjectedRemaining, 21*800)
	}
}

func TestSeparateProjectionsPerPerson(t *testing.T) {
	stubs := map[string][]db.Paystub{
		"Alice": {
			makeStubDates("Alice", 2025, 1, 1, 1, 14, 5000, 800),
			makeStubDates("Alice", 2025, 1, 15, 1, 28, 5000, 800),
		},
		"Bob": {
			makeStubDates("Bob", 2025, 1, 1, 1, 14, 4000, 600),
			makeStubDates("Bob", 2025, 1, 15, 1, 28, 4000, 600),
			makeStubDates("Bob", 2025, 1, 29, 2, 11, 4000, 600),
		},
	}

	refDate := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	proj := calc.ProjectEOYWithholding(stubs, refDate, 2025)

	if len(proj.Earners) != 2 {
		t.Fatalf("expected 2 earners, got %d", len(proj.Earners))
	}

	// Verify each person has independent projections.
	for _, ep := range proj.Earners {
		if ep.TotalWithheldToDate == 0 {
			t.Errorf("%s has 0 withheld", ep.Name)
		}
		if ep.ProjectedEOYTotal == 0 {
			t.Errorf("%s has 0 projected EOY total", ep.Name)
		}
	}

	// Combined should be sum of both.
	if proj.CombinedEOYEstimate == 0 {
		t.Error("combined EOY estimate should be > 0")
	}
}
