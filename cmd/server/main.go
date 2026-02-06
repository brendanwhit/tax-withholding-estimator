package main

import (
	"log"
	"net/http"
	"os"

	"github.com/brendanwhit/tax-withholding-estimator/internal/db"
	"github.com/brendanwhit/tax-withholding-estimator/internal/handler"
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

	tmplDir := os.Getenv("TEMPLATE_DIR")
	if tmplDir == "" {
		tmplDir = "templates"
	}

	srv, err := handler.NewServer(store, tmplDir)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
