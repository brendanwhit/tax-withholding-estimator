package tax

// FilingStatus represents IRS filing status.
type FilingStatus string

const (
	Single                 FilingStatus = "single"
	MarriedFilingJointly   FilingStatus = "married_filing_jointly"
	MarriedFilingSeparately FilingStatus = "married_filing_separately"
	HeadOfHousehold        FilingStatus = "head_of_household"
)

// Bracket represents a single tax bracket.
type Bracket struct {
	Min  float64 // lower bound of taxable income
	Max  float64 // upper bound (0 means no limit)
	Rate float64 // marginal tax rate (e.g., 0.10 for 10%)
}

// BracketSchedule holds the brackets and standard deduction for a filing status.
type BracketSchedule struct {
	TaxYear           int
	FilingStatus      FilingStatus
	Brackets          []Bracket
	StandardDeduction float64
}

// CalculateTax computes the federal income tax on the given taxable income
// using the bracket schedule. It applies the standard deduction first.
func (s *BracketSchedule) CalculateTax(grossIncome float64) float64 {
	taxableIncome := grossIncome - s.StandardDeduction
	if taxableIncome <= 0 {
		return 0
	}

	var totalTax float64
	for _, b := range s.Brackets {
		if taxableIncome <= 0 {
			break
		}

		var bracketWidth float64
		if b.Max == 0 {
			bracketWidth = taxableIncome
		} else {
			bracketWidth = b.Max - b.Min
		}

		taxableInBracket := taxableIncome
		if taxableInBracket > bracketWidth {
			taxableInBracket = bracketWidth
		}

		totalTax += taxableInBracket * b.Rate
		taxableIncome -= taxableInBracket
	}

	return totalTax
}
