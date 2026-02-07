package db

// HardcodedContributionLimits returns the hardcoded IRS contribution limits for a given year.
// Returns nil if not available. Source: IRS guidelines.
func HardcodedContributionLimits(year int) []ContributionLimit {
	limits, ok := hardcodedLimits[year]
	if !ok {
		return nil
	}
	return limits
}

// GetOrCacheContributionLimits loads limits from DB, falling back to hardcoded and caching.
func (s *Store) GetOrCacheContributionLimits(year int) ([]ContributionLimit, error) {
	limits, err := s.GetContributionLimits(year)
	if err != nil {
		return nil, err
	}
	if len(limits) > 0 {
		return limits, nil
	}

	// Fall back to hardcoded.
	limits = HardcodedContributionLimits(year)
	if len(limits) == 0 {
		return nil, nil
	}

	// Cache in DB.
	if err := s.SaveContributionLimits(limits); err != nil {
		return nil, err
	}
	return limits, nil
}

var hardcodedLimits = map[int][]ContributionLimit{
	2025: {
		{TaxYear: 2025, DeductionType: "401k", AnnualLimit: 23500, CatchUpLimit: 7500, Description: "401(k) elective deferral"},
		{TaxYear: 2025, DeductionType: "403b", AnnualLimit: 23500, CatchUpLimit: 7500, Description: "403(b) elective deferral"},
		{TaxYear: 2025, DeductionType: "hsa", AnnualLimit: 4300, CatchUpLimit: 1000, Description: "HSA individual contribution"},
		{TaxYear: 2025, DeductionType: "hsa_family", AnnualLimit: 8550, CatchUpLimit: 1000, Description: "HSA family contribution"},
		{TaxYear: 2025, DeductionType: "fsa_health", AnnualLimit: 3300, Description: "Healthcare FSA"},
		{TaxYear: 2025, DeductionType: "fsa_dependent", AnnualLimit: 5000, Description: "Dependent care FSA"},
		{TaxYear: 2025, DeductionType: "commuter", AnnualLimit: 3600, Description: "Commuter transit/parking (per month $300 x 12)"},
	},
	2026: {
		{TaxYear: 2026, DeductionType: "401k", AnnualLimit: 23500, CatchUpLimit: 7500, Description: "401(k) elective deferral"},
		{TaxYear: 2026, DeductionType: "403b", AnnualLimit: 23500, CatchUpLimit: 7500, Description: "403(b) elective deferral"},
		{TaxYear: 2026, DeductionType: "hsa", AnnualLimit: 4400, CatchUpLimit: 1000, Description: "HSA individual contribution"},
		{TaxYear: 2026, DeductionType: "hsa_family", AnnualLimit: 8750, CatchUpLimit: 1000, Description: "HSA family contribution"},
		{TaxYear: 2026, DeductionType: "fsa_health", AnnualLimit: 3400, Description: "Healthcare FSA"},
		{TaxYear: 2026, DeductionType: "fsa_dependent", AnnualLimit: 5000, Description: "Dependent care FSA"},
		{TaxYear: 2026, DeductionType: "commuter", AnnualLimit: 3720, Description: "Commuter transit/parking (per month $310 x 12)"},
	},
}
