package grpc

import (
	"context"
	"edge-proxy/internal/api/adminpb"
	"edge-proxy/internal/config"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *AdminGRPCServer) AddBackend(ctx context.Context, req *adminpb.AddBackendRequest) (*adminpb.BasicResponse, error) {
	if req.Id == "" {
		return fail("id cannot be empty"), nil
	}
	if req.Url == "" {
		return fail("url cannot be empty"), nil
	}
	if req.Weight <= 0 {
		return fail("weight must be positive"), nil
	}
	if err := s.Runtime.AddBackend(config.BackendConfig{
		Id:      req.Id,
		URL:     req.Url,
		Weight:  req.Weight,
		Enabled: req.Enabled,
	}); err != nil {
		return fail(err.Error()), nil
	}
	return success("Backend added successfully"), nil
}

func (s *AdminGRPCServer) UpdateBackend(ctx context.Context, req *adminpb.UpdateBackendRequest) (*adminpb.BasicResponse, error) {
	if req.Id == "" {
		return fail("id cannot be empty"), nil
	}
	if req.Weight <= 0 {
		return fail("weight must be positive"), nil
	}
	if err := s.Runtime.UpdateBackend(req.Id, req.Url, req.Weight, req.Enabled); err != nil {
		return fail(err.Error()), nil
	}
	return success("Backend updated successfully"), nil
}

func (s *AdminGRPCServer) RemoveBackend(ctx context.Context, req *adminpb.RemoveBackendRequest) (*adminpb.BasicResponse, error) {
	if req.Id == "" {
		return fail("id cannot be empty"), nil
	}
	if err := s.Runtime.RemoveBackend(req.Id); err != nil {
		return fail(err.Error()), nil
	}
	return success("Backend removed successfully"), nil
}

func (s *AdminGRPCServer) GetBackends(ctx context.Context, _ *adminpb.Empty) (*adminpb.GetBackendsResponse, error) {
	backends := s.Runtime.GetBackendsResponse()
	resp := &adminpb.GetBackendsResponse{}
	for _, b := range backends {
		resp.Backends = append(resp.Backends, backendToPB(b))
	}
	return resp, nil
}

func (s *AdminGRPCServer) GetBackend(ctx context.Context, req *adminpb.GetBackendRequest) (*adminpb.BackendResponse, error) {
	backend := s.Runtime.GetBackendResponse(req.Id)
	if backend == nil {
		return nil, status.Error(codes.InvalidArgument, "id does not exists")
	}
	resp := &adminpb.BackendResponse{
		Id:         backend.Id,
		Url:        backend.URL,
		Weight:     backend.Weight,
		Enabled:    backend.Enabled,
		Active:     backend.Active,
		ErrorCount: backend.ErrorCount,
		LastError:  backend.LastError,
	}
	return resp, nil
}
