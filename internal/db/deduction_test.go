package db_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/brendanwhit/tax-withholding-estimator/internal/db"
)

func mustParseDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestSaveAndGetDeductions(t *testing.T) {
	store, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	if err := store.Migrate(); err != nil {
		t.Fatal(err)
	}

	// Insert a paystub first.
	p := &db.Paystub{
		PersonName:         "Alice",
		TaxYear:            2025,
		PayPeriodStart:     mustParseDate("2025-01-01"),
		PayPeriodEnd:       mustParseDate("2025-01-15"),
		GrossPay:           5000,
		FederalTaxWithheld: 800,
	}
	paystubID, err := store.SavePaystub(p)
	if err != nil {
		t.Fatal(err)
	}

	// Save deductions.
	ytd := 500.0
	err = store.SavePreTaxDeduction(&db.PreTaxDeduction{
		PaystubID:     paystubID,
		DeductionType: "401k",
		Amount:        500,
		YTDAmount:     &ytd,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = store.SavePreTaxDeduction(&db.PreTaxDeduction{
		PaystubID:     paystubID,
		DeductionType: "hsa",
		Amount:        150,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Get summaries.
	summaries, err := store.GetDeductionSummaryByYear(2025)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 deduction summaries, got %d", len(summaries))
	}

	// Check total.
	total, err := store.GetTotalPreTaxDeductionsByYear(2025)
	if err != nil {
		t.Fatal(err)
	}
	if total != 650 {
		t.Errorf("total deductions = %v, want 650", total)
	}
}

func TestContributionLimits(t *testing.T) {
	store, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	if err := store.Migrate(); err != nil {
		t.Fatal(err)
	}

	// Get hardcoded limits.
	limits := db.HardcodedContributionLimits(2025)
	if len(limits) == 0 {
		t.Fatal("expected hardcoded limits for 2025")
	}

	// Save and retrieve.
	if err := store.SaveContributionLimits(limits); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetContributionLimits(2025)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != len(limits) {
		t.Errorf("loaded %d limits, want %d", len(loaded), len(limits))
	}

	// Check 401k limit.
	for _, l := range loaded {
		if l.DeductionType == "401k" {
			if l.AnnualLimit != 23500 {
				t.Errorf("401k limit = %v, want 23500", l.AnnualLimit)
			}
			return
		}
	}
	t.Error("401k limit not found")
}

func TestGetOrCacheContributionLimits(t *testing.T) {
	store, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	if err := store.Migrate(); err != nil {
		t.Fatal(err)
	}

	// First call should cache from hardcoded.
	limits, err := store.GetOrCacheContributionLimits(2025)
	if err != nil {
		t.Fatal(err)
	}
	if len(limits) == 0 {
		t.Fatal("expected limits")
	}

	// Second call should load from DB.
	limits2, err := store.GetOrCacheContributionLimits(2025)
	if err != nil {
		t.Fatal(err)
	}
	if len(limits2) != len(limits) {
		t.Errorf("second call returned %d limits, want %d", len(limits2), len(limits))
	}
}
