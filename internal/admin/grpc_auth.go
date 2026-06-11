package admin

import (
	"context"
	"crypto/subtle"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const grpcAuthorizationHeader = "authorization"

func NewGRPCAuthServerInterceptor(token string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if !validGRPCCredentials(ctx, token) {
			return nil, status.Error(codes.Unauthenticated, "invalid admin credentials")
		}
		return handler(ctx, req)
	}
}

func NewGRPCAuthClientInterceptor(token string) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req interface{},
		reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		ctx = metadata.AppendToOutgoingContext(
			ctx,
			grpcAuthorizationHeader,
			"Bearer "+token,
		)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func validGRPCCredentials(ctx context.Context, expectedToken string) bool {
	if expectedToken == "" {
		return false
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}

	values := md.Get(grpcAuthorizationHeader)
	if len(values) != 1 {
		return false
	}

	parts := strings.Fields(values[0])
	return len(parts) == 2 &&
		strings.EqualFold(parts[0], "Bearer") &&
		subtle.ConstantTimeCompare([]byte(expectedToken), []byte(parts[1])) == 1
}
