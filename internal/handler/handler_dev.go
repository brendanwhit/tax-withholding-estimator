//go:build dev

package handler

import (
	"log"
	"net/http"
)

func init() {
	devRoutes = append(devRoutes, devRoute{
		pattern: "POST /admin/clear-db",
		handler: func(s *Server) http.HandlerFunc {
			return s.handleClearDB
		},
	})
}

func (s *Server) handleClearDB(w http.ResponseWriter, r *http.Request) {
	tables := []string{"paystubs", "filing_status_config", "tax_brackets", "standard_deductions", "bracket_cache"}
	for _, table := range tables {
		if _, err := s.Store.DB.Exec("DELETE FROM " + table); err != nil {
			log.Printf("failed to clear %s: %v", table, err)
			http.Error(w, "failed to clear "+table+": "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte("Database cleared successfully"))
}
