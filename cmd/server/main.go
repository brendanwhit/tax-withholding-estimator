package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/brendanwhit/tax-withholding-estimator/internal/db"
)

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/tax.db"
	}

	store, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = store.Close() }()

	if err := store.Migrate(); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "Tax Withholding Estimator")
	})

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
