package handler_test

import (
	"net/http"
	"testing"

	"github.com/ktrubilo9/edge-proxy/internal/admin/handler"
	"github.com/ktrubilo9/edge-proxy/internal/api/adminpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestVirtualHostHTTPContract(t *testing.T) {
	const (
		collection = "/api/vhost"
		domain     = "app.example.local"
		item       = collection + "/" + domain
		hostJSON   = `{"domain":"app.example.local","backend_ids":["backend-1"],"path_routes":[{"path":"/api","backend_ids":["backend-2"],"strip_prefix":true}],"security_policy_id":"default"}`
	)
	host := &adminpb.VirtualHost{
		Domain: domain, BackendIds: []string{"backend-1"}, SecurityPolicyId: "default",
		PathRoutes: []*adminpb.PathRoute{{Path: "/api", BackendIds: []string{"backend-2"}, StripPrefix: true}},
	}
	addRequest := &adminpb.AddVirtualHostRequest{VirtualHost: host}
	addBody := `{"virtual_host":` + hostJSON + "}"

	runAdminHTTPCases(t, []adminHTTPCase{
		{
			name: "list returns a JSON array", handler: handler.HandleVhostsList,
			method: http.MethodGet, path: collection,
			rpc: "GetVirtualHosts", rpcRequest: &adminpb.Empty{},
			rpcResponse: &adminpb.GetVirtualHostsResponse{VirtualHosts: []*adminpb.VirtualHost{host}},
			wantStatus:  http.StatusOK, wantJSON: "[" + hostJSON + "]",
		},
		{
			name: "empty list returns an empty array", handler: handler.HandleVhostsList,
			method: http.MethodGet, path: collection,
			rpc: "GetVirtualHosts", rpcRequest: &adminpb.Empty{}, rpcResponse: &adminpb.GetVirtualHostsResponse{},
			wantStatus: http.StatusOK, wantJSON: "[]",
		},
		{
			name: "get returns virtual host and nested routes", handler: handler.HandleVhostGet,
			method: http.MethodGet, path: item, pathParam: "domain", pathValue: domain,
			rpc: "GetVirtualHost", rpcRequest: &adminpb.GetVirtualHostRequest{Domain: domain}, rpcResponse: host,
			wantStatus: http.StatusOK, wantJSON: hostJSON,
		},
		{
			name: "add decodes the virtual host wrapper", handler: handler.HandleVhostAdd,
			method: http.MethodPost, path: collection, body: addBody,
			rpc: "AddVirtualHost", rpcRequest: addRequest,
			rpcResponse: &adminpb.BasicResponse{Success: true, Message: "Virtual host added"},
			wantStatus:  http.StatusOK, wantJSON: `{"success":true,"message":"Virtual host added"}`,
		},
		{
			name: "update uses path domain", handler: handler.HandleVhostUpdate,
			method: http.MethodPut, path: item, pathParam: "domain", pathValue: domain,
			body: `{"domain":"body.example.local","virtual_host":{"backend_ids":["backend-2"]}}`,
			rpc:  "UpdateVirtualHost", rpcRequest: &adminpb.UpdateVirtualHostRequest{
				Domain: domain, VirtualHost: &adminpb.VirtualHost{BackendIds: []string{"backend-2"}},
			},
			rpcResponse: &adminpb.BasicResponse{Success: true, Message: "Virtual host updated"},
			wantStatus:  http.StatusOK, wantJSON: `{"success":true,"message":"Virtual host updated"}`,
		},
		{
			name: "delete passes path domain", handler: handler.HandleVhostDelete,
			method: http.MethodDelete, path: item, pathParam: "domain", pathValue: domain,
			rpc: "RemoveVirtualHost", rpcRequest: &adminpb.RemoveVirtualHostRequest{Domain: domain},
			rpcResponse: &adminpb.BasicResponse{Success: true, Message: "Virtual host removed"},
			wantStatus:  http.StatusOK, wantJSON: `{"success":true,"message":"Virtual host removed"}`,
		},
		{
			name: "malformed update JSON never calls RPC", handler: handler.HandleVhostUpdate,
			method: http.MethodPut, path: item, pathParam: "domain", pathValue: domain, body: `{"virtual_host":`,
			wantStatus: http.StatusBadRequest, wantText: "Invalid request body: unexpected EOF",
		},
		{
			name: "missing delete domain never calls RPC", handler: handler.HandleVhostDelete,
			method: http.MethodDelete, path: collection,
			wantStatus: http.StatusBadRequest, wantText: "Missing domain in path",
		},
		{
			name: "unknown virtual host maps to not found", handler: handler.HandleVhostGet,
			method: http.MethodGet, path: item, pathParam: "domain", pathValue: domain,
			rpc: "GetVirtualHost", rpcRequest: &adminpb.GetVirtualHostRequest{Domain: domain},
			rpcError:   status.Error(codes.NotFound, "virtual host not found"),
			wantStatus: http.StatusNotFound, wantText: "Failed to get virtual host: virtual host not found",
		},
		{
			name: "validation failure maps to bad request", handler: handler.HandleVhostAdd,
			method: http.MethodPost, path: collection, body: `{"virtual_host":{"domain":"app.example.local","backend_ids":["missing"]}}`,
			rpc: "AddVirtualHost", rpcRequest: &adminpb.AddVirtualHostRequest{
				VirtualHost: &adminpb.VirtualHost{Domain: domain, BackendIds: []string{"missing"}},
			},
			rpcResponse: &adminpb.BasicResponse{Error: "virtual host app.example.local references unknown backend: missing"},
			wantStatus:  http.StatusBadRequest, wantText: "Failed to add virtual host: virtual host app.example.local references unknown backend: missing",
		},
		{
			name: "duplicate virtual host maps to conflict", handler: handler.HandleVhostAdd,
			method: http.MethodPost, path: collection, body: addBody,
			rpc: "AddVirtualHost", rpcRequest: addRequest,
			rpcResponse: &adminpb.BasicResponse{Error: "virtual host already exists: app.example.local"},
			wantStatus:  http.StatusConflict, wantText: "Failed to add virtual host: virtual host already exists: app.example.local",
		},
		{
			name: "deleting unknown virtual host maps to not found", handler: handler.HandleVhostDelete,
			method: http.MethodDelete, path: item, pathParam: "domain", pathValue: domain,
			rpc: "RemoveVirtualHost", rpcRequest: &adminpb.RemoveVirtualHostRequest{Domain: domain},
			rpcResponse: &adminpb.BasicResponse{Error: "virtual host does not exist in config: app.example.local"},
			wantStatus:  http.StatusNotFound, wantText: "Failed to remove virtual host: virtual host does not exist in config: app.example.local",
		},
		{
			name: "persistence failure maps to internal error", handler: handler.HandleVhostAdd,
			method: http.MethodPost, path: collection, body: addBody,
			rpc: "AddVirtualHost", rpcRequest: addRequest,
			rpcError:   status.Error(codes.Internal, "could not persist configuration"),
			wantStatus: http.StatusInternalServerError, wantText: "Failed to add virtual host: could not persist configuration",
		},
	})
}
