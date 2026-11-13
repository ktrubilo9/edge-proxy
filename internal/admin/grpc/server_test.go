package grpc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ktrubilo9/edge-proxy/internal/api/adminpb"
	"github.com/ktrubilo9/edge-proxy/internal/proxy/runtime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	t.Cleanup(func() {
		if closeErr := listener.Close(); closeErr != nil {
			t.Errorf("close admin listener: %v", closeErr)
		}
	})
	server.Stop()
}

func TestBackendAndVirtualHostRPCFailures(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	const config = `{
		"backends": [{"id": "backend-1", "url": "http://backend-a:8080", "weight": 1, "enabled": true}]
	}`
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	rt, err := runtime.NewRuntime(configPath)
	if err != nil {
		t.Fatal(err)
	}
	server := &AdminGRPCServer{Runtime: rt}
	ctx := context.Background()

	t.Run("unknown backend", func(t *testing.T) {
		_, err := server.GetBackend(ctx, &adminpb.GetBackendRequest{Id: "missing"})
		if got := status.Code(err); got != codes.NotFound {
			t.Errorf("GetBackend status = %s, want %s", got, codes.NotFound)
		}
	})

	// Prevent SaveConfig from creating its temporary file on every platform.
	if err := os.Mkdir(configPath+".tmp", 0700); err != nil {
		t.Fatal(err)
	}

	t.Run("backend persistence failure", func(t *testing.T) {
		_, err := server.UpdateBackend(ctx, &adminpb.UpdateBackendRequest{
			Id: "backend-1", Weight: 2, Enabled: true,
		})
		if got := status.Code(err); got != codes.Internal {
			t.Errorf("UpdateBackend status = %s, want %s", got, codes.Internal)
		}
	})

	t.Run("virtual host persistence failure", func(t *testing.T) {
		_, err := server.AddVirtualHost(ctx, &adminpb.AddVirtualHostRequest{
			VirtualHost: &adminpb.VirtualHost{Domain: "app.example.local", BackendIds: []string{"backend-1"}},
		})
		if got := status.Code(err); got != codes.Internal {
			t.Errorf("AddVirtualHost status = %s, want %s", got, codes.Internal)
		}
	})
}
