package handler_test

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/brendanwhit/tax-withholding-estimator/internal/db"
)

func TestDashboardShowsEOYProjection(t *testing.T) {
	srv, mux := setupTestServer(t)

	year := time.Now().Year()
	stubs := []*db.Paystub{
		{PersonName: "Test", TaxYear: year, PayPeriodStart: time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC), PayPeriodEnd: time.Date(year, 1, 14, 0, 0, 0, 0, time.UTC), GrossPay: 5000, FederalTaxWithheld: 800},
		{PersonName: "Test", TaxYear: year, PayPeriodStart: time.Date(year, 1, 15, 0, 0, 0, 0, time.UTC), PayPeriodEnd: time.Date(year, 1, 28, 0, 0, 0, 0, time.UTC), GrossPay: 5000, FederalTaxWithheld: 800},
	}
	for _, s := range stubs {
		if _, err := srv.Store.SavePaystub(s); err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// The dashboard should render the withholding summary section.
	if !strings.Contains(body, "Withholding Summary") {
		t.Error("expected dashboard to contain 'Withholding Summary'")
	}

	// Uploaded paystubs table should be present.
	if !strings.Contains(body, "Uploaded Paystubs") {
		t.Error("expected dashboard to contain 'Uploaded Paystubs'")
	}

	// EOY projection section should render when paystubs exist.
	if !strings.Contains(body, "End-of-Year Withholding Projection") {
		t.Error("expected dashboard to contain 'End-of-Year Withholding Projection'")
	}
}

func TestDashboardShowsDeductionSummary(t *testing.T) {
	srv, mux := setupTestServer(t)

	year := time.Now().Year()
	stub := &db.Paystub{
		PersonName: "Test", TaxYear: year,
		PayPeriodStart: time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC),
		PayPeriodEnd:   time.Date(year, 1, 14, 0, 0, 0, 0, time.UTC),
		GrossPay: 5000, FederalTaxWithheld: 800,
	}
	paystubID, err := srv.Store.SavePaystub(stub)
	if err != nil {
		t.Fatal(err)
	}

	// Save a deduction for this paystub.
	ded := &db.PreTaxDeduction{
		PaystubID:     paystubID,
		DeductionType: "401k",
		Amount:        500,
	}
	if err := srv.Store.SavePreTaxDeduction(ded); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()

	if !strings.Contains(body, "Pre-Tax Deductions") {
		t.Error("expected dashboard to contain 'Pre-Tax Deductions'")
	}
	if !strings.Contains(body, "401(k)") {
		t.Error("expected dashboard to show formatted '401(k)' deduction type")
	}
}
