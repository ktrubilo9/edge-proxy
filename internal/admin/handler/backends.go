package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/ktrubilo9/edge-proxy/internal/api/adminpb"
)

func HandleBackendsList(proxyClient adminpb.ProxyAdminClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		logRequest(r)

		resp, err := proxyClient.GetBackends(ctx, &adminpb.Empty{})
		if err != nil {
			respondAdminRPCError(w, "Failed to get backends", err)
			return
		}
		items := resp.Backends
		if items == nil {
			items = []*adminpb.BackendResponse{}
		}
		respondJSON(w, items)
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
			respondAdminRPCError(w, "Failed to get backend", err)
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
		if err := decodeAdminJSON(r.Body, &req); err != nil {
			respondAdminJSONError(w, err)
			return
		}

		resp, err := proxyClient.AddBackend(ctx, &req)
		if err != nil {
			respondAdminRPCError(w, "Failed to add backend", err)
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
		if err := decodeAdminJSON(r.Body, &req); err != nil {
			respondAdminJSONError(w, err)
			return
		}
		req.Id = id

		resp, err := proxyClient.UpdateBackend(ctx, &req)
		if err != nil {
			respondAdminRPCError(w, "Failed to update backend", err)
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
			respondAdminRPCError(w, "Failed to remove backend", err)
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
