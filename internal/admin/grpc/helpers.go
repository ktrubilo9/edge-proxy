package grpc

import (
	"edge-proxy/internal/api/adminpb"
	"edge-proxy/internal/config"
	"edge-proxy/internal/proxy/runtime"
	"edge-proxy/internal/view"
)

type AdminGRPCServer struct {
	adminpb.UnimplementedProxyAdminServer
	Runtime *runtime.Runtime
}

func backendToPB(b view.BackendResponse) *adminpb.BackendResponse {
	return &adminpb.BackendResponse{
		Id:         b.Id,
		Url:        b.URL,
		Weight:     b.Weight,
		Enabled:    b.Enabled,
		Active:     b.Active,
		ErrorCount: b.ErrorCount,
		LastError:  b.LastError,
	}
}

func RateLimitingToPB(r config.RateLimitingConfig) *adminpb.RateLimitingConfig {
	return &adminpb.RateLimitingConfig{
		Enabled:   r.Enabled,
		RatePerIp: r.RatePerIP,
		Burst:     r.Burst,
		WindowSec: r.WindowSec,
	}
}

func RateLimitingFromPB(pb *adminpb.RateLimitingConfig) config.RateLimitingConfig {
	if pb == nil {
		return config.RateLimitingConfig{}
	}

	return config.RateLimitingConfig{
		Enabled:   pb.Enabled,
		RatePerIP: pb.RatePerIp,
		Burst:     pb.Burst,
		WindowSec: pb.WindowSec,
	}
}

func VhostToPB(v view.VirtualHostResponse) *adminpb.VirtualHost {
	pathRoutes := make([]*adminpb.PathRoute, 0, len(v.PathRoutes))
	for _, route := range v.PathRoutes {
		pathRoutes = append(pathRoutes, &adminpb.PathRoute{
			Path:        route.Path,
			BackendIds:  append([]string(nil), route.BackendIDs...),
			StripPrefix: route.StripPrefix,
		})
	}

	return &adminpb.VirtualHost{
		Domain:           v.Domain,
		BackendIds:       append([]string(nil), v.BackendIDs...),
		PathRoutes:       pathRoutes,
		SecurityPolicyId: v.SecurityPolicyID,
	}
}

func VhostFromPB(pb *adminpb.VirtualHost) config.VirtualHost {
	if pb == nil {
		return config.VirtualHost{}
	}

	pathRoutes := make([]config.PathRoute, 0, len(pb.PathRoutes))
	for _, route := range pb.PathRoutes {
		pathRoutes = append(pathRoutes, config.PathRoute{
			Path:        route.Path,
			BackendIDs:  append([]string(nil), route.BackendIds...),
			StripPrefix: route.StripPrefix,
		})
	}

	return config.VirtualHost{
		Domain:           pb.Domain,
		BackendIDs:       append([]string(nil), pb.BackendIds...),
		PathRoutes:       pathRoutes,
		SecurityPolicyID: pb.SecurityPolicyId,
	}
}

func SecurityPolicyToPB(policy view.SecurityPolicyResponse) *adminpb.SecurityPolicy {
	return &adminpb.SecurityPolicy{
		Id:           policy.Id,
		RateLimiting: RateLimitingToPB(policy.RateLimiting),
	}
}

func SecurityPolicyFromPB(pb *adminpb.SecurityPolicy) config.SecurityPolicy {
	if pb == nil {
		return config.SecurityPolicy{}
	}

	return config.SecurityPolicy{
		Id:           pb.Id,
		RateLimiting: RateLimitingFromPB(pb.RateLimiting),
	}
}

func PoliciesToPB(policies []view.SecurityPolicyResponse) *adminpb.GetPoliciesResponse {
	resp := &adminpb.GetPoliciesResponse{
		Policies: make([]*adminpb.SecurityPolicy, 0, len(policies)),
	}
	for _, policy := range policies {
		resp.Policies = append(resp.Policies, SecurityPolicyToPB(policy))
	}
	return resp
}

func VirtualHostSecurityToPB(sec view.VirtualHostSecurityResponse) *adminpb.VirtualHostSecurityResponse {
	return &adminpb.VirtualHostSecurityResponse{
		Domain:           sec.Domain,
		SecurityPolicyId: sec.SecurityPolicyID,
		Policy:           SecurityPolicyToPB(sec.Policy),
	}
}

func ServerConfigToPB(cfg view.ServerConfigResponse) *adminpb.ServerConfig {
	return &adminpb.ServerConfig{
		ProxyPort:     int32(cfg.ProxyPort),
		AdminGrpcPort: int32(cfg.AdminGrpcPort),
	}
}

func LoadBalancerToPB(cfg view.LoadBalancerConfigResponse) *adminpb.LoadBalancerConfig {
	return &adminpb.LoadBalancerConfig{
		Strategy: cfg.Strategy,
	}
}

func fail(msg string) *adminpb.BasicResponse {
	return &adminpb.BasicResponse{
		Success: false,
		Error:   msg,
	}
}

func success(msg string) *adminpb.BasicResponse {
	return &adminpb.BasicResponse{
		Success: true,
		Message: msg,
	}
}
