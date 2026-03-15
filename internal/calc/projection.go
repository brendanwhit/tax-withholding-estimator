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

	// Sum withholding from individual stubs.
	var sumRegular, sumBonus float64
	for _, s := range regular {
		sumRegular += s.FederalTaxWithheld
	}
	for _, s := range bonuses {
		sumBonus += s.FederalTaxWithheld
	}
	ep.RegularWithheldToDate = sumRegular
	ep.BonusWithheldToDate = sumBonus
	ep.BonusCount = len(bonuses)

	// Use YTD federal tax withheld from the latest stub when available,
	// since it accounts for all pay periods (including ones not uploaded).
	latestStub := stubs[len(stubs)-1]
	if latestStub.YTDFederalTaxWithheld != nil && *latestStub.YTDFederalTaxWithheld >= sumRegular+sumBonus {
		ep.TotalWithheldToDate = *latestStub.YTDFederalTaxWithheld
		// Attribute the difference to regular withholding (unuploaded stubs).
		ep.RegularWithheldToDate = ep.TotalWithheldToDate - ep.BonusWithheldToDate
	} else {
		ep.TotalWithheldToDate = sumRegular + sumBonus
	}

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
// It uses pay period length (end - start) as the primary signal to distinguish
// weekly and monthly frequencies. For the ambiguous biweekly/semi-monthly range,
// it uses gaps between consecutive stubs when available. This handles
// non-consecutive uploads correctly (e.g., two biweekly stubs from different
// months won't be misclassified as monthly).
func InferPayFrequency(stubs []db.Paystub) PayFrequency {
	if len(stubs) < 1 {
		return FrequencyBiweekly // default assumption
	}

	// Compute pay period lengths (end - start) for each stub.
	var periodLengths []int
	for _, s := range stubs {
		days := int(s.PayPeriodEnd.Sub(s.PayPeriodStart).Hours() / 24)
		if days > 0 {
			periodLengths = append(periodLengths, days)
		}
	}

	// Compute gaps between consecutive pay period start dates.
	var gaps []int
	for i := 1; i < len(stubs); i++ {
		days := int(stubs[i].PayPeriodStart.Sub(stubs[i-1].PayPeriodStart).Hours() / 24)
		if days > 0 {
			gaps = append(gaps, days)
		}
	}

	// Use period length to identify clear weekly or monthly cases.
	if len(periodLengths) > 0 {
		sort.Ints(periodLengths)
		medianLength := periodLengths[len(periodLengths)/2]

		if medianLength <= 9 {
			return FrequencyWeekly
		}
		if medianLength > 20 {
			return FrequencyMonthly
		}

		// Period length is in the 10-20 day range (biweekly or semi-monthly).
		// Use gaps between consecutive stubs to disambiguate if available.
		if len(gaps) > 0 {
			sort.Ints(gaps)
			medianGap := gaps[len(gaps)/2]
			if medianGap <= 14 {
				return FrequencyBiweekly
			}
			if medianGap <= 20 {
				return FrequencySemiMonthly
			}
			// Gaps > 20 means stubs are non-consecutive; fall through to
			// period length heuristic below.
		}

		// No consecutive gaps available or gaps are too wide (non-consecutive uploads).
		// Use period length: biweekly periods are typically 13-14 days,
		// semi-monthly periods vary more (12-16 days with a median around 15).
		if medianLength <= 14 {
			return FrequencyBiweekly
		}
		return FrequencySemiMonthly
	}

	// No period length data; fall back to gaps only.
	if len(gaps) == 0 {
		return FrequencyBiweekly
	}

	sort.Ints(gaps)
	medianGap := gaps[len(gaps)/2]
	switch {
	case medianGap <= 9:
		return FrequencyWeekly
	case medianGap <= 14:
		return FrequencyBiweekly
	case medianGap <= 20:
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

// remainingPayPeriods calculates periods left in the year using the latest
// pay period end date rather than counting uploaded stubs, which would
// undercount if uploads are missed.
func remainingPayPeriods(refDate time.Time, taxYear int, periodsPerYear int, stubs []db.Paystub) int {
	if len(stubs) == 0 {
		return periodsRemainingFromDate(refDate, taxYear, periodsPerYear)
	}

	// Find the latest PayPeriodEnd across all stubs.
	latestEnd := stubs[0].PayPeriodEnd
	for _, s := range stubs[1:] {
		if s.PayPeriodEnd.After(latestEnd) {
			latestEnd = s.PayPeriodEnd
		}
	}

	return periodsRemainingFromDate(latestEnd, taxYear, periodsPerYear)
}
