package calc

import (
	"math"
	"sort"
	"time"

	"github.com/brendanwhit/tax-withholding-estimator/internal/db"
)

// PayFrequency represents how often an employee is paid.
type PayFrequency int

const (
	FrequencyUnknown    PayFrequency = 0
	FrequencyWeekly     PayFrequency = 52
	FrequencyBiweekly   PayFrequency = 26
	FrequencySemiMonthly PayFrequency = 24
	FrequencyMonthly    PayFrequency = 12
)

func (f PayFrequency) String() string {
	switch f {
	case FrequencyWeekly:
		return "Weekly"
	case FrequencyBiweekly:
		return "Bi-weekly"
	case FrequencySemiMonthly:
		return "Semi-monthly"
	case FrequencyMonthly:
		return "Monthly"
	default:
		return "Unknown"
	}
}

// EarnerProjection holds the EOY projection for a single person.
type EarnerProjection struct {
	Name                  string
	PayFrequency          PayFrequency
	RegularWithheldToDate float64
	BonusWithheldToDate   float64
	TotalWithheldToDate   float64
	ProjectedRemaining    float64
	ProjectedEOYTotal     float64
	BonusCount            int
	RemainingPayPeriods   int
}

// EOYProjection holds the combined household EOY estimate.
type EOYProjection struct {
	TaxYear             int
	Earners             []EarnerProjection
	CombinedWithheld    float64
	CombinedProjected   float64
	CombinedEOYEstimate float64
}

// ProjectEOYWithholding calculates end-of-year projected withholding
// per person and combined.
func ProjectEOYWithholding(paystubsByPerson map[string][]db.Paystub, referenceDate time.Time, taxYear int) *EOYProjection {
	proj := &EOYProjection{TaxYear: taxYear}

	for name, stubs := range paystubsByPerson {
		ep := projectForPerson(name, stubs, referenceDate, taxYear)
		proj.Earners = append(proj.Earners, ep)
		proj.CombinedWithheld += ep.TotalWithheldToDate
		proj.CombinedProjected += ep.ProjectedRemaining
	}

	// Sort earners by name for consistent ordering.
	sort.Slice(proj.Earners, func(i, j int) bool {
		return proj.Earners[i].Name < proj.Earners[j].Name
	})

	proj.CombinedEOYEstimate = proj.CombinedWithheld + proj.CombinedProjected
	return proj
}

func projectForPerson(name string, stubs []db.Paystub, refDate time.Time, taxYear int) EarnerProjection {
	ep := EarnerProjection{Name: name}

	if len(stubs) == 0 {
		return ep
	}

	// Sort by pay period start date.
	sort.Slice(stubs, func(i, j int) bool {
		return stubs[i].PayPeriodStart.Before(stubs[j].PayPeriodStart)
	})

	// Detect pay frequency from regular paystubs.
	ep.PayFrequency = InferPayFrequency(stubs)

	// Classify each paystub as regular or bonus.
	regular, bonuses := classifyPaystubs(stubs, ep.PayFrequency)

	// Sum withholding.
	for _, s := range regular {
		ep.RegularWithheldToDate += s.FederalTaxWithheld
	}
	for _, s := range bonuses {
		ep.BonusWithheldToDate += s.FederalTaxWithheld
	}
	ep.TotalWithheldToDate = ep.RegularWithheldToDate + ep.BonusWithheldToDate
	ep.BonusCount = len(bonuses)

	// Project remaining withholding from the most recent regular paycheck.
	if len(regular) > 0 && ep.PayFrequency != FrequencyUnknown {
		lastRegular := regular[len(regular)-1]
		periodsPerYear := int(ep.PayFrequency)
		remaining := remainingPayPeriods(refDate, taxYear, periodsPerYear, stubs)
		ep.RemainingPayPeriods = remaining
		ep.ProjectedRemaining = lastRegular.FederalTaxWithheld * float64(remaining)
	}

	ep.ProjectedEOYTotal = ep.TotalWithheldToDate + ep.ProjectedRemaining
	return ep
}

