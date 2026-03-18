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

func makeStubWithHours(person string, year int, startMonth, startDay, endMonth, endDay int, gross, fedTax, hours float64) db.Paystub {
	s := makeStubDates(person, year, startMonth, startDay, endMonth, endDay, gross, fedTax)
	s.Hours = &hours
	return s
}

func float64Ptr(v float64) *float64 { return &v }

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

func TestInferPayFrequencyBiweeklyNonConsecutive(t *testing.T) {
	// Two biweekly stubs from different months — gap is ~28 days but
	// each pay period spans ~14 days, so frequency should be biweekly.
	stubs := []db.Paystub{
		makeStubDates("Lucy", 2025, 1, 13, 1, 24, 5000, 800),
		makeStubDates("Lucy", 2025, 2, 10, 2, 21, 5000, 800),
	}
	freq := calc.InferPayFrequency(stubs)
	if freq != calc.FrequencyBiweekly {
		t.Errorf("expected Bi-weekly for non-consecutive stubs, got %s", freq)
	}
}

func TestInferPayFrequencySingleStub(t *testing.T) {
	// A single biweekly stub should be inferred from its period length.
	stubs := []db.Paystub{
		makeStubDates("A", 2025, 3, 3, 3, 14, 5000, 800),
	}
	freq := calc.InferPayFrequency(stubs)
	if freq != calc.FrequencyBiweekly {
		t.Errorf("expected Bi-weekly for single 14-day stub, got %s", freq)
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
	// Latest pay period end is Feb 25. Days elapsed = 55, period = 14.04 days.
	// Elapsed periods = round(55/14.04) = 4. Remaining = 26 - 4 = 22.
	// Projected remaining should be 22 * 800 = 17600.
	if bob.ProjectedRemaining != 22*800 {
		t.Errorf("projected remaining = %v, want %v", bob.ProjectedRemaining, 22*800)
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

func TestProjectionUsesYTDWithheldFromLatestStub(t *testing.T) {
	// Simulate 3 uploaded stubs out of 5 elapsed pay periods.
	// Each stub has $870 current federal tax, but the latest stub's YTD
	// shows $4,041.03 (covering all 5 periods, not just the 3 uploaded).
	ytdFed := 4041.03
	stubs := map[string][]db.Paystub{
		"Brendan": {
			{
				PersonName:     "Brendan",
				TaxYear:        2025,
				PayPeriodStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				PayPeriodEnd:   time.Date(2025, 1, 14, 0, 0, 0, 0, time.UTC),
				GrossPay:       5838.46,
				FederalTaxWithheld: 870.00,
			},
			{
				PersonName:     "Brendan",
				TaxYear:        2025,
				PayPeriodStart: time.Date(2025, 1, 29, 0, 0, 0, 0, time.UTC),
				PayPeriodEnd:   time.Date(2025, 2, 11, 0, 0, 0, 0, time.UTC),
				GrossPay:       5838.46,
				FederalTaxWithheld: 870.00,
			},
			{
				PersonName:            "Brendan",
				TaxYear:               2025,
				PayPeriodStart:        time.Date(2025, 2, 26, 0, 0, 0, 0, time.UTC),
				PayPeriodEnd:          time.Date(2025, 3, 11, 0, 0, 0, 0, time.UTC),
				GrossPay:              5838.46,
				FederalTaxWithheld:    870.00,
				YTDFederalTaxWithheld: &ytdFed,
			},
		},
	}

	refDate := time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)
	proj := calc.ProjectEOYWithholding(stubs, refDate, 2025)

	brendan := proj.Earners[0]

	// Should use YTD ($4,041.03), not sum of uploaded stubs ($2,610).
	if brendan.TotalWithheldToDate != 4041.03 {
		t.Errorf("TotalWithheldToDate = %v, want 4041.03 (from YTD)", brendan.TotalWithheldToDate)
	}

	// Sum of individual stubs is $2,610, so YTD is higher — confirms
	// the projection accounts for non-uploaded pay periods.
	sumOfStubs := 870.0 * 3
	if brendan.TotalWithheldToDate <= sumOfStubs {
		t.Errorf("TotalWithheldToDate (%v) should exceed sum of uploaded stubs (%v)",
			brendan.TotalWithheldToDate, sumOfStubs)
	}
}

func TestProjectionFallsBackToSumWhenNoYTD(t *testing.T) {
	// Stubs without YTD data should still work by summing individual amounts.
	stubs := map[string][]db.Paystub{
		"Alice": {
			makeStubDates("Alice", 2025, 1, 1, 1, 14, 5000, 800),
			makeStubDates("Alice", 2025, 1, 15, 1, 28, 5000, 800),
			makeStubDates("Alice", 2025, 1, 29, 2, 11, 5000, 800),
		},
	}

	refDate := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	proj := calc.ProjectEOYWithholding(stubs, refDate, 2025)

	alice := proj.Earners[0]
	if alice.TotalWithheldToDate != 2400 {
		t.Errorf("TotalWithheldToDate = %v, want 2400 (sum of stubs)", alice.TotalWithheldToDate)
	}
}

func TestInferPayTypeSalaried(t *testing.T) {
	// Identical gross pay → salaried.
	stubs := []db.Paystub{
		makeStubDates("A", 2025, 1, 1, 1, 14, 5000, 800),
		makeStubDates("A", 2025, 1, 15, 1, 28, 5000, 800),
		makeStubDates("A", 2025, 1, 29, 2, 11, 5000, 800),
	}
	pt := calc.InferPayType(stubs)
	if pt != calc.PayTypeSalaried {
		t.Errorf("expected Salaried, got %s", pt)
	}
}

func TestInferPayTypeHourly(t *testing.T) {
	// Varying gross pay → hourly.
	stubs := []db.Paystub{
		makeStubDates("A", 2025, 1, 1, 1, 14, 2400, 300),
		makeStubDates("A", 2025, 1, 15, 1, 28, 3200, 420),
		makeStubDates("A", 2025, 1, 29, 2, 11, 2800, 360),
		makeStubDates("A", 2025, 2, 12, 2, 25, 3600, 480),
	}
	pt := calc.InferPayType(stubs)
	if pt != calc.PayTypeHourly {
		t.Errorf("expected Hourly, got %s", pt)
	}
}

func TestInferPayTypeSalariedWithFortyHours(t *testing.T) {
	// Varying gross but all 80.0 hours (biweekly 40h/wk) → salaried.
	stubs := []db.Paystub{
		makeStubWithHours("A", 2025, 1, 1, 1, 14, 5000, 800, 80),
		makeStubWithHours("A", 2025, 1, 15, 1, 28, 5100, 810, 80),
		makeStubWithHours("A", 2025, 1, 29, 2, 11, 4900, 790, 80),
	}
	pt := calc.InferPayType(stubs)
	if pt != calc.PayTypeSalaried {
		t.Errorf("expected Salaried (consistent hours), got %s", pt)
	}
}

func TestInferPayTypeDefaultWithFewStubs(t *testing.T) {
	stubs := []db.Paystub{
		makeStubDates("A", 2025, 1, 1, 1, 14, 5000, 800),
	}
	pt := calc.InferPayType(stubs)
	if pt != calc.PayTypeSalaried {
		t.Errorf("expected Salaried default with <2 stubs, got %s", pt)
	}
}

func TestProjectionHourlyUsesAverage(t *testing.T) {
	// Varying gross pay → hourly → projection uses average withholding.
	stubs := map[string][]db.Paystub{
		"Charlie": {
			makeStubDates("Charlie", 2025, 1, 1, 1, 14, 2400, 300),
			makeStubDates("Charlie", 2025, 1, 15, 1, 28, 3200, 420),
			makeStubDates("Charlie", 2025, 1, 29, 2, 11, 2800, 360),
		},
	}

	refDate := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	proj := calc.ProjectEOYWithholding(stubs, refDate, 2025)

	charlie := proj.Earners[0]
	if charlie.PayType != calc.PayTypeHourly {
		t.Errorf("expected Hourly pay type, got %s", charlie.PayType)
	}

	// Average withholding across 3 regular stubs: (300+420+360)/3 = 360.
	expectedAvg := 360.0
	if charlie.AvgWithheldPerPeriod != expectedAvg {
		t.Errorf("AvgWithheldPerPeriod = %v, want %v", charlie.AvgWithheldPerPeriod, expectedAvg)
	}

	// Projected remaining = avg * remaining periods.
	expected := expectedAvg * float64(charlie.RemainingPayPeriods)
	if charlie.ProjectedRemaining != expected {
		t.Errorf("ProjectedRemaining = %v, want %v (avg*remaining)", charlie.ProjectedRemaining, expected)
	}
}

func TestProjectionSalariedUsesLastPaycheck(t *testing.T) {
	// Consistent gross → salaried → uses last paycheck withholding.
	stubs := map[string][]db.Paystub{
		"Diana": {
			makeStubDates("Diana", 2025, 1, 1, 1, 14, 5000, 800),
			makeStubDates("Diana", 2025, 1, 15, 1, 28, 5000, 800),
			makeStubDates("Diana", 2025, 1, 29, 2, 11, 5000, 850),
		},
	}

	refDate := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	proj := calc.ProjectEOYWithholding(stubs, refDate, 2025)

	diana := proj.Earners[0]
	if diana.PayType != calc.PayTypeSalaried {
		t.Errorf("expected Salaried pay type, got %s", diana.PayType)
	}

	// Salaried should use last regular paycheck ($850), not average.
	expected := 850.0 * float64(diana.RemainingPayPeriods)
	if diana.ProjectedRemaining != expected {
		t.Errorf("ProjectedRemaining = %v, want %v (lastRegular*remaining)", diana.ProjectedRemaining, expected)
	}
}
