package grpc

import (
	"context"
	"edge-proxy/internal/api/adminpb"
)

func (s *AdminGRPCServer) GetGlobalConfig(ctx context.Context, _ *adminpb.Empty) (*adminpb.GlobalConfig, error) {
	cfg := s.Runtime.GetGlobalConfig()
	return &adminpb.GlobalConfig{
		ProxyPort:  int32(cfg.ProxyPort),
		LbStrategy: cfg.LBStrategy,
	}, nil
}

func (s *AdminGRPCServer) SetGlobalConfig(ctx context.Context, cfg *adminpb.GlobalConfig) (*adminpb.BasicResponse, error) {
	if cfg.ProxyPort != 0 && (cfg.ProxyPort < 1 || cfg.ProxyPort > 65535) {
		return fail("invalid proxy port"), nil
	}
	if cfg.LbStrategy == "" {
		return fail("invalid load balancer strategy"), nil
	}

	if err := s.Runtime.UpdateGlobalConfig(cfg.ProxyPort, cfg.LbStrategy); err != nil {
		return fail(err.Error()), nil
	}
	return success("Global config updated"), nil
}
