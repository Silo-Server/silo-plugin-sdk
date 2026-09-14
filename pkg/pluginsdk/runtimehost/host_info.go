package runtimehost

import (
	"context"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

// Host roles reported in HostInfo.HostRole.
const (
	HostRoleAPI   = "api"
	HostRoleProxy = "proxy"
)

// Listener names reported in HostListener.Name.
const (
	ListenerAPI      = "api"
	ListenerJellyfin = "jellyfin"
	ListenerABS      = "abs"
)

// HostInfo is the typed view of RuntimeHost.GetHostInfo.
type HostInfo struct {
	PublicBaseURL      string
	InternalBaseURL    string
	PluginProxyBaseURL string

	// HostRole is "api" or "proxy". Empty on hosts that predate v0.16.0.
	HostRole string
	// HostName is the node name on proxies or the server name on the api host.
	HostName string
	// NodeID is stream_nodes.id on proxies and 0 on the api host.
	NodeID int64
	// IngressToken is the per-process-start secret a network access provider
	// stamps on proxied requests as X-Silo-Ingress-Token. Never log or persist
	// it; call GetHostInfo again after a restart.
	IngressToken string
	// Listeners are the local listeners a network access provider should
	// expose. Always contains "api" on hosts that implement v0.16.0.
	Listeners []HostListener
}

// HostListener is one local listener the host asks a network access provider
// to expose on the overlay network.
type HostListener struct {
	// Name is "api", "jellyfin", or "abs".
	Name string
	// Address is the loopback dial address, host:port.
	Address string
	// DefaultPort is the port the plugin should expose the listener on; zero
	// means provider default.
	DefaultPort int
}

// Listener returns the named listener, or nil when the host did not report it.
func (h *HostInfo) Listener(name string) *HostListener {
	if h == nil {
		return nil
	}
	for i := range h.Listeners {
		if h.Listeners[i].Name == name {
			return &h.Listeners[i]
		}
	}
	return nil
}

func (c *Client) GetHostInfo(ctx context.Context) (*HostInfo, error) {
	resp, err := c.rpc.GetHostInfo(ctx, &pluginv1.GetHostInfoRequest{})
	if err != nil {
		return nil, err
	}
	info := &HostInfo{
		PublicBaseURL:      resp.GetPublicBaseUrl(),
		InternalBaseURL:    resp.GetInternalBaseUrl(),
		PluginProxyBaseURL: resp.GetPluginProxyBaseUrl(),
		HostRole:           resp.GetHostRole(),
		HostName:           resp.GetHostName(),
		NodeID:             resp.GetNodeId(),
		IngressToken:       resp.GetIngressToken(),
	}
	if listeners := resp.GetListeners(); len(listeners) > 0 {
		info.Listeners = make([]HostListener, 0, len(listeners))
		for _, l := range listeners {
			info.Listeners = append(info.Listeners, HostListener{
				Name:        l.GetName(),
				Address:     l.GetAddress(),
				DefaultPort: int(l.GetDefaultPort()),
			})
		}
	}
	return info, nil
}
