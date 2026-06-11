package grpc

import (
	"edge-proxy/internal/admin"
	"edge-proxy/internal/api/adminpb"
	"edge-proxy/internal/logger"
	"edge-proxy/internal/proxy/runtime"
	"errors"
	"net"
	"os"

	"google.golang.org/grpc"
)

const DefaultAdminAddr = "127.0.0.1:50051"

func NewAdminGRPCServer(rt *runtime.Runtime) (*grpc.Server, net.Listener, error) {
	addr := os.Getenv("ADMIN_GRPC_ADDR")
	if addr == "" {
		addr = DefaultAdminAddr
	}
	token := os.Getenv("ADMIN_GRPC_TOKEN")
	if token == "" {
		return nil, nil, errors.New("ADMIN_GRPC_TOKEN is required")
	}

	logger.Info("Starting Admin gRPC server", map[string]interface{}{
		"port": addr,
	})

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("Failed to listen", map[string]interface{}{
			"error": err.Error(),
			"port":  addr,
		})
		return nil, nil, err
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(admin.NewGRPCAuthServerInterceptor(token)),
	)
	adminpb.RegisterProxyAdminServer(grpcServer, &AdminGRPCServer{Runtime: rt})

	return grpcServer, lis, nil
}

func ServeAdminGRPC(grpcServer *grpc.Server, lis net.Listener) error {
	logger.Info("Admin gRPC server started", map[string]interface{}{
		"port": lis.Addr().String(),
	})
	return grpcServer.Serve(lis)
}
