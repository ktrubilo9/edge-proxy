package grpc

import (
	"edge-proxy/internal/api/adminpb"
	"edge-proxy/internal/config"
	"edge-proxy/internal/proxy/runtime"
)

type AdminGRPCServer struct {
	adminpb.UnimplementedProxyAdminServer
	Runtime *runtime.Runtime
}

func backendToPB(b config.BackendResponse) *adminpb.BackendResponse {
	return &adminpb.BackendResponse{
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
		RatePerIp: int32(r.RatePerIP),
		Burst:     int32(r.Burst),
		WindowSec: int32(r.WindowSec),
	}
}

func RateLimitingFromPB(pb *adminpb.RateLimitingConfig) config.RateLimitingConfig {
	return config.RateLimitingConfig{
		Enabled:   pb.Enabled,
		RatePerIP: pb.RatePerIp,
		Burst:     pb.Burst,
		WindowSec: pb.WindowSec,
	}
}

func VhostToPB(v config.VirtualHostResponse) *adminpb.VirtualHost {
	backends := append([]string(nil), v.Backends...)
	pathRoutes := make([]*adminpb.PathRoute, 0, len(v.PathRoutes))
	for _, route := range v.PathRoutes {
		pathRoutes = append(pathRoutes, &adminpb.PathRoute{
			Path:        route.Path,
			Backends:    route.Backends,
			StripPrefix: route.StripPrefix,
		})
	}

	return &adminpb.VirtualHost{
		Domain:         v.Domain,
		Backends:       backends,
		PathRoutes:     pathRoutes,
		SecurityConfig: SecurityToPB(v.Security),
	}
}

func VhostFromPB(pb *adminpb.VirtualHost) config.VirtualHost {
	backends := append([]string(nil), pb.Backends...)

	pathRoutes := make([]config.PathRoute, 0, len(pb.PathRoutes))
	for _, route := range pb.PathRoutes {
		pathRoutes = append(pathRoutes, config.PathRoute{
			Path:        route.Path,
			Backends:    route.Backends,
			StripPrefix: route.StripPrefix,
		})
	}

	var security *config.SecurityConfig
	if pb.SecurityConfig != nil {
		var rateLimiting config.RateLimitingConfig
		if pb.SecurityConfig.RateLimiting != nil {
			rateLimiting = config.RateLimitingConfig{
				Enabled:   pb.SecurityConfig.RateLimiting.Enabled,
				RatePerIP: pb.SecurityConfig.RateLimiting.RatePerIp,
				Burst:     pb.SecurityConfig.RateLimiting.Burst,
				WindowSec: pb.SecurityConfig.RateLimiting.WindowSec,
			}
		}
		security = &config.SecurityConfig{
			RateLimiting: rateLimiting,
		}
	}

	return config.VirtualHost{
		Domain:     pb.Domain,
		Backends:   backends,
		PathRoutes: pathRoutes,
		Security:   security,
	}
}

func SecurityToPB(sec *config.SecurityConfig) *adminpb.SecurityConfigResponse {
	if sec == nil {
		return &adminpb.SecurityConfigResponse{
			RateLimiting: &adminpb.RateLimitingConfig{},
		}
	}

	return &adminpb.SecurityConfigResponse{
		RateLimiting: &adminpb.RateLimitingConfig{
			Enabled:   sec.RateLimiting.Enabled,
			RatePerIp: int32(sec.RateLimiting.RatePerIP),
			Burst:     int32(sec.RateLimiting.Burst),
			WindowSec: int32(sec.RateLimiting.WindowSec),
		},
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
