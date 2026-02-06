package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	_ "modernc.org/sqlite"
)

// Store wraps the SQLite database connection.
type Store struct {
	DB *sql.DB
}

// New opens (or creates) a SQLite database at the given path.
func New(path string) (*Store, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode and foreign keys.
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("exec %q: %w", pragma, err)
		}
	}

	return &Store{DB: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.DB.Close()
}

// Migrate runs all pending SQL migrations in order.
// Migrations are embedded as .sql files and tracked in a migrations table.
func (s *Store) Migrate() error {
	_, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	// Sort by filename to ensure order.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		name := entry.Name()

		var count int
		err := s.DB.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", name).Scan(&count)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if count > 0 {
			continue
		}

		data, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		if _, err := s.DB.Exec(string(data)); err != nil {
			return fmt.Errorf("exec migration %s: %w", name, err)
		}

		if _, err := s.DB.Exec("INSERT INTO schema_migrations (version) VALUES (?)", name); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}
	}

	return nil
}
