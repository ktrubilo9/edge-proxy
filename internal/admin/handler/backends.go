package handler

import (
	"context"
	"edge-proxy/internal/api/adminpb"
	"encoding/json"
	"net/http"
	"time"
)

func HandleBackendsList(proxyClient adminpb.ProxyAdminClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		logRequest(r)

		resp, err := proxyClient.GetBackends(ctx, &adminpb.Empty{})
		if err != nil {
			http.Error(w, "Failed to get backends: "+err.Error(), http.StatusInternalServerError)
			return
		}
		respondJSON(w, resp.Backends)
	}
}

func HandleBackendGet(proxyClient adminpb.ProxyAdminClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "Missing id in path", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		logRequest(r)

		req := &adminpb.GetBackendRequest{Id: id}
		resp, err := proxyClient.GetBackend(ctx, req)
		if err != nil {
			http.Error(w, "Failed to get backend: "+err.Error(), http.StatusInternalServerError)
			return
		}
		respondJSON(w, resp)
	}
}

func HandleBackendAdd(proxyClient adminpb.ProxyAdminClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		logRequest(r)

		var req adminpb.AddBackendRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		resp, err := proxyClient.AddBackend(ctx, &req)
		if err != nil {
			http.Error(w, "Failed to add backend: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !resp.Success {
			status := clientErrorStatus(resp.Error)
			http.Error(w, "Failed to add backend: "+resp.Error, status)
			return
		}
		respondJSON(w, resp)
	}
}

func HandleBackendUpdate(proxyClient adminpb.ProxyAdminClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "Missing id in path", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		logRequest(r)

		var req adminpb.UpdateBackendRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		req.Id = id

		resp, err := proxyClient.UpdateBackend(ctx, &req)
		if err != nil {
			http.Error(w, "Failed to update backend: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !resp.Success {
			status := clientErrorStatus(resp.Error)
			http.Error(w, "Failed to update backend: "+resp.Error, status)
			return
		}
		respondJSON(w, resp)
	}
}

func HandleBackendDelete(proxyClient adminpb.ProxyAdminClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "Missing id in path", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		logRequest(r)

		req := &adminpb.RemoveBackendRequest{Id: id}
		resp, err := proxyClient.RemoveBackend(ctx, req)
		if err != nil {
			http.Error(w, "Failed to remove backend: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !resp.Success {
			status := clientErrorStatus(resp.Error)
			http.Error(w, "Failed to remove backend: "+resp.Error, status)
			return
		}
		respondJSON(w, resp)
	}
}
