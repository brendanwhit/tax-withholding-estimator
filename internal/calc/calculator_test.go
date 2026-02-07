package calc_test

import (
	"math"
	"testing"
	"time"

	"github.com/brendanwhit/tax-withholding-estimator/internal/calc"
	"github.com/brendanwhit/tax-withholding-estimator/internal/tax"
)

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) < tol
}

func withholding(schedule *tax.BracketSchedule, earners []calc.EarnerSummary, ref time.Time) *calc.WithholdingResult {
	return calc.CalculateWithholding(calc.WithholdingInput{
		Schedule:               schedule,
		Earners:                earners,
		TotalPayPeriodsPerYear: 26,
		ReferenceDate:          ref,
	})
}

func TestCombinedTaxLiabilityFromTwoIncomes(t *testing.T) {
	schedule := tax.HardcodedBrackets(2025, tax.MarriedFilingJointly)
	if schedule == nil {
		t.Fatal("expected MFJ brackets")
	}

	earners := []calc.EarnerSummary{
		{
			Name:                 "Alice",
			TotalGrossPay:        50000,
			TotalFederalWithheld: 7500,
			PayPeriodsUploaded:   13, // half year
			LatestYTDGross:       50000,
			LatestYTDFedWithheld: 7500,
			AvgGrossPerPeriod:    50000.0 / 13.0,
		},
		{
			Name:                 "Bob",
			TotalGrossPay:        40000,
			TotalFederalWithheld: 5000,
			PayPeriodsUploaded:   13,
			LatestYTDGross:       40000,
			LatestYTDFedWithheld: 5000,
			AvgGrossPerPeriod:    40000.0 / 13.0,
		},
	}

	result := withholding(schedule, earners, time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC))

	// Combined income = (50000/13*26) + (40000/13*26) = 100000 + 80000 = ~180000 annually
	if result.EstimatedAnnualIncome < 170000 || result.EstimatedAnnualIncome > 190000 {
		t.Errorf("EstimatedAnnualIncome = %v, expected ~180000", result.EstimatedAnnualIncome)
	}

	// Tax liability should be > 0.
	if result.TotalTaxLiability <= 0 {
		t.Errorf("TotalTaxLiability = %v, expected > 0", result.TotalTaxLiability)
	}

	// Combined withheld = 7500 + 5000 = 12500.
	if result.TotalWithheldToDate != 12500 {
		t.Errorf("TotalWithheldToDate = %v, want 12500", result.TotalWithheldToDate)
	}
}

func TestSubtractsTotalWithheldFromBothEarners(t *testing.T) {
	schedule := tax.HardcodedBrackets(2025, tax.MarriedFilingJointly)
	if schedule == nil {
		t.Fatal("expected brackets")
	}

	earners := []calc.EarnerSummary{
		{
			Name:                 "Carol",
			TotalGrossPay:        60000,
			TotalFederalWithheld: 10000,
			PayPeriodsUploaded:   26,
			LatestYTDGross:       60000,
			LatestYTDFedWithheld: 10000,
			AvgGrossPerPeriod:    60000.0 / 26.0,
		},
		{
			Name:                 "Dan",
			TotalGrossPay:        50000,
			TotalFederalWithheld: 8000,
			PayPeriodsUploaded:   26,
			LatestYTDGross:       50000,
			LatestYTDFedWithheld: 8000,
			AvgGrossPerPeriod:    50000.0 / 26.0,
		},
	}

	result := withholding(schedule, earners, time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC))

	// Total withheld should include both earners.
	if result.TotalWithheldToDate != 18000 {
		t.Errorf("TotalWithheldToDate = %v, want 18000", result.TotalWithheldToDate)
	}
}

func TestRecommendsAdditionalWithholdingForHigherEarner(t *testing.T) {
	schedule := tax.HardcodedBrackets(2025, tax.MarriedFilingJointly)
	if schedule == nil {
		t.Fatal("expected brackets")
	}

	earners := []calc.EarnerSummary{
		{
			Name:                 "Eve",
			TotalGrossPay:        80000,
			TotalFederalWithheld: 8000,
			PayPeriodsUploaded:   13,
			LatestYTDGross:       80000,
			LatestYTDFedWithheld: 8000,
			AvgGrossPerPeriod:    80000.0 / 13.0,
		},
		{
			Name:                 "Frank",
			TotalGrossPay:        30000,
			TotalFederalWithheld: 2000,
			PayPeriodsUploaded:   13,
			LatestYTDGross:       30000,
			LatestYTDFedWithheld: 2000,
			AvgGrossPerPeriod:    30000.0 / 13.0,
		},
	}

	result := withholding(schedule, earners, time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC))

	// Higher earner should be Eve.
	if result.HigherEarnerName != "Eve" {
		t.Errorf("HigherEarnerName = %q, want %q", result.HigherEarnerName, "Eve")
	}

	// There should be remaining pay periods.
	if result.RemainingPayPeriods <= 0 {
		t.Errorf("RemainingPayPeriods = %d, expected > 0", result.RemainingPayPeriods)
	}
}

