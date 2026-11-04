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
	c.recordStateVersions(resp.GetVersioned())
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
// It returns ErrInstanceStateConflict when another write got there first,
// and ErrInstanceStateUnversioned, without writing, on a host that doesn't
// keep versions: such a host would ignore the condition. The first call on
// a client reads the key once to find out, unless a read already did.
func (c *Client) WriteInstanceStateIf(ctx context.Context, key string, value []byte, expectedVersion int64) (int64, error) {
	if err := validateInstanceStateKey(key); err != nil {
		return 0, err
	}
	if len(value) > MaxInstanceStateValueBytes {
		return 0, fmt.Errorf("runtimehost: instance state value for %q is %d bytes, limit %d", key, len(value), MaxInstanceStateValueBytes)
	}
	versioned, err := c.hostKeepsStateVersions(ctx, key)
	if err != nil {
		return 0, err
	}
	if !versioned {
		return 0, ErrInstanceStateUnversioned
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

// hostKeepsStateVersions reports whether the host honors expected_version,
// reading key to find out the first time.
func (c *Client) hostKeepsStateVersions(ctx context.Context, key string) (bool, error) {
	switch c.stateVersions.Load() {
	case 1:
		return true, nil
	case 2:
		return false, nil
	}
	resp, err := c.rpc.ReadInstanceState(ctx, &pluginv1.ReadInstanceStateRequest{Key: key})
	if err != nil {
		return false, err
	}
	c.recordStateVersions(resp.GetVersioned())
	return resp.GetVersioned(), nil
}

func (c *Client) recordStateVersions(versioned bool) {
	if versioned {
		c.stateVersions.Store(1)
	} else {
		c.stateVersions.Store(2)
	}
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
