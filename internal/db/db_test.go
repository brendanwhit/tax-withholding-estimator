package db_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/brendanwhit/tax-withholding-estimator/internal/db"
)

func TestNewAndMigrate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	store, err := db.New(path)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer func() { _ = store.Close() }()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}

	// Verify tables exist by querying them.
	tables := []string{"tax_brackets", "standard_deductions", "filing_status_config", "paystubs", "bracket_cache", "schema_migrations"}
	for _, table := range tables {
		var count int
		err := store.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if err != nil {
			t.Errorf("table %s not accessible: %v", table, err)
		}
	}
}

func TestMigrateIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	store, err := db.New(path)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Run migrations twice — should not error.
	if err := store.Migrate(); err != nil {
		t.Fatalf("first Migrate() error: %v", err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatalf("second Migrate() error: %v", err)
	}
}

func TestDBPathFromEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "env.db")

	store, err := db.New(path)
	if err != nil {
		t.Fatalf("New() with nested path error: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Verify the directory was created.
	if _, err := os.Stat(filepath.Dir(path)); os.IsNotExist(err) {
		t.Error("expected db directory to be created")
	}
}
