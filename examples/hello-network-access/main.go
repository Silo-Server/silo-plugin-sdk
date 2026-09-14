// Command hello-network-access is a stub network_access_provider.v1 plugin.
//
// It has no overlay network: Connect flips an in-memory state to "connected",
// reports a fake origin built from the host's api listener, persists
// desired_connected in host-provided instance state, and Disconnect reverses
// it. It exists so silo-server tests can exercise the resident plugin
// supervisor, the instance state RPCs, and the status push without a tsnet
// dependency. A real provider follows docs/network-access-provider.md.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"sync"

	"github.com/hashicorp/go-hclog"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	sdkruntime "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtimehost"
)

//go:embed manifest.json
var manifestJSON []byte

const (
	version = "0.1.0"
	// desiredKey is the instance state key holding plugin-owned intent.
	desiredKey = "desired_connected"
)

type provider struct {
	pluginv1.UnimplementedNetworkAccessProviderServer

	logger hclog.Logger

	mu        sync.Mutex
	connected bool
	desired   bool
	restored  bool
	hostInfo  *runtimehost.HostInfo
}

func (p *provider) Connect(ctx context.Context, _ *pluginv1.NetworkAccessConnectRequest) (*pluginv1.NetworkAccessStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreLocked(ctx)
	host := sdkruntime.Host()
	if host != nil {
		info, err := host.GetHostInfo(ctx)
		if err != nil {
			return nil, fmt.Errorf("get host info: %w", err)
		}
		p.hostInfo = info
	}
	p.connected = true
	p.desired = true
	p.persistLocked(ctx, host)
	status := p.statusLocked()
	p.reportLocked(ctx, host, status)
	return status, nil
}

func (p *provider) Disconnect(ctx context.Context, _ *pluginv1.NetworkAccessDisconnectRequest) (*pluginv1.NetworkAccessStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreLocked(ctx)
	host := sdkruntime.Host()
	p.connected = false
	p.desired = false
	p.persistLocked(ctx, host)
	status := p.statusLocked()
	p.reportLocked(ctx, host, status)
	return status, nil
}

func (p *provider) GetStatus(ctx context.Context, _ *pluginv1.NetworkAccessGetStatusRequest) (*pluginv1.NetworkAccessStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restoreLocked(ctx)
	return p.statusLocked(), nil
}

// restoreLocked loads desired_connected from instance state once per process
// and, when it was true, resumes the "connection" so the intent survives
// plugin restarts. A real provider would bring the overlay up here.
func (p *provider) restoreLocked(ctx context.Context) {
	if p.restored {
		return
	}
	host := sdkruntime.Host()
	if host == nil {
		return
	}
	p.restored = true
	value, found, err := host.ReadInstanceState(ctx, desiredKey)
	if err != nil {
		p.logger.Warn("read instance state", "err", err)
		return
	}
	if found && string(value) == "1" {
		p.desired = true
		p.connected = true
		if info, err := host.GetHostInfo(ctx); err == nil {
			p.hostInfo = info
		}
	}
}

func (p *provider) persistLocked(ctx context.Context, host *runtimehost.Client) {
	if host == nil {
		return
	}
	value := []byte("0")
	if p.desired {
		value = []byte("1")
	}
	if err := host.WriteInstanceState(ctx, desiredKey, value); err != nil {
		p.logger.Warn("write instance state", "err", err)
	}
}

func (p *provider) reportLocked(ctx context.Context, host *runtimehost.Client, status *pluginv1.NetworkAccessStatus) {
	if host == nil {
		return
	}
	if err := host.ReportNetworkAccessStatus(ctx, status); err != nil {
		p.logger.Warn("report network access status", "err", err)
	}
}

func (p *provider) statusLocked() *pluginv1.NetworkAccessStatus {
	status := &pluginv1.NetworkAccessStatus{
		State:            runtimehost.NetworkAccessStateDisconnected,
		ProviderVersion:  "stub " + version,
		DesiredConnected: p.desired,
	}
	if !p.connected {
		return status
	}
	status.State = runtimehost.NetworkAccessStateConnected
	status.Hostname = "stub.invalid"
	status.Addresses = []string{"100.64.0.1"}
	if p.hostInfo != nil {
		for _, l := range p.hostInfo.Listeners {
			origin := fmt.Sprintf("https://%s:%d", status.Hostname, l.DefaultPort)
			status.Listeners = append(status.Listeners, &pluginv1.NetworkAccessListener{Name: l.Name, Origin: origin})
			if l.Name == runtimehost.ListenerAPI {
				status.Origin = origin
			}
		}
	}
	if status.Origin == "" {
		status.Origin = "https://" + status.Hostname
	}
	return status
}

func main() {
	logger := hclog.New(&hclog.LoggerOptions{Name: "hello-network-access", Output: os.Stderr})
	sdkruntime.ServeManifest(manifestJSON, version, sdkruntime.CapabilityServers{
		NetworkAccessProvider: &provider{logger: logger},
	})
}
