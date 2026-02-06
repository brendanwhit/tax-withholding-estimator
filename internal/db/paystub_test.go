package db_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/brendanwhit/tax-withholding-estimator/internal/db"
)

func newTestDB(t *testing.T) *db.Store {
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
	return store
}

func TestSaveAndRetrievePaystub(t *testing.T) {
	store := newTestDB(t)

	ytdGross := 50000.00
	ytdFed := 7500.00
	p := &db.Paystub{
		PersonName:            "John",
		TaxYear:               2025,
		PayPeriodStart:        time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		PayPeriodEnd:          time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
		GrossPay:              5000.00,
		FederalTaxWithheld:    750.00,
		YTDGrossPay:           &ytdGross,
		YTDFederalTaxWithheld: &ytdFed,
	}

	id, err := store.SavePaystub(p)
	if err != nil {
		t.Fatalf("SavePaystub: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero ID")
	}

	paystubs, err := store.GetPaystubsByPersonAndYear("John", 2025)
	if err != nil {
		t.Fatalf("GetPaystubsByPersonAndYear: %v", err)
	}
	if len(paystubs) != 1 {
		t.Fatalf("expected 1 paystub, got %d", len(paystubs))
	}

	got := paystubs[0]
	if got.PersonName != "John" {
		t.Errorf("PersonName = %q, want %q", got.PersonName, "John")
	}
	if got.GrossPay != 5000.00 {
		t.Errorf("GrossPay = %v, want 5000.00", got.GrossPay)
	}
	if got.FederalTaxWithheld != 750.00 {
		t.Errorf("FederalTaxWithheld = %v, want 750.00", got.FederalTaxWithheld)
	}
	if got.YTDGrossPay == nil || *got.YTDGrossPay != 50000.00 {
		t.Errorf("YTDGrossPay = %v, want 50000.00", got.YTDGrossPay)
	}
}

func TestPaystubQueryByPersonAndYear(t *testing.T) {
	store := newTestDB(t)

	// Save paystubs for different people and years.
	stubs := []*db.Paystub{
		{PersonName: "Alice", TaxYear: 2025, PayPeriodStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), PayPeriodEnd: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC), GrossPay: 3000, FederalTaxWithheld: 450},
		{PersonName: "Alice", TaxYear: 2025, PayPeriodStart: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), PayPeriodEnd: time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC), GrossPay: 3000, FederalTaxWithheld: 450},
		{PersonName: "Bob", TaxYear: 2025, PayPeriodStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), PayPeriodEnd: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC), GrossPay: 4000, FederalTaxWithheld: 600},
		{PersonName: "Alice", TaxYear: 2026, PayPeriodStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), PayPeriodEnd: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), GrossPay: 3200, FederalTaxWithheld: 480},
	}

	for _, s := range stubs {
		if _, err := store.SavePaystub(s); err != nil {
			t.Fatalf("SavePaystub: %v", err)
		}
	}

	// Query Alice 2025 — should get 2.
	alice2025, err := store.GetPaystubsByPersonAndYear("Alice", 2025)
	if err != nil {
		t.Fatalf("query Alice 2025: %v", err)
	}
	if len(alice2025) != 2 {
		t.Errorf("Alice 2025: got %d paystubs, want 2", len(alice2025))
	}

	// Query Bob 2025 — should get 1.
	bob2025, err := store.GetPaystubsByPersonAndYear("Bob", 2025)
	if err != nil {
		t.Fatalf("query Bob 2025: %v", err)
	}
	if len(bob2025) != 1 {
		t.Errorf("Bob 2025: got %d paystubs, want 1", len(bob2025))
	}

	// Query Alice 2026 — should get 1.
	alice2026, err := store.GetPaystubsByPersonAndYear("Alice", 2026)
	if err != nil {
		t.Fatalf("query Alice 2026: %v", err)
	}
	if len(alice2026) != 1 {
		t.Errorf("Alice 2026: got %d paystubs, want 1", len(alice2026))
	}
}

func TestDuplicatePaystubUpload(t *testing.T) {
	store := newTestDB(t)

	p := &db.Paystub{
		PersonName:         "Carol",
		TaxYear:            2025,
		PayPeriodStart:     time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		PayPeriodEnd:       time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC),
		GrossPay:           5000.00,
		FederalTaxWithheld: 750.00,
	}

	// Save once.
	_, err := store.SavePaystub(p)
	if err != nil {
		t.Fatalf("first save: %v", err)
	}

	// Save again with updated amount (duplicate period).
	p.GrossPay = 5200.00
	p.FederalTaxWithheld = 780.00
	_, err = store.SavePaystub(p)
	if err != nil {
		t.Fatalf("duplicate save: %v", err)
	}

	// Should still have only 1 record, with updated values.
	stubs, err := store.GetPaystubsByPersonAndYear("Carol", 2025)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(stubs) != 1 {
		t.Fatalf("expected 1 paystub after duplicate, got %d", len(stubs))
	}
	if stubs[0].GrossPay != 5200.00 {
		t.Errorf("GrossPay after upsert = %v, want 5200.00", stubs[0].GrossPay)
	}
}

func TestFilingStatusSaveAndRetrieve(t *testing.T) {
	store := newTestDB(t)

	if err := store.SaveFilingStatus(2025, "married_filing_jointly"); err != nil {
		t.Fatalf("SaveFilingStatus: %v", err)
	}

	status, err := store.GetFilingStatus(2025)
	if err != nil {
		t.Fatalf("GetFilingStatus: %v", err)
	}
	if status != "married_filing_jointly" {
		t.Errorf("status = %q, want %q", status, "married_filing_jointly")
	}
}

func TestFilingStatusNotSet(t *testing.T) {
	store := newTestDB(t)

	status, err := store.GetFilingStatus(2025)
	if err != nil {
		t.Fatalf("GetFilingStatus: %v", err)
	}
	if status != "" {
		t.Errorf("expected empty status, got %q", status)
	}
}

func TestFilingStatusUpdate(t *testing.T) {
	store := newTestDB(t)

	if err := store.SaveFilingStatus(2025, "single"); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := store.SaveFilingStatus(2025, "married_filing_jointly"); err != nil {
		t.Fatalf("update: %v", err)
	}

	status, err := store.GetFilingStatus(2025)
	if err != nil {
		t.Fatalf("GetFilingStatus: %v", err)
	}
	if status != "married_filing_jointly" {
		t.Errorf("status = %q, want %q", status, "married_filing_jointly")
	}
}
