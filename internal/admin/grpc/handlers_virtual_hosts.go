package grpc

import (
	"context"
	"edge-proxy/internal/api/adminpb"
	"edge-proxy/internal/config"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *AdminGRPCServer) GetVirtualHosts(ctx context.Context, _ *adminpb.Empty) (*adminpb.GetVirtualHostsResponse, error) {
	vhosts := s.Runtime.GetVirtualHosts()
	resp := &adminpb.GetVirtualHostsResponse{}
	for _, v := range vhosts {
		resp.VirtualHosts = append(resp.VirtualHosts, VhostToPB(v))
	}
	return resp, nil
}

func (s *AdminGRPCServer) GetVirtualHost(ctx context.Context, req *adminpb.GetVirtualHostRequest) (*adminpb.VirtualHost, error) {
	vhost := s.Runtime.GetVirtualHost(req.Domain)
	if vhost == nil {
		return nil, status.Error(codes.NotFound, "virtual host not found")
	}
	return VhostToPB(*vhost), nil
}

func (s *AdminGRPCServer) AddVirtualHost(ctx context.Context, req *adminpb.AddVirtualHostRequest) (*adminpb.BasicResponse, error) {
	if err := s.Runtime.AddVirtualHost(VhostFromPB(req.Vhost)); err != nil {
		return fail(err.Error()), nil
	}
	return success("Virtual host added"), nil
}

func (s *AdminGRPCServer) RemoveVirtualHost(ctx context.Context, req *adminpb.RemoveVirtualHostRequest) (*adminpb.BasicResponse, error) {
	if err := s.Runtime.RemoveVirtualHost(req.Domain); err != nil {
		return fail(err.Error()), nil
	}
	return success("Virtual host removed"), nil
}

func (s *AdminGRPCServer) UpdateVirtualHost(ctx context.Context, req *adminpb.UpdateVirtualHostRequest) (*adminpb.BasicResponse, error) {
	if err := s.Runtime.UpdateVirtualHost(req.Domain, VhostFromPB(req.Vhost)); err != nil {
		return fail(err.Error()), nil
	}
	return success("Virtual host updated"), nil
}

func (s *AdminGRPCServer) GetVirtualHostSecurityConfig(ctx context.Context, req *adminpb.GetVirtualHostRequest) (*adminpb.SecurityConfigResponse, error) {
	if req.Domain == "" {
		return nil, status.Error(codes.InvalidArgument, "domain required")
	}
	sec := s.Runtime.GetVirtualHostSecurityConfig(req.Domain)
	if sec == nil {
		return nil, status.Error(codes.NotFound, "virtual host not found")
	}

	return SecurityToPB(sec), nil
}

func (s *AdminGRPCServer) UpdateVirtualHostSecurityConfig(ctx context.Context, req *adminpb.UpdateSecurityConfigRequest) (*adminpb.BasicResponse, error) {
	if req.Config == nil || req.Domain == "" {
		return fail("domain and config are required"), nil
	}

	if s.Runtime.GetVirtualHost(req.Domain) == nil {
		return fail("virtual host not found"), nil
	}

	cfg := req.Config

	if cfg.RateLimiting != nil {
		if err := s.Runtime.UpdateVirtualHostRateLimiting(req.Domain, config.RateLimitingConfig{
			Enabled:   cfg.RateLimiting.Enabled,
			RatePerIP: cfg.RateLimiting.RatePerIp,
			Burst:     cfg.RateLimiting.Burst,
			WindowSec: cfg.RateLimiting.WindowSec,
		}); err != nil {
			return fail(err.Error()), nil
		}
	}

	return success("security config updated"), nil
}
