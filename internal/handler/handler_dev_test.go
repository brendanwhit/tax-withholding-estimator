//go:build dev

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brendanwhit/tax-withholding-estimator/internal/db"
)

func TestClearDBEndpoint(t *testing.T) {
	store, err := db.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatal(err)
	}

	srv, err := NewServer(store, "../../templates")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Insert a paystub so we can verify it gets cleared.
	_, err = store.DB.Exec(`INSERT INTO paystubs (person_name, tax_year, pay_period_start, pay_period_end, gross_pay, federal_tax_withheld)
		VALUES ('Test', 2026, '2026-01-01', '2026-01-15', 5000, 1000)`)
	if err != nil {
		t.Fatal(err)
	}

	// Verify data exists.
	var count int
	if err := store.DB.QueryRow("SELECT COUNT(*) FROM paystubs").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 paystub before clear, got %d", count)
	}

	// POST /admin/clear-db should clear all data.
	req := httptest.NewRequest(http.MethodPost, "/admin/clear-db", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify data cleared.
	if err := store.DB.QueryRow("SELECT COUNT(*) FROM paystubs").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 paystubs after clear, got %d", count)
	}
}

func TestClearDBEndpointGetNotAllowed(t *testing.T) {
	store, err := db.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatal(err)
	}

	srv, err := NewServer(store, "../../templates")
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// GET /admin/clear-db should not succeed (POST-only route).
	req := httptest.NewRequest(http.MethodGet, "/admin/clear-db", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatal("expected non-200 for GET request to POST-only endpoint")
	}
}
