package grpc

import (
	"edge-proxy/internal/api/adminpb"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type AdminClient struct {
	client adminpb.ProxyAdminClient
	conn   *grpc.ClientConn
}

func ConnectToAdminAPI(address string) (*AdminClient, error) {
	log.Printf("Connecting to admin gRPC at %s", address)
	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
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
