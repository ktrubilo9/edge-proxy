package grpc

import (
	"context"
	"edge-proxy/internal/api/adminpb"
)

func (s *AdminGRPCServer) GetServerConfig(ctx context.Context, _ *adminpb.Empty) (*adminpb.ServerConfig, error) {
	return ServerConfigToPB(s.Runtime.GetServerConfig()), nil
}

func (s *AdminGRPCServer) SetServerConfig(ctx context.Context, cfg *adminpb.ServerConfig) (*adminpb.BasicResponse, error) {
	if cfg == nil {
		return fail("server config is required"), nil
	}
	if cfg.ProxyPort != 0 && (cfg.ProxyPort < 1 || cfg.ProxyPort > 65535) {
		return fail("invalid proxy port"), nil
	}
	if cfg.AdminGrpcPort != 0 && (cfg.AdminGrpcPort < 1 || cfg.AdminGrpcPort > 65535) {
		return fail("invalid admin grpc port"), nil
	}

	if err := s.Runtime.UpdateServerConfig(cfg.ProxyPort, cfg.AdminGrpcPort); err != nil {
		return fail(err.Error()), nil
	}
	return success("Server config updated"), nil
}

func (s *AdminGRPCServer) GetLoadBalancer(ctx context.Context, _ *adminpb.Empty) (*adminpb.LoadBalancerConfig, error) {
	return LoadBalancerToPB(s.Runtime.GetLoadBalancerConfig()), nil
}

func (s *AdminGRPCServer) SetLoadBalancer(ctx context.Context, cfg *adminpb.LoadBalancerConfig) (*adminpb.BasicResponse, error) {
	if cfg == nil || cfg.Strategy == "" {
		return fail("load balancer strategy is required"), nil
	}
	if err := s.Runtime.UpdateLoadBalancerConfig(cfg.Strategy); err != nil {
		return fail(err.Error()), nil
	}
	return success("Load balancer config updated"), nil
}
