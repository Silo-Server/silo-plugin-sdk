package runtimehost

import (
	"context"
	"fmt"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

// NetworkAccessStatus state values. The vocabulary is open; hosts tolerate
// values they do not recognize.
const (
	NetworkAccessStateDisconnected          = "disconnected"
	NetworkAccessStateAwaitingAuthorization = "awaiting_authorization"
	NetworkAccessStateConnecting            = "connecting"
	NetworkAccessStateConnected             = "connected"
	NetworkAccessStateError                 = "error"
)

// ReportNetworkAccessStatus pushes a network access status change to the
// host. Call it on every state transition so the host does not have to poll.
// The host logs state transitions only; auth_url is never logged.
func (c *Client) ReportNetworkAccessStatus(ctx context.Context, status *pluginv1.NetworkAccessStatus) error {
	if status == nil {
		return fmt.Errorf("runtimehost: network access status is required")
	}
	if status.GetState() == "" {
		return fmt.Errorf("runtimehost: network access status state is required")
	}
	_, err := c.rpc.ReportNetworkAccessStatus(ctx, &pluginv1.ReportNetworkAccessStatusRequest{Status: status})
	return err
}
