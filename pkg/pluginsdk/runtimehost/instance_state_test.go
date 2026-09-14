package runtimehost_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtimehost"
)

type instanceStateServer struct {
	pluginv1.UnimplementedRuntimeHostServer
	values   map[string][]byte
	statuses []*pluginv1.NetworkAccessStatus
}

func newInstanceStateServer() *instanceStateServer {
	return &instanceStateServer{values: map[string][]byte{}}
}

func (s *instanceStateServer) ReadInstanceState(_ context.Context, req *pluginv1.ReadInstanceStateRequest) (*pluginv1.ReadInstanceStateResponse, error) {
	value, ok := s.values[req.GetKey()]
	if !ok {
		return &pluginv1.ReadInstanceStateResponse{}, nil
	}
	return &pluginv1.ReadInstanceStateResponse{Value: value, Found: true}, nil
}

func (s *instanceStateServer) WriteInstanceState(_ context.Context, req *pluginv1.WriteInstanceStateRequest) (*pluginv1.WriteInstanceStateResponse, error) {
	s.values[req.GetKey()] = append([]byte(nil), req.GetValue()...)
	return &pluginv1.WriteInstanceStateResponse{}, nil
}

func (s *instanceStateServer) ReportNetworkAccessStatus(_ context.Context, req *pluginv1.ReportNetworkAccessStatusRequest) (*pluginv1.ReportNetworkAccessStatusResponse, error) {
	s.statuses = append(s.statuses, req.GetStatus())
	return &pluginv1.ReportNetworkAccessStatusResponse{}, nil
}

func dialInstanceState(t *testing.T, srv *instanceStateServer) *runtimehost.Client {
	t.Helper()
	return runtimehost.NewClient(dialServer(t, srv))
}

func TestInstanceState_RoundTrip(t *testing.T) {
	srv := newInstanceStateServer()
	c := dialInstanceState(t, srv)
	ctx := context.Background()

	_, found, err := c.ReadInstanceState(ctx, "_machinekey")
	if err != nil || found {
		t.Fatalf("ReadInstanceState before write = found %v, err %v; want absent", found, err)
	}
	if err := c.WriteInstanceState(ctx, "_machinekey", []byte("secret")); err != nil {
		t.Fatalf("WriteInstanceState: %v", err)
	}
	value, found, err := c.ReadInstanceState(ctx, "_machinekey")
	if err != nil || !found || string(value) != "secret" {
		t.Fatalf("ReadInstanceState after write = %q found %v err %v", value, found, err)
	}
	// An empty stored value is still found.
	if err := c.WriteInstanceState(ctx, "empty", nil); err != nil {
		t.Fatalf("WriteInstanceState empty: %v", err)
	}
	if _, found, err := c.ReadInstanceState(ctx, "empty"); err != nil || !found {
		t.Fatalf("empty value: found %v err %v, want found", found, err)
	}
}

func TestInstanceState_EnforcesLimitsClientSide(t *testing.T) {
	srv := newInstanceStateServer()
	c := dialInstanceState(t, srv)
	ctx := context.Background()

	if _, _, err := c.ReadInstanceState(ctx, ""); err == nil {
		t.Fatal("empty key accepted")
	}
	longKey := strings.Repeat("k", runtimehost.MaxInstanceStateKeyBytes+1)
	if err := c.WriteInstanceState(ctx, longKey, []byte("x")); err == nil {
		t.Fatal("oversized key accepted")
	}
	big := bytes.Repeat([]byte("v"), runtimehost.MaxInstanceStateValueBytes+1)
	if err := c.WriteInstanceState(ctx, "big", big); err == nil {
		t.Fatal("oversized value accepted")
	}
	if len(srv.values) != 0 {
		t.Fatalf("rejected writes reached the host: %v", srv.values)
	}
	limit := bytes.Repeat([]byte("v"), runtimehost.MaxInstanceStateValueBytes)
	if err := c.WriteInstanceState(ctx, "limit", limit); err != nil {
		t.Fatalf("value at the limit rejected: %v", err)
	}
}

func TestInstanceStateStore_ImplementsStateStoreShape(t *testing.T) {
	srv := newInstanceStateServer()
	c := dialInstanceState(t, srv)
	var store runtimehost.StateStore = c.InstanceStateStore()

	if _, err := store.ReadState("_profiles"); !errors.Is(err, runtimehost.ErrStateNotExist) {
		t.Fatalf("ReadState missing = %v, want ErrStateNotExist", err)
	}
	if err := store.WriteState("_profiles", []byte(`{}`)); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	got, err := store.ReadState("_profiles")
	if err != nil || string(got) != `{}` {
		t.Fatalf("ReadState = %q, %v", got, err)
	}
	if err := c.InstanceStateStore().WithTimeout(0).WriteState("k", []byte("v")); err != nil {
		t.Fatalf("WriteState without deadline: %v", err)
	}
}

func TestReportNetworkAccessStatus(t *testing.T) {
	srv := newInstanceStateServer()
	c := dialInstanceState(t, srv)
	ctx := context.Background()

	if err := c.ReportNetworkAccessStatus(ctx, nil); err == nil {
		t.Fatal("nil status accepted")
	}
	if err := c.ReportNetworkAccessStatus(ctx, &pluginv1.NetworkAccessStatus{}); err == nil {
		t.Fatal("status without state accepted")
	}
	status := &pluginv1.NetworkAccessStatus{
		State:            runtimehost.NetworkAccessStateConnected,
		Hostname:         "silo.tail1234.ts.net",
		Origin:           "https://silo.tail1234.ts.net",
		DesiredConnected: true,
	}
	if err := c.ReportNetworkAccessStatus(ctx, status); err != nil {
		t.Fatalf("ReportNetworkAccessStatus: %v", err)
	}
	if len(srv.statuses) != 1 || srv.statuses[0].GetHostname() != "silo.tail1234.ts.net" || !srv.statuses[0].GetDesiredConnected() {
		t.Fatalf("host received %v", srv.statuses)
	}
}
