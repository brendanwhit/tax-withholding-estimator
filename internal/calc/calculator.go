package calc

import (
	"math"
	"time"

	"github.com/brendanwhit/tax-withholding-estimator/internal/tax"
)

// EarnerSummary holds aggregated paystub data for one earner.
type EarnerSummary struct {
	Name                  string
	TotalGrossPay         float64
	TotalFederalWithheld  float64
	PayPeriodsUploaded    int
	LatestYTDGross        float64
	LatestYTDFedWithheld  float64
	AvgGrossPerPeriod     float64
	LatestPayPeriodEnd    time.Time
}

// WithholdingResult holds the full recommendation.
type WithholdingResult struct {
	TaxYear               int
	FilingStatus          tax.FilingStatus
	CombinedGrossIncome   float64
	EstimatedAnnualIncome float64
	TotalTaxLiability     float64
	TotalWithheldToDate   float64
	RemainingTaxOwed      float64
	RemainingPayPeriods   int
	AdditionalPerPaycheck float64
	HigherEarnerName      string
	Earners               []EarnerSummary
	SupplementalIncome    float64
	PreTaxDeductions      float64
}

// WithholdingInput holds all inputs for the withholding calculation.
type WithholdingInput struct {
	Schedule               *tax.BracketSchedule
	Earners                []EarnerSummary
	SupplementalIncome     float64
	TotalPayPeriodsPerYear int
	ReferenceDate          time.Time
	PreTaxDeductions       float64
	// ProjectedRemainingWithholding, if > 0, overrides the internal average-based
	// estimate with the EOY projection's number for consistency.
	ProjectedRemainingWithholding float64
}

// CalculateWithholding computes the withholding recommendation.
func CalculateWithholding(in WithholdingInput) *WithholdingResult {
	schedule := in.Schedule
	earners := in.Earners
	totalPayPeriodsPerYear := in.TotalPayPeriodsPerYear
	referenceDate := in.ReferenceDate
	if totalPayPeriodsPerYear <= 0 {
		totalPayPeriodsPerYear = 26 // default biweekly
	}

	result := &WithholdingResult{
		TaxYear:            schedule.TaxYear,
		FilingStatus:       schedule.FilingStatus,
		Earners:            earners,
		SupplementalIncome: in.SupplementalIncome,
		PreTaxDeductions:   in.PreTaxDeductions,
	}

	// Find higher earner and calculate totals.
	var higherIdx int
	for i, e := range earners {
		ytdGross := e.LatestYTDGross
		if ytdGross == 0 {
			ytdGross = e.TotalGrossPay
		}
		ytdWithheld := e.LatestYTDFedWithheld
		if ytdWithheld == 0 {
			ytdWithheld = e.TotalFederalWithheld
		}

		result.CombinedGrossIncome += ytdGross
		result.TotalWithheldToDate += ytdWithheld

		if ytdGross > earners[higherIdx].LatestYTDGross {
			higherIdx = i
		}
	}
	if len(earners) > 0 {
		result.HigherEarnerName = earners[higherIdx].Name
	}

	// Estimate annual income by projecting from current data.
	result.EstimatedAnnualIncome = estimateAnnualIncome(earners, totalPayPeriodsPerYear) + in.SupplementalIncome

	// Calculate total tax liability on estimated income minus pre-tax deductions.
	taxableIncome := result.EstimatedAnnualIncome - in.PreTaxDeductions
	if taxableIncome < 0 {
		taxableIncome = 0
	}
	result.TotalTaxLiability = schedule.CalculateTax(taxableIncome)

	// Remaining tax owed.
	result.RemainingTaxOwed = result.TotalTaxLiability - result.TotalWithheldToDate
	if result.RemainingTaxOwed < 0 {
		result.RemainingTaxOwed = 0
	}

	// Determine remaining pay periods in the year.
	result.RemainingPayPeriods = estimateRemainingPayPeriods(
		referenceDate, schedule.TaxYear, totalPayPeriodsPerYear, earners,
	)

	// Additional withholding per paycheck for the higher earner.
	if result.RemainingPayPeriods > 0 && result.RemainingTaxOwed > 0 {
		// Use EOY projection if provided, otherwise fall back to average-based estimate.
		normalWithholdingRemaining := in.ProjectedRemainingWithholding
		if normalWithholdingRemaining == 0 {
			normalWithholdingRemaining = estimateNormalWithholdingRemaining(earners, result.RemainingPayPeriods, totalPayPeriodsPerYear)
		}
		gap := result.TotalTaxLiability - result.TotalWithheldToDate - normalWithholdingRemaining
		if gap > 0 {
			result.AdditionalPerPaycheck = gap / float64(result.RemainingPayPeriods)
		}
	}

	return result
}

// estimateAnnualIncome projects annual income from YTD data and pay frequency.
func estimateAnnualIncome(earners []EarnerSummary, totalPayPeriods int) float64 {
	var total float64
	for _, e := range earners {
		if e.AvgGrossPerPeriod > 0 {
			total += e.AvgGrossPerPeriod * float64(totalPayPeriods)
		} else if e.PayPeriodsUploaded > 0 {
			total += (e.TotalGrossPay / float64(e.PayPeriodsUploaded)) * float64(totalPayPeriods)
		} else if e.LatestYTDGross > 0 {
			total += e.LatestYTDGross * 2 // rough estimate if minimal data
		}
	}
	return total
}

// estimateRemainingPayPeriods calculates how many pay periods are left in the year.
// It uses the latest pay period end date across earners to determine elapsed periods
// rather than counting uploaded stubs, which would undercount if uploads are missed.
func estimateRemainingPayPeriods(refDate time.Time, taxYear int, totalPerYear int, earners []EarnerSummary) int {
	// Find the latest pay period end date across all earners.
	var latestEnd time.Time
	for _, e := range earners {
		if !e.LatestPayPeriodEnd.IsZero() && e.LatestPayPeriodEnd.After(latestEnd) {
			latestEnd = e.LatestPayPeriodEnd
		}
	}

	if !latestEnd.IsZero() {
		return periodsRemainingFromDate(latestEnd, taxYear, totalPerYear)
	}

	// Fallback: estimate from calendar position using refDate.
	return periodsRemainingFromDate(refDate, taxYear, totalPerYear)
}

// periodsRemainingFromDate calculates remaining pay periods from a date within the tax year.
func periodsRemainingFromDate(date time.Time, taxYear int, totalPerYear int) int {
	yearStart := time.Date(taxYear, 1, 1, 0, 0, 0, 0, time.UTC)
	daysElapsed := date.Sub(yearStart).Hours() / 24
	if daysElapsed < 0 {
		return totalPerYear
	}
	periodLengthDays := 365.0 / float64(totalPerYear)
	elapsed := int(math.Round(daysElapsed / periodLengthDays))
	remaining := totalPerYear - elapsed
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}

// estimateNormalWithholdingRemaining estimates what will be withheld in remaining periods
// based on the current average withholding rate.
func estimateNormalWithholdingRemaining(earners []EarnerSummary, remainingPeriods int, totalPerYear int) float64 {
	var total float64
	for _, e := range earners {
		var avgWithholdPerPeriod float64
		if e.PayPeriodsUploaded > 0 {
			avgWithholdPerPeriod = e.TotalFederalWithheld / float64(e.PayPeriodsUploaded)
		} else if e.LatestYTDFedWithheld > 0 && totalPerYear > 0 {
			avgWithholdPerPeriod = e.LatestYTDFedWithheld / float64(totalPerYear/2) // rough
		}
		total += avgWithholdPerPeriod * float64(remainingPeriods)
	}
	return total
}
