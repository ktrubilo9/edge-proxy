package handler

import (
	"context"
	"edge-proxy/internal/api/adminpb"
	"encoding/json"
	"net/http"
	"time"
)

func HandleVhostsList(proxyClient adminpb.ProxyAdminClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		logRequest(r)

		resp, err := proxyClient.GetVirtualHosts(ctx, &adminpb.Empty{})
		if err != nil {
			http.Error(w, "Failed to get virtual hosts: "+err.Error(), http.StatusInternalServerError)
			return
		}
		respondJSON(w, resp.VirtualHosts)
	}
}

func HandleVhostGet(proxyClient adminpb.ProxyAdminClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domain := r.PathValue("domain")
		if domain == "" {
			http.Error(w, "Missing domain in path", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		logRequest(r)

		req := &adminpb.GetVirtualHostRequest{Domain: domain}
		resp, err := proxyClient.GetVirtualHost(ctx, req)
		if err != nil {
			http.Error(w, "Failed to get virtual host: "+err.Error(), http.StatusInternalServerError)
			return
		}
		respondJSON(w, resp)
	}
}

func HandleVhostAdd(proxyClient adminpb.ProxyAdminClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		logRequest(r)

		var req adminpb.AddVirtualHostRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		resp, err := proxyClient.AddVirtualHost(ctx, &req)
		if err != nil {
			http.Error(w, "Failed to add virtual host: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !resp.Success {
			status := clientErrorStatus(resp.Error)
			http.Error(w, "Failed to add virtual host: "+resp.Error, status)
			return
		}
		respondJSON(w, resp)
	}
}

func HandleVhostUpdate(proxyClient adminpb.ProxyAdminClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domain := r.PathValue("domain")
		if domain == "" {
			http.Error(w, "Missing domain in path", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		logRequest(r)

		var req adminpb.UpdateVirtualHostRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		req.Domain = domain

		resp, err := proxyClient.UpdateVirtualHost(ctx, &req)
		if err != nil {
			http.Error(w, "Failed to update virtual host: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !resp.Success {
			status := clientErrorStatus(resp.Error)
			http.Error(w, "Failed to update virtual host: "+resp.Error, status)
			return
		}
		respondJSON(w, resp)
	}
}

func HandleVhostDelete(proxyClient adminpb.ProxyAdminClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domain := r.PathValue("domain")
		if domain == "" {
			http.Error(w, "Missing domain in path", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		logRequest(r)

		req := &adminpb.RemoveVirtualHostRequest{Domain: domain}
		resp, err := proxyClient.RemoveVirtualHost(ctx, req)
		if err != nil {
			http.Error(w, "Failed to remove virtual host: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !resp.Success {
			status := clientErrorStatus(resp.Error)
			http.Error(w, "Failed to remove virtual host: "+resp.Error, status)
			return
		}
		respondJSON(w, resp)
	}
}

func HandleSecurityConfigGet(proxyClient adminpb.ProxyAdminClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domain := r.PathValue("domain")
		if domain == "" {
			http.Error(w, "Missing domain in path", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		logRequest(r)

		req := &adminpb.GetVirtualHostRequest{Domain: domain}
		resp, err := proxyClient.GetVirtualHostSecurity(ctx, req)
		if err != nil {
			http.Error(w, "Failed to get security config: "+err.Error(), http.StatusInternalServerError)
			return
		}
		respondJSON(w, resp)
	}
}

func HandleSecurityConfigUpdate(proxyClient adminpb.ProxyAdminClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domain := r.PathValue("domain")
		if domain == "" {
			http.Error(w, "Missing domain in path", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		logRequest(r)

		var req adminpb.SetVirtualHostSecurityPolicyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		req.Domain = domain

		resp, err := proxyClient.SetVirtualHostSecurityPolicy(ctx, &req)
		if err != nil {
			http.Error(w, "Failed to update security config: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !resp.Success {
			status := clientErrorStatus(resp.Error)
			http.Error(w, "Failed to update security config: "+resp.Error, status)
			return
		}
		respondJSON(w, resp)
	}
}

func HandlePoliciesList(proxyClient adminpb.ProxyAdminClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		logRequest(r)

		resp, err := proxyClient.GetPolicies(ctx, &adminpb.Empty{})
		if err != nil {
			http.Error(w, "Failed to get policies: "+err.Error(), http.StatusInternalServerError)
			return
		}
		respondJSON(w, resp.Policies)
	}
}

func HandlePolicyUpsert(proxyClient adminpb.ProxyAdminClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		logRequest(r)

		var policy adminpb.SecurityPolicy
		if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if pathID := r.PathValue("id"); pathID != "" {
			policy.Id = pathID
		}

		resp, err := proxyClient.UpsertPolicy(ctx, &adminpb.UpsertPolicyRequest{Policy: &policy})
		if err != nil {
			http.Error(w, "Failed to upsert policy: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !resp.Success {
			status := clientErrorStatus(resp.Error)
			http.Error(w, "Failed to upsert policy: "+resp.Error, status)
			return
		}
		respondJSON(w, resp)
	}
}
