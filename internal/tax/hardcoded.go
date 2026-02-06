package tax

// HardcodedBrackets returns the hardcoded federal tax brackets for the given
// year and filing status. Returns nil if not available.
// Source: IRS Revenue Procedure 2024-40 (2025) and projected 2026 brackets.
func HardcodedBrackets(year int, status FilingStatus) *BracketSchedule {
	schedules, ok := hardcodedData[year]
	if !ok {
		return nil
	}
	s, ok := schedules[status]
	if !ok {
		return nil
	}
	return &s
}

// AllFilingStatuses returns all supported filing statuses.
func AllFilingStatuses() []FilingStatus {
	return []FilingStatus{
		Single,
		MarriedFilingJointly,
		MarriedFilingSeparately,
		HeadOfHousehold,
	}
}

var hardcodedData = map[int]map[FilingStatus]BracketSchedule{
	2025: {
		Single: {
			TaxYear:           2025,
			FilingStatus:      Single,
			StandardDeduction: 15000,
			Brackets: []Bracket{
				{Min: 0, Max: 11925, Rate: 0.10},
				{Min: 11925, Max: 48475, Rate: 0.12},
				{Min: 48475, Max: 103350, Rate: 0.22},
				{Min: 103350, Max: 197300, Rate: 0.24},
				{Min: 197300, Max: 250525, Rate: 0.32},
				{Min: 250525, Max: 626350, Rate: 0.35},
				{Min: 626350, Max: 0, Rate: 0.37},
			},
		},
		MarriedFilingJointly: {
			TaxYear:           2025,
			FilingStatus:      MarriedFilingJointly,
			StandardDeduction: 30000,
			Brackets: []Bracket{
				{Min: 0, Max: 23850, Rate: 0.10},
				{Min: 23850, Max: 96950, Rate: 0.12},
				{Min: 96950, Max: 206700, Rate: 0.22},
				{Min: 206700, Max: 394600, Rate: 0.24},
				{Min: 394600, Max: 501050, Rate: 0.32},
				{Min: 501050, Max: 751600, Rate: 0.35},
				{Min: 751600, Max: 0, Rate: 0.37},
			},
		},
		MarriedFilingSeparately: {
			TaxYear:           2025,
			FilingStatus:      MarriedFilingSeparately,
			StandardDeduction: 15000,
			Brackets: []Bracket{
				{Min: 0, Max: 11925, Rate: 0.10},
				{Min: 11925, Max: 48475, Rate: 0.12},
				{Min: 48475, Max: 103350, Rate: 0.22},
				{Min: 103350, Max: 197300, Rate: 0.24},
				{Min: 197300, Max: 250525, Rate: 0.32},
				{Min: 250525, Max: 375800, Rate: 0.35},
				{Min: 375800, Max: 0, Rate: 0.37},
			},
		},
		HeadOfHousehold: {
			TaxYear:           2025,
			FilingStatus:      HeadOfHousehold,
			StandardDeduction: 22500,
			Brackets: []Bracket{
				{Min: 0, Max: 17000, Rate: 0.10},
				{Min: 17000, Max: 64850, Rate: 0.12},
				{Min: 64850, Max: 103350, Rate: 0.22},
				{Min: 103350, Max: 197300, Rate: 0.24},
				{Min: 197300, Max: 250500, Rate: 0.32},
				{Min: 250500, Max: 626350, Rate: 0.35},
				{Min: 626350, Max: 0, Rate: 0.37},
			},
		},
	},
	2026: {
		Single: {
			TaxYear:           2026,
			FilingStatus:      Single,
			StandardDeduction: 15300,
			Brackets: []Bracket{
				{Min: 0, Max: 12150, Rate: 0.10},
				{Min: 12150, Max: 49475, Rate: 0.12},
				{Min: 49475, Max: 105400, Rate: 0.22},
				{Min: 105400, Max: 201150, Rate: 0.24},
				{Min: 201150, Max: 255550, Rate: 0.32},
				{Min: 255550, Max: 639200, Rate: 0.35},
				{Min: 639200, Max: 0, Rate: 0.37},
			},
		},
		MarriedFilingJointly: {
			TaxYear:           2026,
			FilingStatus:      MarriedFilingJointly,
			StandardDeduction: 30600,
			Brackets: []Bracket{
				{Min: 0, Max: 24300, Rate: 0.10},
				{Min: 24300, Max: 98950, Rate: 0.12},
				{Min: 98950, Max: 210800, Rate: 0.22},
				{Min: 210800, Max: 402300, Rate: 0.24},
				{Min: 402300, Max: 511050, Rate: 0.32},
				{Min: 511050, Max: 766550, Rate: 0.35},
				{Min: 766550, Max: 0, Rate: 0.37},
			},
		},
		MarriedFilingSeparately: {
			TaxYear:           2026,
			FilingStatus:      MarriedFilingSeparately,
			StandardDeduction: 15300,
			Brackets: []Bracket{
				{Min: 0, Max: 12150, Rate: 0.10},
				{Min: 12150, Max: 49475, Rate: 0.12},
				{Min: 49475, Max: 105400, Rate: 0.22},
				{Min: 105400, Max: 201150, Rate: 0.24},
				{Min: 201150, Max: 255550, Rate: 0.32},
				{Min: 255550, Max: 383275, Rate: 0.35},
				{Min: 383275, Max: 0, Rate: 0.37},
			},
		},
		HeadOfHousehold: {
			TaxYear:           2026,
			FilingStatus:      HeadOfHousehold,
			StandardDeduction: 22950,
			Brackets: []Bracket{
				{Min: 0, Max: 17350, Rate: 0.10},
				{Min: 17350, Max: 66150, Rate: 0.12},
				{Min: 66150, Max: 105400, Rate: 0.22},
				{Min: 105400, Max: 201150, Rate: 0.24},
				{Min: 201150, Max: 255550, Rate: 0.32},
				{Min: 255550, Max: 639200, Rate: 0.35},
				{Min: 639200, Max: 0, Rate: 0.37},
			},
		},
	},
}
