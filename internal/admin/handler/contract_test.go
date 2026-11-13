package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ktrubilo9/edge-proxy/internal/api/adminpb"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type adminHTTPCase struct {
	name        string
	handler     func(adminpb.ProxyAdminClient) http.HandlerFunc
	method      string
	path        string
	pathParam   string
	pathValue   string
	body        string
	bodyLimit   int64
	rpc         string
	rpcRequest  proto.Message
	rpcResponse proto.Message
	rpcError    error
	wantStatus  int
	wantJSON    string
	wantText    string
}

func runAdminHTTPCases(t *testing.T, cases []adminHTTPCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeAdminClient{t: t, test: tc}
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.pathParam != "" {
				request.SetPathValue(tc.pathParam, tc.pathValue)
			}

			recorder := httptest.NewRecorder()
			var handler http.Handler = tc.handler(client)
			if tc.bodyLimit > 0 {
				handler = http.MaxBytesHandler(handler, tc.bodyLimit)
			}
			handler.ServeHTTP(recorder, request)

			if tc.rpc != "" && !client.called {
				t.Errorf("expected RPC %s to be called", tc.rpc)
			}
			if recorder.Code != tc.wantStatus {
				t.Errorf("HTTP status = %d, want %d; body = %s", recorder.Code, tc.wantStatus, recorder.Body)
			}
			wantContentType := "text/plain; charset=utf-8"
			if tc.wantJSON != "" {
				wantContentType = "application/json"
			}
			if got := recorder.Header().Get("Content-Type"); got != wantContentType {
				t.Errorf("Content-Type = %q, want %q", got, wantContentType)
			}
			if tc.wantJSON != "" {
				assertJSON(t, recorder.Body.String(), tc.wantJSON)
			} else if got := recorder.Body.String(); got != tc.wantText+"\n" {
				t.Errorf("body = %q, want %q", got, tc.wantText+"\n")
			}
		})
	}
}

func assertJSON(t *testing.T, got, want string) {
	t.Helper()
	var actual, expected any
	if err := json.Unmarshal([]byte(got), &actual); err != nil {
		t.Fatalf("decode response JSON %q: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		t.Fatalf("decode expected JSON %q: %v", want, err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("response JSON = %s, want %s", got, want)
	}
}

// Embed the interface for RPCs outside the backend and virtual-host tests.
type fakeAdminClient struct {
	adminpb.ProxyAdminClient
	t      *testing.T
	test   adminHTTPCase
	called bool
}

func fakeReply[T proto.Message](f *fakeAdminClient, method string, request proto.Message) (T, error) {
	f.t.Helper()
	if f.called || method != f.test.rpc {
		f.t.Fatalf("unexpected RPC %s; want %q called once", method, f.test.rpc)
	}
	f.called = true
	if !proto.Equal(request, f.test.rpcRequest) {
		f.t.Errorf("%s request = %v, want %v", method, request, f.test.rpcRequest)
	}
	if f.test.rpcError != nil {
		var zero T
		return zero, f.test.rpcError
	}
	return f.test.rpcResponse.(T), nil
}

func (f *fakeAdminClient) GetBackends(_ context.Context, req *adminpb.Empty, _ ...grpc.CallOption) (*adminpb.GetBackendsResponse, error) {
	return fakeReply[*adminpb.GetBackendsResponse](f, "GetBackends", req)
}

func (f *fakeAdminClient) GetBackend(_ context.Context, req *adminpb.GetBackendRequest, _ ...grpc.CallOption) (*adminpb.BackendResponse, error) {
	return fakeReply[*adminpb.BackendResponse](f, "GetBackend", req)
}

func (f *fakeAdminClient) AddBackend(_ context.Context, req *adminpb.AddBackendRequest, _ ...grpc.CallOption) (*adminpb.BasicResponse, error) {
	return fakeReply[*adminpb.BasicResponse](f, "AddBackend", req)
}

func (f *fakeAdminClient) UpdateBackend(_ context.Context, req *adminpb.UpdateBackendRequest, _ ...grpc.CallOption) (*adminpb.BasicResponse, error) {
	return fakeReply[*adminpb.BasicResponse](f, "UpdateBackend", req)
}

func (f *fakeAdminClient) RemoveBackend(_ context.Context, req *adminpb.RemoveBackendRequest, _ ...grpc.CallOption) (*adminpb.BasicResponse, error) {
	return fakeReply[*adminpb.BasicResponse](f, "RemoveBackend", req)
}

func (f *fakeAdminClient) GetVirtualHosts(_ context.Context, req *adminpb.Empty, _ ...grpc.CallOption) (*adminpb.GetVirtualHostsResponse, error) {
	return fakeReply[*adminpb.GetVirtualHostsResponse](f, "GetVirtualHosts", req)
}

func (f *fakeAdminClient) GetVirtualHost(_ context.Context, req *adminpb.GetVirtualHostRequest, _ ...grpc.CallOption) (*adminpb.VirtualHost, error) {
	return fakeReply[*adminpb.VirtualHost](f, "GetVirtualHost", req)
}

func (f *fakeAdminClient) AddVirtualHost(_ context.Context, req *adminpb.AddVirtualHostRequest, _ ...grpc.CallOption) (*adminpb.BasicResponse, error) {
	return fakeReply[*adminpb.BasicResponse](f, "AddVirtualHost", req)
}

func (f *fakeAdminClient) UpdateVirtualHost(_ context.Context, req *adminpb.UpdateVirtualHostRequest, _ ...grpc.CallOption) (*adminpb.BasicResponse, error) {
	return fakeReply[*adminpb.BasicResponse](f, "UpdateVirtualHost", req)
}

func (f *fakeAdminClient) RemoveVirtualHost(_ context.Context, req *adminpb.RemoveVirtualHostRequest, _ ...grpc.CallOption) (*adminpb.BasicResponse, error) {
	return fakeReply[*adminpb.BasicResponse](f, "RemoveVirtualHost", req)
}
