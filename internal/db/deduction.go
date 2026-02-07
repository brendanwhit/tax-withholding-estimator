package db

import (
	"fmt"
)

// PreTaxDeduction represents a single pre-tax deduction from a paystub.
type PreTaxDeduction struct {
	ID            int64
	PaystubID     int64
	DeductionType string
	Amount        float64
	YTDAmount     *float64
}

// SavePreTaxDeduction inserts or updates a pre-tax deduction for a paystub.
func (s *Store) SavePreTaxDeduction(d *PreTaxDeduction) error {
	_, err := s.DB.Exec(`INSERT INTO pre_tax_deductions (paystub_id, deduction_type, amount, ytd_amount)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(paystub_id, deduction_type) DO UPDATE SET
			amount = excluded.amount,
			ytd_amount = excluded.ytd_amount`,
		d.PaystubID, d.DeductionType, d.Amount, d.YTDAmount)
	if err != nil {
		return fmt.Errorf("save deduction: %w", err)
	}
	return nil
}

// DeductionYTDSummary holds the YTD total for a deduction type for a person.
type DeductionYTDSummary struct {
	DeductionType string
	PersonName    string
	TotalAmount   float64 // sum of per-period amounts
	LatestYTD     float64 // YTD from most recent paystub (if available)
}

// GetDeductionSummaryByYear returns YTD deduction totals per person per type for a tax year.
func (s *Store) GetDeductionSummaryByYear(year int) ([]DeductionYTDSummary, error) {
	rows, err := s.DB.Query(`
		SELECT p.person_name, d.deduction_type, SUM(d.amount) as total_amount
		FROM pre_tax_deductions d
		JOIN paystubs p ON p.id = d.paystub_id
		WHERE p.tax_year = ?
		GROUP BY p.person_name, d.deduction_type
		ORDER BY p.person_name, d.deduction_type`, year)
	if err != nil {
		return nil, fmt.Errorf("query deduction summary: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var summaries []DeductionYTDSummary
	for rows.Next() {
		var s DeductionYTDSummary
		if err := rows.Scan(&s.PersonName, &s.DeductionType, &s.TotalAmount); err != nil {
			return nil, fmt.Errorf("scan deduction summary: %w", err)
		}

		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}

// GetTotalPreTaxDeductionsByYear returns the total pre-tax deductions for a tax year.
func (s *Store) GetTotalPreTaxDeductionsByYear(year int) (float64, error) {
	var total float64
	err := s.DB.QueryRow(`
		SELECT COALESCE(SUM(d.amount), 0)
		FROM pre_tax_deductions d
		JOIN paystubs p ON p.id = d.paystub_id
		WHERE p.tax_year = ?`, year).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("query total deductions: %w", err)
	}
	return total, nil
}

// ContributionLimit represents an IRS annual limit for a deduction type.
type ContributionLimit struct {
	TaxYear       int
	DeductionType string
	AnnualLimit   float64
	CatchUpLimit  float64
	Description   string
}

// SaveContributionLimits saves IRS contribution limits for a year.
func (s *Store) SaveContributionLimits(limits []ContributionLimit) error {
	for _, l := range limits {
		_, err := s.DB.Exec(`INSERT INTO contribution_limits (tax_year, deduction_type, annual_limit, catch_up_limit, description)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(tax_year, deduction_type) DO UPDATE SET
				annual_limit = excluded.annual_limit,
				catch_up_limit = excluded.catch_up_limit,
				description = excluded.description`,
			l.TaxYear, l.DeductionType, l.AnnualLimit, l.CatchUpLimit, l.Description)
		if err != nil {
			return fmt.Errorf("save contribution limit: %w", err)
		}
	}
	return nil
}

// GetContributionLimits returns all contribution limits for a year.
func (s *Store) GetContributionLimits(year int) ([]ContributionLimit, error) {
	rows, err := s.DB.Query(`SELECT tax_year, deduction_type, annual_limit, COALESCE(catch_up_limit, 0), COALESCE(description, '')
		FROM contribution_limits WHERE tax_year = ?
		ORDER BY deduction_type`, year)
	if err != nil {
		return nil, fmt.Errorf("query contribution limits: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var limits []ContributionLimit
	for rows.Next() {
		var l ContributionLimit
		if err := rows.Scan(&l.TaxYear, &l.DeductionType, &l.AnnualLimit, &l.CatchUpLimit, &l.Description); err != nil {
			return nil, fmt.Errorf("scan contribution limit: %w", err)
		}
		limits = append(limits, l)
	}
	return limits, rows.Err()
}
