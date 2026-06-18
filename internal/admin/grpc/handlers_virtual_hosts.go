package grpc

import (
	"context"
	"edge-proxy/internal/api/adminpb"

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
	if err := s.Runtime.AddVirtualHost(VhostFromPB(req.VirtualHost)); err != nil {
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
	if err := s.Runtime.UpdateVirtualHost(req.Domain, VhostFromPB(req.VirtualHost)); err != nil {
		return fail(err.Error()), nil
	}
	return success("Virtual host updated"), nil
}

func (s *AdminGRPCServer) GetVirtualHostSecurity(ctx context.Context, req *adminpb.GetVirtualHostRequest) (*adminpb.VirtualHostSecurityResponse, error) {
	if req.Domain == "" {
		return nil, status.Error(codes.InvalidArgument, "domain required")
	}
	sec := s.Runtime.GetVirtualHostSecurity(req.Domain)
	if sec == nil {
		return nil, status.Error(codes.NotFound, "virtual host not found")
	}

	return VirtualHostSecurityToPB(*sec), nil
}

func (s *AdminGRPCServer) SetVirtualHostSecurityPolicy(ctx context.Context, req *adminpb.SetVirtualHostSecurityPolicyRequest) (*adminpb.BasicResponse, error) {
	if req.Domain == "" || req.PolicyId == "" {
		return fail("domain and policy_id are required"), nil
	}
	if err := s.Runtime.SetVirtualHostSecurityPolicy(req.Domain, req.PolicyId); err != nil {
		return fail(err.Error()), nil
	}
	return success("virtual host security policy updated"), nil
}

func (s *AdminGRPCServer) GetPolicies(ctx context.Context, _ *adminpb.Empty) (*adminpb.GetPoliciesResponse, error) {
	return PoliciesToPB(s.Runtime.GetPolicies()), nil
}

func (s *AdminGRPCServer) UpsertPolicy(ctx context.Context, req *adminpb.UpsertPolicyRequest) (*adminpb.BasicResponse, error) {
	if req == nil || req.Policy == nil {
		return fail("policy is required"), nil
	}
	if err := s.Runtime.UpsertPolicy(SecurityPolicyFromPB(req.Policy)); err != nil {
		return fail(err.Error()), nil
	}
	return success("security policy saved"), nil
}
