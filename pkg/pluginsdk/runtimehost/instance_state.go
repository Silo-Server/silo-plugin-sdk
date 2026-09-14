package runtimehost

import (
	"context"
	"errors"
	"fmt"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

// Instance state limits enforced by the host. The client checks them before
// the round trip so a plugin gets a clear error instead of a gRPC status.
const (
	MaxInstanceStateKeyBytes   = 256
	MaxInstanceStateValueBytes = 256 << 10
	MaxInstanceStateKeys       = 256
)

// ErrStateNotExist is returned by ReadState when no value is stored under the
// key. It plays the role of tailscale's ipn.ErrStateNotExist: callers compare
// with errors.Is.
var ErrStateNotExist = errors.New("runtimehost: instance state key does not exist")

// DefaultInstanceStateTimeout bounds each InstanceStateStore round trip when
// the caller does not supply a context (ipn.StateStore has none).
const DefaultInstanceStateTimeout = 30 * time.Second

// ReadInstanceState reads one key of the calling plugin instance's private
// state. It returns (nil, false, nil) when the key is absent.
func (c *Client) ReadInstanceState(ctx context.Context, key string) ([]byte, bool, error) {
	if err := validateInstanceStateKey(key); err != nil {
		return nil, false, err
	}
	resp, err := c.rpc.ReadInstanceState(ctx, &pluginv1.ReadInstanceStateRequest{Key: key})
	if err != nil {
		return nil, false, err
	}
	if !resp.GetFound() {
		return nil, false, nil
	}
	return append([]byte(nil), resp.GetValue()...), true, nil
}

// WriteInstanceState stores value under key in the calling plugin instance's
// private state. The host encrypts the value and never returns it through any
// API.
func (c *Client) WriteInstanceState(ctx context.Context, key string, value []byte) error {
	if err := validateInstanceStateKey(key); err != nil {
		return err
	}
	if len(value) > MaxInstanceStateValueBytes {
		return fmt.Errorf("runtimehost: instance state value for %q is %d bytes, limit %d", key, len(value), MaxInstanceStateValueBytes)
	}
	_, err := c.rpc.WriteInstanceState(ctx, &pluginv1.WriteInstanceStateRequest{Key: key, Value: value})
	return err
}

func validateInstanceStateKey(key string) error {
	if key == "" {
		return fmt.Errorf("runtimehost: instance state key is required")
	}
	if len(key) > MaxInstanceStateKeyBytes {
		return fmt.Errorf("runtimehost: instance state key is %d bytes, limit %d", len(key), MaxInstanceStateKeyBytes)
	}
	return nil
}

// StateStore is the two-method shape of tailscale's ipn.StateStore, declared
// here so the SDK does not depend on tailscale. ReadState returns
// ErrStateNotExist for an absent key.
//
// ipn.StateStore takes ipn.StateKey (a named string type) rather than string,
// so Go will not accept an InstanceStateStore as an ipn.StateStore directly.
// A tsnet plugin wraps it with a few lines; see docs/network-access-provider.md.
type StateStore interface {
	ReadState(key string) ([]byte, error)
	WriteState(key string, value []byte) error
}

// InstanceStateStore implements StateStore over the ReadInstanceState and
// WriteInstanceState RPCs. Every value lives in the host's encrypted
// per-instance scope (installation plus host), so each proxy node keeps its
// own node key without the plugin managing files.
type InstanceStateStore struct {
	client  *Client
	timeout time.Duration
}

// InstanceStateStore returns a store that scopes every round trip with
// DefaultInstanceStateTimeout.
func (c *Client) InstanceStateStore() *InstanceStateStore {
	return &InstanceStateStore{client: c, timeout: DefaultInstanceStateTimeout}
}

// WithTimeout returns a copy of the store using the given per-call timeout.
// A non-positive timeout disables the deadline.
func (s *InstanceStateStore) WithTimeout(timeout time.Duration) *InstanceStateStore {
	return &InstanceStateStore{client: s.client, timeout: timeout}
}

// ReadState returns the value stored under key or ErrStateNotExist.
func (s *InstanceStateStore) ReadState(key string) ([]byte, error) {
	ctx, cancel := s.context()
	defer cancel()
	value, found, err := s.client.ReadInstanceState(ctx, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrStateNotExist
	}
	return value, nil
}

// WriteState stores value under key.
func (s *InstanceStateStore) WriteState(key string, value []byte) error {
	ctx, cancel := s.context()
	defer cancel()
	return s.client.WriteInstanceState(ctx, key, value)
}

var _ StateStore = (*InstanceStateStore)(nil)

func (s *InstanceStateStore) context() (context.Context, context.CancelFunc) {
	if s.timeout <= 0 {
		return context.WithCancel(context.Background())
	}
	return context.WithTimeout(context.Background(), s.timeout)
}
