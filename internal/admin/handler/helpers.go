package handler

import (
	"encoding/json"
	"log"
	"net/http"
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
