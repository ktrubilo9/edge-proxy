package handler

import (
	"context"
	"edge-proxy/internal/api/adminpb"
	"encoding/json"
	"net/http"
	"time"
)

func HandleServerConfig(proxyClient adminpb.ProxyAdminClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		logRequest(r)

		switch r.Method {
		case http.MethodGet:
			resp, err := proxyClient.GetServerConfig(ctx, &adminpb.Empty{})
			if err != nil {
				http.Error(w, "Failed to get server config: "+err.Error(), http.StatusInternalServerError)
				return
			}
			respondJSON(w, resp)
		case http.MethodPut:
			var req adminpb.ServerConfig
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
				return
			}
			resp, err := proxyClient.SetServerConfig(ctx, &req)
			if err != nil {
				http.Error(w, "Failed to set server config: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if !resp.Success {
			status := clientErrorStatus(resp.Error)
			http.Error(w, "Failed to set server config: "+resp.Error, status)
			return
		}
			respondJSON(w, resp)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func HandleLoadBalancerConfig(proxyClient adminpb.ProxyAdminClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		logRequest(r)

		switch r.Method {
		case http.MethodGet:
			resp, err := proxyClient.GetLoadBalancer(ctx, &adminpb.Empty{})
			if err != nil {
				http.Error(w, "Failed to get load balancer config: "+err.Error(), http.StatusInternalServerError)
				return
			}
			respondJSON(w, resp)
		case http.MethodPut:
			var req adminpb.LoadBalancerConfig
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
				return
			}
			resp, err := proxyClient.SetLoadBalancer(ctx, &req)
			if err != nil {
				http.Error(w, "Failed to set load balancer config: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if !resp.Success {
			status := clientErrorStatus(resp.Error)
			http.Error(w, "Failed to set load balancer config: "+resp.Error, status)
			return
		}
			respondJSON(w, resp)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
