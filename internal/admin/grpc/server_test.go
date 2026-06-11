package grpc

import (
	"edge-proxy/internal/proxy/runtime"
	"testing"
)

func TestNewAdminGRPCServerRequiresToken(t *testing.T) {
	t.Setenv("ADMIN_GRPC_ADDR", "127.0.0.1:0")
	t.Setenv("ADMIN_GRPC_TOKEN", "")

	server, listener, err := NewAdminGRPCServer(&runtime.Runtime{})
	if err == nil {
		t.Fatal("expected missing token error")
	}
	if server != nil || listener != nil {
		t.Fatal("server or listener created without authentication token")
	}
}

func TestNewAdminGRPCServerCreatesAuthenticatedServer(t *testing.T) {
	t.Setenv("ADMIN_GRPC_ADDR", "127.0.0.1:0")
	t.Setenv("ADMIN_GRPC_TOKEN", "secret")

	server, listener, err := NewAdminGRPCServer(&runtime.Runtime{})
	if err != nil {
		t.Fatalf("NewAdminGRPCServer returned error: %v", err)
	}
	defer listener.Close()
	server.Stop()
}
