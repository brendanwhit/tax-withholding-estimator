package calc

import (
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
}

// CalculateWithholding computes the withholding recommendation.
func CalculateWithholding(
	schedule *tax.BracketSchedule,
	earners []EarnerSummary,
	supplementalIncome float64,
	totalPayPeriodsPerYear int,
	referenceDate time.Time,
) *WithholdingResult {
	if totalPayPeriodsPerYear <= 0 {
		totalPayPeriodsPerYear = 26 // default biweekly
	}

	result := &WithholdingResult{
		TaxYear:            schedule.TaxYear,
		FilingStatus:       schedule.FilingStatus,
		Earners:            earners,
		SupplementalIncome: supplementalIncome,
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
	result.EstimatedAnnualIncome = estimateAnnualIncome(earners, totalPayPeriodsPerYear) + supplementalIncome

	// Calculate total tax liability on the estimated annual income.
	result.TotalTaxLiability = schedule.CalculateTax(result.EstimatedAnnualIncome)

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
		// Estimate what will be withheld normally in remaining periods.
		normalWithholdingRemaining := estimateNormalWithholdingRemaining(earners, result.RemainingPayPeriods, totalPayPeriodsPerYear)
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
func estimateRemainingPayPeriods(refDate time.Time, taxYear int, totalPerYear int, earners []EarnerSummary) int {
	// Count how many periods have been uploaded so far across earners.
	maxUploaded := 0
	for _, e := range earners {
		if e.PayPeriodsUploaded > maxUploaded {
			maxUploaded = e.PayPeriodsUploaded
		}
	}

	if maxUploaded > 0 {
		remaining := totalPerYear - maxUploaded
		if remaining < 0 {
			remaining = 0
		}
		return remaining
	}

	// Fallback: estimate from calendar position.
	yearStart := time.Date(taxYear, 1, 1, 0, 0, 0, 0, time.UTC)
	yearEnd := time.Date(taxYear, 12, 31, 0, 0, 0, 0, time.UTC)
	totalDays := yearEnd.Sub(yearStart).Hours() / 24
	elapsedDays := refDate.Sub(yearStart).Hours() / 24
	if elapsedDays < 0 {
		elapsedDays = 0
	}
	fractionRemaining := 1.0 - (elapsedDays / totalDays)
	if fractionRemaining < 0 {
		fractionRemaining = 0
	}
	return int(fractionRemaining * float64(totalPerYear))
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