// InferPayFrequency determines pay frequency from a sorted slice of paystubs.
func InferPayFrequency(stubs []db.Paystub) PayFrequency {
	if len(stubs) < 2 {
		return FrequencyBiweekly // default assumption
	}

	// Calculate gaps between consecutive pay period start dates.
	var gaps []int
	for i := 1; i < len(stubs); i++ {
		days := int(stubs[i].PayPeriodStart.Sub(stubs[i-1].PayPeriodStart).Hours() / 24)
		if days > 0 {
			gaps = append(gaps, days)
		}
	}

	if len(gaps) == 0 {
		return FrequencyBiweekly
	}

	// Use median gap to be robust to outliers (bonuses).
	sort.Ints(gaps)
	median := gaps[len(gaps)/2]

	switch {
	case median <= 9:
		return FrequencyWeekly
	case median <= 14:
		return FrequencyBiweekly
	case median <= 20:
		return FrequencySemiMonthly
	default:
		return FrequencyMonthly
	}
}

// classifyPaystubs separates regular paychecks from bonuses using multiple signals.
func classifyPaystubs(stubs []db.Paystub, freq PayFrequency) (regular, bonuses []db.Paystub) {
	if len(stubs) <= 1 {
		return stubs, nil
	}

	// Calculate median gross pay for salaried baseline.
	grossPays := make([]float64, len(stubs))
	for i, s := range stubs {
		grossPays[i] = s.GrossPay
	}
	sort.Float64s(grossPays)
	medianGross := grossPays[len(grossPays)/2]

	// Calculate expected period gap.
	var expectedGapDays int
	switch freq {
	case FrequencyWeekly:
		expectedGapDays = 7
	case FrequencyBiweekly:
		expectedGapDays = 14
	case FrequencySemiMonthly:
		expectedGapDays = 15
	case FrequencyMonthly:
		expectedGapDays = 30
	default:
		expectedGapDays = 14
	}

	// Determine expected pay period length.
	var expectedPeriodDays int
	switch freq {
	case FrequencyWeekly:
		expectedPeriodDays = 7
	case FrequencyBiweekly:
		expectedPeriodDays = 14
	case FrequencySemiMonthly:
		expectedPeriodDays = 15
	case FrequencyMonthly:
		expectedPeriodDays = 30
	default:
		expectedPeriodDays = 14
	}

	for i, s := range stubs {
		bonusScore := 0

		// Signal 1: Anomalous gross pay (>2x or <0.5x median).
		if medianGross > 0 {
			ratio := s.GrossPay / medianGross
			if ratio > 2.0 || ratio < 0.3 {
				bonusScore++
			}
		}

		// Signal 2: Off-cycle date (not on regular schedule).
		if i > 0 {
			daysSincePrev := int(s.PayPeriodStart.Sub(stubs[i-1].PayPeriodStart).Hours() / 24)
			if daysSincePrev > 0 {
				deviation := math.Abs(float64(daysSincePrev - expectedGapDays))
				if deviation > float64(expectedGapDays)*0.4 {
					bonusScore++
				}
			}
		}

		// Signal 3: Different withholding pattern.
		// Supplemental rate is flat 22%. Check if effective rate is close to 22%.
		if s.GrossPay > 0 {
			effectiveRate := s.FederalTaxWithheld / s.GrossPay
			if math.Abs(effectiveRate-0.22) < 0.02 && medianGross > 0 {
				// Only flag if the amount is also anomalous (>1.5x median).
				if s.GrossPay/medianGross > 1.5 {
					bonusScore++
				}
			}
		}

		// Signal 4: Unusually short or no pay period (e.g., single-day bonus).
		periodDays := int(s.PayPeriodEnd.Sub(s.PayPeriodStart).Hours() / 24)
		if periodDays < expectedPeriodDays/2 {
			bonusScore++
		}

		if bonusScore >= 2 {
			bonuses = append(bonuses, s)
		} else {
			regular = append(regular, s)
		}
	}

	return regular, bonuses
}

// remainingPayPeriods calculates periods left in the year from the reference date.
func remainingPayPeriods(refDate time.Time, taxYear int, periodsPerYear int, stubs []db.Paystub) int {
	// Count distinct periods already received.
	periodsElapsed := len(stubs)

	remaining := periodsPerYear - periodsElapsed
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}
