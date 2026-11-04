package runtimehost_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtimehost"
)

type versionedEntry struct {
	value   []byte
	version int64
}

// versionedStateServer keeps versions like a current host. beforeWrite, when
// set, runs once before the next conditional write, to simulate another
// server's write landing in between.
type versionedStateServer struct {
	pluginv1.UnimplementedRuntimeHostServer
	values      map[string]versionedEntry
	beforeWrite func()
}

func (s *versionedStateServer) ReadInstanceState(_ context.Context, req *pluginv1.ReadInstanceStateRequest) (*pluginv1.ReadInstanceStateResponse, error) {
	entry, ok := s.values[req.GetKey()]
	return &pluginv1.ReadInstanceStateResponse{Value: entry.value, Found: ok, Version: entry.version, Versioned: true}, nil
}

func (s *versionedStateServer) WriteInstanceState(_ context.Context, req *pluginv1.WriteInstanceStateRequest) (*pluginv1.WriteInstanceStateResponse, error) {
	if hook := s.beforeWrite; hook != nil {
		s.beforeWrite = nil
		hook()
	}
	current := s.values[req.GetKey()]
	if req.ExpectedVersion != nil && req.GetExpectedVersion() != current.version {
		return nil, status.Error(codes.Aborted, "version changed")
	}
	next := versionedEntry{value: req.GetValue(), version: current.version + 1}
	s.values[req.GetKey()] = next
	return &pluginv1.WriteInstanceStateResponse{Version: next.version}, nil
}

func TestUpdateInstanceStateRedoesTheChangeAfterAConflict(t *testing.T) {
	srv := &versionedStateServer{values: map[string]versionedEntry{"count": {value: []byte("1"), version: 1}}}
	// Another server bumps the count between this client's read and write.
	srv.beforeWrite = func() { srv.values["count"] = versionedEntry{value: []byte("5"), version: 2} }
	c := runtimehost.NewClient(dialServer(t, srv))

	calls := 0
	err := c.UpdateInstanceState(context.Background(), "count", func(current []byte, _ bool) ([]byte, error) {
		calls++
		return append(current, '+'), nil
	})
	if err != nil {
		t.Fatalf("UpdateInstanceState: %v", err)
	}
	if got := srv.values["count"]; string(got.value) != "5+" || got.version != 3 || calls != 2 {
		t.Fatalf("stored %q at version %d after %d calls; want the other write kept, \"5+\" at 3, 2 calls", got.value, got.version, calls)
	}
}

func TestWriteInstanceStateIfConflicts(t *testing.T) {
	srv := &versionedStateServer{values: map[string]versionedEntry{"k": {value: []byte("a"), version: 4}}}
	c := runtimehost.NewClient(dialServer(t, srv))
	// 0 means "create": the key exists, so it conflicts.
	if _, err := c.WriteInstanceStateIf(context.Background(), "k", []byte("b"), 0); !errors.Is(err, runtimehost.ErrInstanceStateConflict) {
		t.Fatalf("WriteInstanceStateIf = %v, want ErrInstanceStateConflict", err)
	}
	if string(srv.values["k"].value) != "a" {
		t.Fatal("a conflicting write changed the value")
	}
}

// An older host would ignore expected_version, so the update must stop at
// the read instead of writing unconditionally.
func TestUpdateInstanceStateOnAnUnversionedHostWritesNothing(t *testing.T) {
	srv := newInstanceStateServer()
	c := dialInstanceState(t, srv)
	err := c.UpdateInstanceState(context.Background(), "k", func([]byte, bool) ([]byte, error) { return []byte("x"), nil })
	if !errors.Is(err, runtimehost.ErrInstanceStateUnversioned) {
		t.Fatalf("UpdateInstanceState = %v, want ErrInstanceStateUnversioned", err)
	}
	if _, wrote := srv.values["k"]; wrote {
		t.Fatal("UpdateInstanceState wrote on a host that can't make writes conditional")
	}
}
