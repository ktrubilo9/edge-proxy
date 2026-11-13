package handler_test

import (
	"net/http"
	"testing"

	"github.com/ktrubilo9/edge-proxy/internal/admin/handler"
	"github.com/ktrubilo9/edge-proxy/internal/api/adminpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestBackendHTTPContract(t *testing.T) {
	const (
		collection  = "/api/backend"
		item        = "/api/backend/backend-1"
		backendURL  = "http://backend-a:8080"
		backendID   = "backend-1"
		backendJSON = `{"id":"backend-1","url":"http://backend-a:8080","weight":2,"enabled":true,"error_count":3,"last_error":"probe failed"}`
	)
	backend := &adminpb.BackendResponse{
		Id: backendID, Url: backendURL, Weight: 2, Enabled: true, ErrorCount: 3, LastError: "probe failed",
	}
	addRequest := &adminpb.AddBackendRequest{Id: backendID, Url: backendURL, Weight: 2, Enabled: true}
	const addBody = `{"id":"backend-1","url":"http://backend-a:8080","weight":2,"enabled":true}`

	runAdminHTTPCases(t, []adminHTTPCase{
		{
			name: "list returns a JSON array", handler: handler.HandleBackendsList,
			method: http.MethodGet, path: collection,
			rpc: "GetBackends", rpcRequest: &adminpb.Empty{},
			rpcResponse: &adminpb.GetBackendsResponse{Backends: []*adminpb.BackendResponse{backend}},
			wantStatus:  http.StatusOK, wantJSON: "[" + backendJSON + "]",
		},
		{
			name: "empty list returns an empty array", handler: handler.HandleBackendsList,
			method: http.MethodGet, path: collection,
			rpc: "GetBackends", rpcRequest: &adminpb.Empty{}, rpcResponse: &adminpb.GetBackendsResponse{},
			wantStatus: http.StatusOK, wantJSON: "[]",
		},
		{
			name: "get returns backend and health fields", handler: handler.HandleBackendGet,
			method: http.MethodGet, path: item, pathParam: "id", pathValue: backendID,
			rpc: "GetBackend", rpcRequest: &adminpb.GetBackendRequest{Id: backendID}, rpcResponse: backend,
			wantStatus: http.StatusOK, wantJSON: backendJSON,
		},
		{
			name: "add decodes backend fields", handler: handler.HandleBackendAdd,
			method: http.MethodPost, path: collection, body: addBody,
			rpc: "AddBackend", rpcRequest: addRequest,
			rpcResponse: &adminpb.BasicResponse{Success: true, Message: "Backend added successfully"},
			wantStatus:  http.StatusOK, wantJSON: `{"success":true,"message":"Backend added successfully"}`,
		},
		{
			name: "update uses path id and defaults omitted fields", handler: handler.HandleBackendUpdate,
			method: http.MethodPut, path: item, pathParam: "id", pathValue: backendID,
			body: `{"id":"body-id","weight":4}`,
			rpc:  "UpdateBackend", rpcRequest: &adminpb.UpdateBackendRequest{Id: backendID, Weight: 4},
			rpcResponse: &adminpb.BasicResponse{Success: true, Message: "Backend updated successfully"},
			wantStatus:  http.StatusOK, wantJSON: `{"success":true,"message":"Backend updated successfully"}`,
		},
		{
			name: "delete passes path id", handler: handler.HandleBackendDelete,
			method: http.MethodDelete, path: item, pathParam: "id", pathValue: backendID,
			rpc: "RemoveBackend", rpcRequest: &adminpb.RemoveBackendRequest{Id: backendID},
			rpcResponse: &adminpb.BasicResponse{Success: true, Message: "Backend removed successfully"},
			wantStatus:  http.StatusOK, wantJSON: `{"success":true,"message":"Backend removed successfully"}`,
		},
		{
			name: "malformed create JSON never calls RPC", handler: handler.HandleBackendAdd,
			method: http.MethodPost, path: collection, body: `{"id":`,
			wantStatus: http.StatusBadRequest, wantText: "Invalid request body: unexpected EOF",
		},
		{
			name: "update rejects an extra JSON value", handler: handler.HandleBackendUpdate,
			method: http.MethodPut, path: item, pathParam: "id", pathValue: backendID,
			body:       `{"weight":4}{}`,
			wantStatus: http.StatusBadRequest, wantText: "Invalid request body: expected a single JSON value",
		},
		{
			name: "body exceeding the HTTP limit never calls RPC", handler: handler.HandleBackendAdd,
			method: http.MethodPost, path: collection, body: addBody + " ", bodyLimit: int64(len(addBody)),
			wantStatus: http.StatusRequestEntityTooLarge, wantText: "Request body too large",
		},
		{
			name: "missing update id never calls RPC", handler: handler.HandleBackendUpdate,
			method: http.MethodPut, path: collection, body: `{"weight":4}`,
			wantStatus: http.StatusBadRequest, wantText: "Missing id in path",
		},
		{
			name: "unknown backend maps to not found", handler: handler.HandleBackendGet,
			method: http.MethodGet, path: item, pathParam: "id", pathValue: backendID,
			rpc: "GetBackend", rpcRequest: &adminpb.GetBackendRequest{Id: backendID},
			rpcError:   status.Error(codes.NotFound, "backend not found"),
			wantStatus: http.StatusNotFound, wantText: "Failed to get backend: backend not found",
		},
		{
			name: "invalid backend id maps to bad request", handler: handler.HandleBackendGet,
			method: http.MethodGet, path: item, pathParam: "id", pathValue: backendID,
			rpc: "GetBackend", rpcRequest: &adminpb.GetBackendRequest{Id: backendID},
			rpcError:   status.Error(codes.InvalidArgument, "invalid backend id"),
			wantStatus: http.StatusBadRequest, wantText: "Failed to get backend: invalid backend id",
		},
		{
			name: "validation failure maps to bad request", handler: handler.HandleBackendAdd,
			method: http.MethodPost, path: collection, body: `{"id":"backend-1","url":"http://backend-a:8080"}`,
			rpc: "AddBackend", rpcRequest: &adminpb.AddBackendRequest{Id: backendID, Url: backendURL},
			rpcResponse: &adminpb.BasicResponse{Error: "weight must be positive"},
			wantStatus:  http.StatusBadRequest, wantText: "Failed to add backend: weight must be positive",
		},
		{
			name: "duplicate backend maps to conflict", handler: handler.HandleBackendAdd,
			method: http.MethodPost, path: collection, body: addBody,
			rpc: "AddBackend", rpcRequest: addRequest,
			rpcResponse: &adminpb.BasicResponse{Error: "backend already exists: backend-1"},
			wantStatus:  http.StatusConflict, wantText: "Failed to add backend: backend already exists: backend-1",
		},
		{
			name: "deleting unknown backend maps to not found", handler: handler.HandleBackendDelete,
			method: http.MethodDelete, path: item, pathParam: "id", pathValue: backendID,
			rpc: "RemoveBackend", rpcRequest: &adminpb.RemoveBackendRequest{Id: backendID},
			rpcResponse: &adminpb.BasicResponse{Error: "backend does not exist in config: backend-1"},
			wantStatus:  http.StatusNotFound, wantText: "Failed to remove backend: backend does not exist in config: backend-1",
		},
		{
			name: "unavailable service maps to internal error", handler: handler.HandleBackendsList,
			method: http.MethodGet, path: collection,
			rpc: "GetBackends", rpcRequest: &adminpb.Empty{},
			rpcError:   status.Error(codes.Unavailable, "control plane unavailable"),
			wantStatus: http.StatusInternalServerError, wantText: "Failed to get backends: control plane unavailable",
		},
	})
}
