package runtimehost

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

// ErrHostAPINotGranted means the host has no API key for this installation:
// the manifest declares no host_api_scopes, or an administrator hasn't
// approved them yet. Ask the administrator to review the plugin's
// permissions; don't retry in a loop.
var ErrHostAPINotGranted = errors.New("runtimehost: no approved host API scopes for this installation")

// ErrHostAPIUnsupported means the host predates GetHostAPICredential.
var ErrHostAPIUnsupported = errors.New("runtimehost: host does not issue host API credentials")

// HostAPICredential is a bearer token for the host's HTTP API, limited to
// the installation's approved host_api_scopes. Keep it in memory only, never
// log or persist it, and fetch it again after the host answers 401: an
// administrator can revoke or reissue it.
type HostAPICredential struct {
	Token   string
	Scopes  []string
	BaseURL string
}

// HostAPICredential returns the API key the host issued for this
// installation. It returns ErrHostAPINotGranted or ErrHostAPIUnsupported
// when there is none.
func (c *Client) HostAPICredential(ctx context.Context) (*HostAPICredential, error) {
	resp, err := c.rpc.GetHostAPICredential(ctx, &pluginv1.GetHostAPICredentialRequest{})
	switch status.Code(err) {
	case codes.OK:
	case codes.FailedPrecondition:
		return nil, fmt.Errorf("%w: %s", ErrHostAPINotGranted, status.Convert(err).Message())
	case codes.Unimplemented:
		return nil, ErrHostAPIUnsupported
	default:
		return nil, err
	}
	if resp.GetToken() == "" {
		return nil, ErrHostAPINotGranted
	}
	return &HostAPICredential{
		Token:   resp.GetToken(),
		Scopes:  append([]string(nil), resp.GetScopes()...),
		BaseURL: resp.GetBaseUrl(),
	}, nil
}
