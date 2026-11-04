package runtimehost

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

// ErrInstanceStateConflict means another write changed the key after the
// version the caller expected. Read the key again and redo the change.
var ErrInstanceStateConflict = errors.New("runtimehost: instance state key changed since it was read")

// ErrInstanceStateUnversioned means the host doesn't keep versions, so it
// can't make a write conditional.
var ErrInstanceStateUnversioned = errors.New("runtimehost: host does not keep instance state versions")

// maxUpdateAttempts bounds UpdateInstanceState's retries on conflict.
const maxUpdateAttempts = 8

// VersionedState is one key's value and the version it was read at. Version
// is 0 when Found is false.
type VersionedState struct {
	Value   []byte
	Found   bool
	Version int64
}

// ReadInstanceStateVersion reads a key with its version. It returns
// ErrInstanceStateUnversioned on a host that doesn't keep versions.
func (c *Client) ReadInstanceStateVersion(ctx context.Context, key string) (VersionedState, error) {
	if err := validateInstanceStateKey(key); err != nil {
		return VersionedState{}, err
	}
	resp, err := c.rpc.ReadInstanceState(ctx, &pluginv1.ReadInstanceStateRequest{Key: key})
	if err != nil {
		return VersionedState{}, err
	}
	if !resp.GetVersioned() {
		return VersionedState{}, ErrInstanceStateUnversioned
	}
	if !resp.GetFound() {
		return VersionedState{}, nil
	}
	return VersionedState{Value: append([]byte(nil), resp.GetValue()...), Found: true, Version: resp.GetVersion()}, nil
}

// WriteInstanceStateIf writes value only if the key is still at
// expectedVersion (0: the key must not exist), and returns the new version.
// It returns ErrInstanceStateConflict when another write got there first.
//
// Pass a version from ReadInstanceStateVersion, which fails on hosts that
// don't keep versions: such a host ignores the condition, writes anyway, and
// this method then returns ErrInstanceStateUnversioned.
func (c *Client) WriteInstanceStateIf(ctx context.Context, key string, value []byte, expectedVersion int64) (int64, error) {
	if err := validateInstanceStateKey(key); err != nil {
		return 0, err
	}
	if len(value) > MaxInstanceStateValueBytes {
		return 0, fmt.Errorf("runtimehost: instance state value for %q is %d bytes, limit %d", key, len(value), MaxInstanceStateValueBytes)
	}
	resp, err := c.rpc.WriteInstanceState(ctx, &pluginv1.WriteInstanceStateRequest{Key: key, Value: value, ExpectedVersion: &expectedVersion})
	if status.Code(err) == codes.Aborted {
		return 0, ErrInstanceStateConflict
	}
	if err != nil {
		return 0, err
	}
	if resp.GetVersion() == 0 {
		return 0, ErrInstanceStateUnversioned
	}
	return resp.GetVersion(), nil
}

// UpdateInstanceState reads key, passes its value to update, and writes what
// update returns only if no other write changed the key in between. On a
// conflict it reads again and calls update again, a few times at most, so
// update must be safe to repeat and should not have side effects. An error
// from update stops the loop and is returned as is.
//
// Use it when several API servers can change the same key, such as one
// record shared by every process of the plugin.
func (c *Client) UpdateInstanceState(ctx context.Context, key string, update func(current []byte, found bool) ([]byte, error)) error {
	for range maxUpdateAttempts {
		current, err := c.ReadInstanceStateVersion(ctx, key)
		if err != nil {
			return err
		}
		next, err := update(current.Value, current.Found)
		if err != nil {
			return err
		}
		_, err = c.WriteInstanceStateIf(ctx, key, next, current.Version)
		if !errors.Is(err, ErrInstanceStateConflict) {
			return err
		}
	}
	return fmt.Errorf("%w: gave up on %q after %d attempts", ErrInstanceStateConflict, key, maxUpdateAttempts)
}
