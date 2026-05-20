package grpc

import (
	"context"
	"edge-proxy/internal/api/adminpb"
	"edge-proxy/internal/config"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *AdminGRPCServer) AddBackend(ctx context.Context, req *adminpb.AddBackendRequest) (*adminpb.BasicResponse, error) {
	if req.Url == "" {
		return fail("url cannot be empty"), nil
	}
	if req.Weight < 0 {
		return fail("weight cannot be negative"), nil
	}
	if err := s.Runtime.AddBackend(config.BackendConfig{
		URL:     req.Url,
		Weight:  req.Weight,
		Enabled: true,
	}); err != nil {
		return fail(err.Error()), nil
	}
	return success("Backend added successfully"), nil
}

func (s *AdminGRPCServer) UpdateBackend(ctx context.Context, req *adminpb.UpdateBackendRequest) (*adminpb.BasicResponse, error) {
	if req.Url == "" {
		return fail("url cannot be empty"), nil
	}
	if req.Weight < 0 {
		return fail("weight cannot be negative"), nil
	}
	if err := s.Runtime.UpdateBackend(req.Url, req.Weight, req.Enabled); err != nil {
		return fail(err.Error()), nil
	}
	return success("Backend updated successfully"), nil
}

func (s *AdminGRPCServer) RemoveBackend(ctx context.Context, req *adminpb.RemoveBackendRequest) (*adminpb.BasicResponse, error) {
	if req.Url == "" {
		return fail("url cannot be empty"), nil
	}
	if err := s.Runtime.RemoveBackend(req.Url); err != nil {
		return fail(err.Error()), nil
	}
	return success("Backend removed successfully"), nil
}

func (s *AdminGRPCServer) GetBackends(ctx context.Context, _ *adminpb.Empty) (*adminpb.GetBackendsResponse, error) {
	backends := s.Runtime.GetBackends()
	resp := &adminpb.GetBackendsResponse{}
	for _, b := range backends {
		resp.Backends = append(resp.Backends, backendToPB(b))
	}
	return resp, nil
}

func (s *AdminGRPCServer) GetBackend(ctx context.Context, req *adminpb.GetBackendRequest) (*adminpb.BackendResponse, error) {
	backend := s.Runtime.GetBackend(req.Url)
	if backend == nil {
		return nil, status.Error(codes.InvalidArgument, "url does not exists")
	}
	resp := &adminpb.BackendResponse{
		Url:        backend.URL,
		Weight:     int32(backend.Weight),
		Enabled:    backend.Enabled,
		Active:     backend.Active,
		ErrorCount: backend.ErrorCount,
		LastError:  backend.LastError,
	}
	return resp, nil
}
