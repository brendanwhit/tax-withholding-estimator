package tax

import (
	"database/sql"
	"fmt"
	"time"
)

// Store handles tax bracket persistence in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a new tax store backed by the given database.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// SaveBracketSchedule saves a bracket schedule (brackets + standard deduction) to SQLite.
func (s *Store) SaveBracketSchedule(schedule *BracketSchedule, source string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Upsert standard deduction.
	_, err = tx.Exec(`INSERT INTO standard_deductions (tax_year, filing_status, amount)
		VALUES (?, ?, ?)
		ON CONFLICT(tax_year, filing_status) DO UPDATE SET amount = excluded.amount`,
		schedule.TaxYear, string(schedule.FilingStatus), schedule.StandardDeduction)
	if err != nil {
		return fmt.Errorf("save standard deduction: %w", err)
	}

	// Delete existing brackets for this year+status, then insert new ones.
	_, err = tx.Exec(`DELETE FROM tax_brackets WHERE tax_year = ? AND filing_status = ?`,
		schedule.TaxYear, string(schedule.FilingStatus))
	if err != nil {
		return fmt.Errorf("delete old brackets: %w", err)
	}

	for _, b := range schedule.Brackets {
		var maxVal *float64
		if b.Max != 0 {
			v := b.Max
			maxVal = &v
		}
		_, err = tx.Exec(`INSERT INTO tax_brackets (tax_year, filing_status, bracket_min, bracket_max, rate)
			VALUES (?, ?, ?, ?, ?)`,
			schedule.TaxYear, string(schedule.FilingStatus), b.Min, maxVal, b.Rate)
		if err != nil {
			return fmt.Errorf("insert bracket: %w", err)
		}
	}

	// Update bracket cache metadata.
	_, err = tx.Exec(`INSERT INTO bracket_cache (tax_year, source, fetched_at)
		VALUES (?, ?, ?)
		ON CONFLICT(tax_year) DO UPDATE SET source = excluded.source, fetched_at = excluded.fetched_at`,
		schedule.TaxYear, source, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("update bracket cache: %w", err)
	}

	return tx.Commit()
}

// LoadBracketSchedule loads a bracket schedule from SQLite.
// Returns nil, nil if not found.
func (s *Store) LoadBracketSchedule(year int, status FilingStatus) (*BracketSchedule, error) {
	// Load standard deduction.
	var deduction float64
	err := s.db.QueryRow(`SELECT amount FROM standard_deductions WHERE tax_year = ? AND filing_status = ?`,
		year, string(status)).Scan(&deduction)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load standard deduction: %w", err)
	}

	// Load brackets.
	rows, err := s.db.Query(`SELECT bracket_min, bracket_max, rate FROM tax_brackets
		WHERE tax_year = ? AND filing_status = ?
		ORDER BY bracket_min ASC`,
		year, string(status))
	if err != nil {
		return nil, fmt.Errorf("load brackets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var brackets []Bracket
	for rows.Next() {
		var b Bracket
		var maxVal *float64
		if err := rows.Scan(&b.Min, &maxVal, &b.Rate); err != nil {
			return nil, fmt.Errorf("scan bracket: %w", err)
		}
		if maxVal != nil {
			b.Max = *maxVal
		}
		brackets = append(brackets, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate brackets: %w", err)
	}

	if len(brackets) == 0 {
		return nil, nil
	}

	return &BracketSchedule{
		TaxYear:           year,
		FilingStatus:      status,
		Brackets:          brackets,
		StandardDeduction: deduction,
	}, nil
}

// GetBrackets returns the bracket schedule for the given year and filing status.
// It first checks SQLite cache, then falls back to hardcoded data and caches it.
func (s *Store) GetBrackets(year int, status FilingStatus) (*BracketSchedule, error) {
	// Try loading from database first.
	schedule, err := s.LoadBracketSchedule(year, status)
	if err != nil {
		return nil, err
	}
	if schedule != nil {
		return schedule, nil
	}

	// Fall back to hardcoded data.
	schedule = HardcodedBrackets(year, status)
	if schedule == nil {
		return nil, fmt.Errorf("no bracket data available for year %d, status %s", year, status)
	}

	// Cache the hardcoded data in SQLite.
	if err := s.SaveBracketSchedule(schedule, "hardcoded"); err != nil {
		return nil, fmt.Errorf("cache hardcoded brackets: %w", err)
	}

	return schedule, nil
}
