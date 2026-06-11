package grpc

import (
	"edge-proxy/internal/admin"
	"edge-proxy/internal/api/adminpb"
	"errors"
	"log"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type AdminClient struct {
	client adminpb.ProxyAdminClient
	conn   *grpc.ClientConn
}

func ConnectToAdminAPI(address string) (*AdminClient, error) {
	token := os.Getenv("ADMIN_GRPC_TOKEN")
	if token == "" {
		return nil, errors.New("ADMIN_GRPC_TOKEN is required")
	}

	log.Printf("Connecting to admin gRPC at %s", address)
	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(admin.NewGRPCAuthClientInterceptor(token)),
	)
	if err != nil {
		log.Printf("Failed to connect: %v", err)
		return nil, err
	}
	log.Printf("Successfully connected to admin gRPC.")

	client := adminpb.NewProxyAdminClient(conn)
	return &AdminClient{
		client: client,
		conn:   conn,
	}, nil
}

func (a *AdminClient) Close() {
	if a.conn != nil {
		a.conn.Close()
	}
}
