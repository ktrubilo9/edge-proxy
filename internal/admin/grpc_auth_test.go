package admin

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestGRPCAuthServerInterceptor(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		header  string
		wantErr codes.Code
	}{
		{
			name:    "accepts valid bearer token",
			token:   "secret",
			header:  "Bearer secret",
			wantErr: codes.OK,
		},
		{
			name:    "rejects missing metadata",
			token:   "secret",
			wantErr: codes.Unauthenticated,
		},
		{
			name:    "rejects invalid token",
			token:   "secret",
			header:  "Bearer wrong",
			wantErr: codes.Unauthenticated,
		},
		{
			name:    "rejects empty configured token",
			header:  "Bearer secret",
			wantErr: codes.Unauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.header != "" {
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(grpcAuthorizationHeader, tt.header))
			}

			handlerCalled := false
			interceptor := NewGRPCAuthServerInterceptor(tt.token)
			_, err := interceptor(ctx, struct{}{}, nil, func(context.Context, interface{}) (interface{}, error) {
				handlerCalled = true
				return struct{}{}, nil
			})

			if got := status.Code(err); got != tt.wantErr {
				t.Fatalf("status code = %v, want %v", got, tt.wantErr)
			}
			if handlerCalled != (tt.wantErr == codes.OK) {
				t.Fatalf("handlerCalled = %v", handlerCalled)
			}
		})
	}
}

func TestGRPCAuthClientInterceptorAddsBearerToken(t *testing.T) {
	interceptor := NewGRPCAuthClientInterceptor("secret")

	err := interceptor(
		context.Background(),
		"/admin.ProxyAdmin/GetBackends",
		struct{}{},
		struct{}{},
		nil,
		func(ctx context.Context, _ string, _ interface{}, _ interface{}, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			md, ok := metadata.FromOutgoingContext(ctx)
			if !ok {
				t.Fatal("missing outgoing metadata")
			}
			values := md.Get(grpcAuthorizationHeader)
			if len(values) != 1 || values[0] != "Bearer secret" {
				t.Fatalf("authorization metadata = %v", values)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
}
