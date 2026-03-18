package db

import (
	"database/sql"
	"fmt"
	"time"
)

// Paystub represents a stored paystub record.
type Paystub struct {
	ID                    int64
	PersonName            string
	TaxYear               int
	PayPeriodStart        time.Time
	PayPeriodEnd          time.Time
	GrossPay              float64
	FederalTaxWithheld    float64
	YTDGrossPay           *float64
	YTDFederalTaxWithheld *float64
	Hours                 *float64
	YTDHours              *float64
	CreatedAt             time.Time
}

// SavePaystub inserts or updates a paystub record.
// Duplicate detection is based on (person_name, pay_period_start, pay_period_end).
// On conflict, the existing record is updated with the new values.
func (s *Store) SavePaystub(p *Paystub) (int64, error) {
	result, err := s.DB.Exec(`INSERT INTO paystubs
		(person_name, tax_year, pay_period_start, pay_period_end, gross_pay,
		 federal_tax_withheld, ytd_gross_pay, ytd_federal_tax_withheld, hours, ytd_hours)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(person_name, pay_period_start, pay_period_end) DO UPDATE SET
			gross_pay = excluded.gross_pay,
			federal_tax_withheld = excluded.federal_tax_withheld,
			ytd_gross_pay = excluded.ytd_gross_pay,
			ytd_federal_tax_withheld = excluded.ytd_federal_tax_withheld,
			hours = excluded.hours,
			ytd_hours = excluded.ytd_hours`,
		p.PersonName, p.TaxYear,
		p.PayPeriodStart.Format("2006-01-02"), p.PayPeriodEnd.Format("2006-01-02"),
		p.GrossPay, p.FederalTaxWithheld, p.YTDGrossPay, p.YTDFederalTaxWithheld,
		p.Hours, p.YTDHours)
	if err != nil {
		return 0, fmt.Errorf("save paystub: %w", err)
	}
	return result.LastInsertId()
}

// GetPaystubsByPersonAndYear returns all paystubs for a person in a given tax year,
// ordered by pay period start date.
func (s *Store) GetPaystubsByPersonAndYear(name string, year int) ([]Paystub, error) {
	rows, err := s.DB.Query(`SELECT id, person_name, tax_year, pay_period_start, pay_period_end,
		gross_pay, federal_tax_withheld, ytd_gross_pay, ytd_federal_tax_withheld, hours, ytd_hours, created_at
		FROM paystubs WHERE person_name = ? AND tax_year = ?
		ORDER BY pay_period_start ASC`, name, year)
	if err != nil {
		return nil, fmt.Errorf("query paystubs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanPaystubs(rows)
}

// GetAllPaystubsByYear returns all paystubs for a given tax year.
func (s *Store) GetAllPaystubsByYear(year int) ([]Paystub, error) {
	rows, err := s.DB.Query(`SELECT id, person_name, tax_year, pay_period_start, pay_period_end,
		gross_pay, federal_tax_withheld, ytd_gross_pay, ytd_federal_tax_withheld, hours, ytd_hours, created_at
		FROM paystubs WHERE tax_year = ?
		ORDER BY person_name ASC, pay_period_start ASC`, year)
	if err != nil {
		return nil, fmt.Errorf("query paystubs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanPaystubs(rows)
}

func scanPaystubs(rows *sql.Rows) ([]Paystub, error) {
	var paystubs []Paystub
	for rows.Next() {
		var p Paystub
		var start, end, created string
		if err := rows.Scan(&p.ID, &p.PersonName, &p.TaxYear, &start, &end,
			&p.GrossPay, &p.FederalTaxWithheld, &p.YTDGrossPay, &p.YTDFederalTaxWithheld,
			&p.Hours, &p.YTDHours, &created); err != nil {
			return nil, fmt.Errorf("scan paystub: %w", err)
		}
		var err error
		p.PayPeriodStart, err = parseFlexDate(start)
		if err != nil {
			return nil, fmt.Errorf("parse start date: %w", err)
		}
		p.PayPeriodEnd, err = parseFlexDate(end)
		if err != nil {
			return nil, fmt.Errorf("parse end date: %w", err)
		}
		p.CreatedAt, _ = parseFlexDate(created)
		paystubs = append(paystubs, p)
	}
	return paystubs, rows.Err()
}

// parseFlexDate tries multiple date formats that SQLite may return.
func parseFlexDate(s string) (time.Time, error) {
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date format: %s", s)
}

// SaveFilingStatus saves or updates the filing status for a tax year.
func (s *Store) SaveFilingStatus(year int, status string) error {
	_, err := s.DB.Exec(`INSERT INTO filing_status_config (tax_year, filing_status)
		VALUES (?, ?)
		ON CONFLICT(tax_year) DO UPDATE SET filing_status = excluded.filing_status, updated_at = CURRENT_TIMESTAMP`,
		year, status)
	if err != nil {
		return fmt.Errorf("save filing status: %w", err)
	}
	return nil
}

// GetFilingStatus returns the filing status for a tax year.
// Returns empty string if not set.
func (s *Store) GetFilingStatus(year int) (string, error) {
	var status string
	err := s.DB.QueryRow(`SELECT filing_status FROM filing_status_config WHERE tax_year = ?`, year).Scan(&status)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get filing status: %w", err)
	}
	return status, nil
}
