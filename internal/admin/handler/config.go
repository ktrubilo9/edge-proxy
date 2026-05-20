package handler

import (
	"context"
	"edge-proxy/internal/api/adminpb"
	"encoding/json"
	"net/http"
	"time"
)

func HandleGlobalConfig(proxyClient adminpb.ProxyAdminClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		logRequest(r)

		switch r.Method {
		case http.MethodGet:
			resp, err := proxyClient.GetGlobalConfig(ctx, &adminpb.Empty{})
			if err != nil {
				http.Error(w, "Failed to get global config: "+err.Error(), http.StatusInternalServerError)
				return
			}
			respondJSON(w, resp)
		case http.MethodPut:
			var req adminpb.GlobalConfig
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
				return
			}
			resp, err := proxyClient.SetGlobalConfig(ctx, &req)
			if err != nil {
				http.Error(w, "Failed to set global config: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if !resp.Success {
				http.Error(w, "Failed to set global config: "+resp.Error, http.StatusInternalServerError)
				return
			}
			respondJSON(w, resp)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
