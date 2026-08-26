package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

func logRequest(r *http.Request) {
	log.Printf("[ADMIN-API] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
}

func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// clientErrorStatus maps validation-ish admin failures to 4xx.
func clientErrorStatus(msg string) int {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "not found"):
		return http.StatusNotFound
	case strings.Contains(lower, "already exists"), strings.Contains(lower, "duplicate"), strings.Contains(lower, "conflict"):
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}