func TestAdjustsRecommendationWithMorePayPeriods(t *testing.T) {
	schedule := tax.HardcodedBrackets(2025, tax.MarriedFilingJointly)
	if schedule == nil {
		t.Fatal("expected brackets")
	}

	makeEarners := func(periods int, totalGross, totalWithheld float64) []calc.EarnerSummary {
		return []calc.EarnerSummary{
			{
				Name:                 "Gina",
				TotalGrossPay:        totalGross,
				TotalFederalWithheld: totalWithheld,
				PayPeriodsUploaded:   periods,
				LatestYTDGross:       totalGross,
				LatestYTDFedWithheld: totalWithheld,
				AvgGrossPerPeriod:    totalGross / float64(periods),
			},
			{
				Name:                 "Hank",
				TotalGrossPay:        totalGross * 0.8,
				TotalFederalWithheld: totalWithheld * 0.8,
				PayPeriodsUploaded:   periods,
				LatestYTDGross:       totalGross * 0.8,
				LatestYTDFedWithheld: totalWithheld * 0.8,
				AvgGrossPerPeriod:    (totalGross * 0.8) / float64(periods),
			},
		}
	}

	// Mid-year: 13 periods uploaded.
	earlyResult := withholding(schedule, makeEarners(13, 65000, 6500), time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC))

	// Late year: 20 periods uploaded.
	lateResult := withholding(schedule, makeEarners(20, 100000, 10000), time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC))

	// With fewer remaining pay periods, the per-paycheck additional should be higher
	// (assuming similar under-withholding). The late result has fewer remaining periods
	// to spread the same total shortfall over.
	if lateResult.RemainingPayPeriods >= earlyResult.RemainingPayPeriods {
		t.Errorf("late remaining periods (%d) should be < early (%d)",
			lateResult.RemainingPayPeriods, earlyResult.RemainingPayPeriods)
	}
}

func TestSupplementalIncomeAffectsLiability(t *testing.T) {
	schedule := tax.HardcodedBrackets(2025, tax.Single)
	if schedule == nil {
		t.Fatal("expected brackets")
	}

	earners := []calc.EarnerSummary{
		{
			Name:                 "Iris",
			TotalGrossPay:        70000,
			TotalFederalWithheld: 10000,
			PayPeriodsUploaded:   26,
			LatestYTDGross:       70000,
			LatestYTDFedWithheld: 10000,
			AvgGrossPerPeriod:    70000.0 / 26.0,
		},
	}

	ref := time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC)
	withoutSupp := withholding(schedule, earners, ref)
	withSupp := calc.CalculateWithholding(calc.WithholdingInput{
		Schedule:               schedule,
		Earners:                earners,
		SupplementalIncome:     5000,
		TotalPayPeriodsPerYear: 26,
		ReferenceDate:          ref,
	})

	if withSupp.TotalTaxLiability <= withoutSupp.TotalTaxLiability {
		t.Error("supplemental income should increase tax liability")
	}
	if !almostEqual(withSupp.EstimatedAnnualIncome, withoutSupp.EstimatedAnnualIncome+5000, 1) {
		t.Errorf("EstimatedAnnualIncome with supplement = %v, expected %v + 5000",
			withSupp.EstimatedAnnualIncome, withoutSupp.EstimatedAnnualIncome)
	}
}

func TestZeroEarners(t *testing.T) {
	schedule := tax.HardcodedBrackets(2025, tax.Single)
	if schedule == nil {
		t.Fatal("expected brackets")
	}

	result := calc.CalculateWithholding(calc.WithholdingInput{
		Schedule:               schedule,
		TotalPayPeriodsPerYear: 26,
		ReferenceDate:          time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
	})
	if result.TotalTaxLiability != 0 {
		t.Errorf("TotalTaxLiability = %v, want 0 for no earners", result.TotalTaxLiability)
	}
}

