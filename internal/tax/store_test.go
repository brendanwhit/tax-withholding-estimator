package tax_test

import (
	"path/filepath"
	"testing"

	"github.com/brendanwhit/tax-withholding-estimator/internal/db"
	"github.com/brendanwhit/tax-withholding-estimator/internal/tax"
)

func newTestStore(t *testing.T) *tax.Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	store, err := db.New(path)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return tax.NewStore(store.DB)
}

func TestSaveAndLoadBracketSchedule(t *testing.T) {
	s := newTestStore(t)

	schedule := tax.HardcodedBrackets(2025, tax.Single)
	if schedule == nil {
		t.Fatal("expected hardcoded brackets")
	}

	if err := s.SaveBracketSchedule(schedule, "test"); err != nil {
		t.Fatalf("SaveBracketSchedule: %v", err)
	}

	loaded, err := s.LoadBracketSchedule(2025, tax.Single)
	if err != nil {
		t.Fatalf("LoadBracketSchedule: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected loaded schedule")
	}

	if loaded.StandardDeduction != schedule.StandardDeduction {
		t.Errorf("standard deduction = %v, want %v", loaded.StandardDeduction, schedule.StandardDeduction)
	}
	if len(loaded.Brackets) != len(schedule.Brackets) {
		t.Fatalf("bracket count = %d, want %d", len(loaded.Brackets), len(schedule.Brackets))
	}

	// Verify the loaded schedule produces the same tax calculation.
	for _, income := range []float64{0, 15000, 50000, 100000, 500000} {
		original := schedule.CalculateTax(income)
		roundTripped := loaded.CalculateTax(income)
		if !almostEqual(original, roundTripped, 0.01) {
			t.Errorf("income %v: original tax %v != loaded tax %v", income, original, roundTripped)
		}
	}
}

func TestLoadBracketScheduleNotFound(t *testing.T) {
	s := newTestStore(t)

	loaded, err := s.LoadBracketSchedule(2099, tax.Single)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != nil {
		t.Error("expected nil for missing schedule")
	}
}

func TestGetBracketsFallsBackToHardcoded(t *testing.T) {
	s := newTestStore(t)

	// First call should fall back to hardcoded and cache.
	schedule, err := s.GetBrackets(2025, tax.MarriedFilingJointly)
	if err != nil {
		t.Fatalf("GetBrackets: %v", err)
	}
	if schedule == nil {
		t.Fatal("expected schedule")
	}

	// Second call should load from cache.
	cached, err := s.GetBrackets(2025, tax.MarriedFilingJointly)
	if err != nil {
		t.Fatalf("GetBrackets (cached): %v", err)
	}
	if cached == nil {
		t.Fatal("expected cached schedule")
	}

	// Both should produce same results.
	income := 150000.0
	if !almostEqual(schedule.CalculateTax(income), cached.CalculateTax(income), 0.01) {
		t.Error("cached schedule produces different results")
	}
}

func TestGetBracketsUnsupportedYear(t *testing.T) {
	s := newTestStore(t)

	_, err := s.GetBrackets(2020, tax.Single)
	if err == nil {
		t.Error("expected error for unsupported year")
	}
}

func TestSaveBracketScheduleUpsert(t *testing.T) {
	s := newTestStore(t)

	schedule := tax.HardcodedBrackets(2025, tax.Single)
	if schedule == nil {
		t.Fatal("expected hardcoded brackets")
	}

	// Save twice — should not error (upsert).
	if err := s.SaveBracketSchedule(schedule, "first"); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := s.SaveBracketSchedule(schedule, "second"); err != nil {
		t.Fatalf("second save: %v", err)
	}

	loaded, err := s.LoadBracketSchedule(2025, tax.Single)
	if err != nil {
		t.Fatalf("LoadBracketSchedule: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected loaded schedule")
	}
}

func TestBracketCacheStoredByYear(t *testing.T) {
	s := newTestStore(t)

	// Cache brackets for two different years.
	for _, year := range []int{2025, 2026} {
		_, err := s.GetBrackets(year, tax.Single)
		if err != nil {
			t.Fatalf("GetBrackets(%d): %v", year, err)
		}
	}

	// Verify both are independently retrievable.
	s2025, err := s.LoadBracketSchedule(2025, tax.Single)
	if err != nil || s2025 == nil {
		t.Fatalf("load 2025: err=%v, schedule=%v", err, s2025)
	}
	s2026, err := s.LoadBracketSchedule(2026, tax.Single)
	if err != nil || s2026 == nil {
		t.Fatalf("load 2026: err=%v, schedule=%v", err, s2026)
	}

	// They should have different standard deductions.
	if s2025.StandardDeduction == s2026.StandardDeduction {
		t.Error("expected different standard deductions for different years")
	}
}