func TestOverwithheld(t *testing.T) {
	schedule := tax.HardcodedBrackets(2025, tax.Single)
	if schedule == nil {
		t.Fatal("expected brackets")
	}

	earners := []calc.EarnerSummary{
		{
			Name:                 "Jack",
			TotalGrossPay:        50000,
			TotalFederalWithheld: 20000, // way over-withheld
			PayPeriodsUploaded:   26,
			LatestYTDGross:       50000,
			LatestYTDFedWithheld: 20000,
			AvgGrossPerPeriod:    50000.0 / 26.0,
		},
	}

	result := withholding(schedule, earners, time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC))

	// Should not recommend additional withholding.
	if result.AdditionalPerPaycheck > 0 {
		t.Errorf("should not recommend additional when over-withheld, got %v", result.AdditionalPerPaycheck)
	}
	if result.RemainingTaxOwed > 0 {
		t.Errorf("RemainingTaxOwed should be 0 when over-withheld, got %v", result.RemainingTaxOwed)
	}
}

func TestPreTaxDeductionsReduceTaxLiability(t *testing.T) {
	schedule := tax.HardcodedBrackets(2025, tax.MarriedFilingJointly)
	if schedule == nil {
		t.Fatal("expected brackets")
	}

	earners := []calc.EarnerSummary{
		{
			Name:                 "Kate",
			TotalGrossPay:        75000,
			TotalFederalWithheld: 8000,
			PayPeriodsUploaded:   26,
			LatestYTDGross:       75000,
			LatestYTDFedWithheld: 8000,
			AvgGrossPerPeriod:    75000.0 / 26.0,
		},
		{
			Name:                 "Leo",
			TotalGrossPay:        65000,
			TotalFederalWithheld: 7000,
			PayPeriodsUploaded:   26,
			LatestYTDGross:       65000,
			LatestYTDFedWithheld: 7000,
			AvgGrossPerPeriod:    65000.0 / 26.0,
		},
	}

	ref := time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC)

	withoutDeductions := withholding(schedule, earners, ref)
	withDeductions := calc.CalculateWithholding(calc.WithholdingInput{
		Schedule:               schedule,
		Earners:                earners,
		TotalPayPeriodsPerYear: 26,
		ReferenceDate:          ref,
		PreTaxDeductions:       30000,
	})

	if withDeductions.TotalTaxLiability >= withoutDeductions.TotalTaxLiability {
		t.Errorf("pre-tax deductions should reduce tax liability: without=%v, with=%v",
			withoutDeductions.TotalTaxLiability, withDeductions.TotalTaxLiability)
	}

	if withDeductions.PreTaxDeductions != 30000 {
		t.Errorf("PreTaxDeductions = %v, want 30000", withDeductions.PreTaxDeductions)
	}

	// Both should have the same estimated annual income (deductions don't change gross).
	if withDeductions.EstimatedAnnualIncome != withoutDeductions.EstimatedAnnualIncome {
		t.Errorf("EstimatedAnnualIncome should be same regardless of deductions: without=%v, with=%v",
			withoutDeductions.EstimatedAnnualIncome, withDeductions.EstimatedAnnualIncome)
	}
}

func TestProjectedRemainingOverridesAverageEstimate(t *testing.T) {
	schedule := tax.HardcodedBrackets(2025, tax.MarriedFilingJointly)
	if schedule == nil {
		t.Fatal("expected brackets")
	}

	// Earner with low average withholding (early paychecks had less withheld)
	// but whose current paycheck rate is higher.
	earners := []calc.EarnerSummary{
		{
			Name:                 "Mia",
			TotalGrossPay:        80000,
			TotalFederalWithheld: 6000, // low average: 6000/13 = ~461/period
			PayPeriodsUploaded:   13,
			LatestYTDGross:       80000,
			LatestYTDFedWithheld: 6000,
			AvgGrossPerPeriod:    80000.0 / 13.0,
		},
	}

	ref := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)

	// Without EOY projection: uses average-based estimate.
	withoutProjection := calc.CalculateWithholding(calc.WithholdingInput{
		Schedule:               schedule,
		Earners:                earners,
		TotalPayPeriodsPerYear: 26,
		ReferenceDate:          ref,
	})

	// With EOY projection: higher projected remaining (e.g., recent paychecks withhold more).
	withProjection := calc.CalculateWithholding(calc.WithholdingInput{
		Schedule:                      schedule,
		Earners:                       earners,
		TotalPayPeriodsPerYear:        26,
		ReferenceDate:                 ref,
		ProjectedRemainingWithholding: 10000, // higher than average would predict
	})

	// The projected version should recommend less additional withholding
	// since it expects more to come in naturally.
	if withProjection.AdditionalPerPaycheck >= withoutProjection.AdditionalPerPaycheck {
		t.Errorf("with EOY projection should recommend less additional: without=%v, with=%v",
			withoutProjection.AdditionalPerPaycheck, withProjection.AdditionalPerPaycheck)
	}
}
