package runtime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/structpb"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/capability"
)

// Compile-time: manifestRuntime satisfies the Runtime server contract.
var _ pluginv1.RuntimeServer = (*manifestRuntime)(nil)

func TestManifestRuntimeServesManifestAndConfigure(t *testing.T) {
	m := &pluginv1.PluginManifest{PluginId: "silo.test", Version: "1.0.0"}
	rt := &manifestRuntime{manifest: m}

	resp, err := rt.GetManifest(context.Background(), &pluginv1.GetManifestRequest{})
	if err != nil || resp.GetManifest().GetPluginId() != "silo.test" {
		t.Fatalf("GetManifest: resp=%v err=%v", resp, err)
	}
	if _, err := rt.Configure(context.Background(), &pluginv1.ConfigureRequest{}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	bresp, err := rt.BindHostBroker(context.Background(), &pluginv1.BindHostBrokerRequest{BrokerId: 7})
	if err != nil || bresp == nil {
		t.Fatalf("BindHostBroker: resp=%v err=%v", bresp, err)
	}
}

// serveConfigClient serves cfg's plugin set over an in-memory listener, the
// way ServeManifestWithOptions would, and returns the host-side client.
func serveConfigClient(t *testing.T, cfg ServeConfig) *Client {
	t.Helper()
	p, ok := cfg.Plugins[PluginSetName].(plugin.GRPCPlugin)
	if !ok {
		t.Fatalf("plugin set entry %T is not a GRPCPlugin", cfg.Plugins[PluginSetName])
	}
	listener := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	if err := p.GRPCServer(nil, srv); err != nil {
		t.Fatalf("GRPCServer = %v", err)
	}
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return NewClient(conn)
}

// errUndecodable is what a plugin following ConfigureFunc's rule returns: the
// input could not be stored at all.
var errUndecodable = errors.New("cannot decode settings")

// oidcSettings stands in for a plugin's settings type. applyEntries follows
// the ConfigureFunc rule: missing keys are stored as empty, and only a value
// of the wrong type fails.
type oidcSettings struct {
	issuer string
}

func (s *oidcSettings) applyEntries(_ context.Context, entries []*pluginv1.ConfigEntry) error {
	for _, entry := range entries {
		if entry.GetKey() != "connection" {
			continue
		}
		issuer, ok := entry.GetValue().GetFields()["issuer"]
		if !ok {
			continue
		}
		value, isString := issuer.GetKind().(*structpb.Value_StringValue)
		if !isString {
			return fmt.Errorf("connection.issuer: %w", errUndecodable)
		}
		s.issuer = value.StringValue
	}
	return nil
}

func TestManifestServeConfigDeliversConfigure(t *testing.T) {
	settings := &oidcSettings{}
	cfg, err := manifestServeConfig(&pluginv1.PluginManifest{PluginId: "silo.test"},
		CapabilityServers{}, WithConfigure(settings.applyEntries))
	if err != nil {
		t.Fatalf("manifestServeConfig = %v", err)
	}
	client := serveConfigClient(t, cfg)
	ctx := context.Background()

	// A fresh install sends no entries; Configure must still succeed.
	if _, err := client.Runtime().Configure(ctx, &pluginv1.ConfigureRequest{}); err != nil {
		t.Fatalf("Configure with no entries = %v, want nil", err)
	}

	value, err := structpb.NewStruct(map[string]any{"issuer": "https://id.example"})
	if err != nil {
		t.Fatal(err)
	}
	req := &pluginv1.ConfigureRequest{Config: []*pluginv1.ConfigEntry{{Key: "connection", Value: value}}}
	if _, err := client.Runtime().Configure(ctx, req); err != nil {
		t.Fatalf("Configure = %v", err)
	}
	if settings.issuer != "https://id.example" {
		t.Fatalf("callback stored issuer %q, want the configured one", settings.issuer)
	}
}

func TestManifestServeConfigConfigureErrorFailsConfigure(t *testing.T) {
	settings := &oidcSettings{}
	cfg, err := manifestServeConfig(&pluginv1.PluginManifest{PluginId: "silo.test"},
		CapabilityServers{}, WithConfigure(settings.applyEntries))
	if err != nil {
		t.Fatalf("manifestServeConfig = %v", err)
	}
	value, err := structpb.NewStruct(map[string]any{"issuer": 42})
	if err != nil {
		t.Fatal(err)
	}
	req := &pluginv1.ConfigureRequest{Config: []*pluginv1.ConfigEntry{{Key: "connection", Value: value}}}
	_, err = cfg.Servers.Runtime.Configure(context.Background(), req)
	if !errors.Is(err, errUndecodable) {
		t.Fatalf("Configure error = %v, want %v", err, errUndecodable)
	}
}

func TestManifestServeConfigWithoutConfigureIsNoOp(t *testing.T) {
	cfg, err := manifestServeConfig(&pluginv1.PluginManifest{PluginId: "silo.test"}, CapabilityServers{})
	if err != nil {
		t.Fatalf("manifestServeConfig = %v", err)
	}
	if _, err := cfg.Servers.Runtime.Configure(context.Background(), &pluginv1.ConfigureRequest{}); err != nil {
		t.Fatalf("Configure = %v, want nil", err)
	}
}

type onlyAuthProvider struct {
	pluginv1.UnimplementedAuthProviderServer
}

// checksServer answers EndSessionUrl with its name, so tests can tell which
// registered server handled the call.
type checksServer struct {
	pluginv1.UnimplementedAuthProviderChecksServer
	name string
}

func (s checksServer) EndSessionUrl(context.Context, *pluginv1.AuthEndSessionUrlRequest) (*pluginv1.AuthEndSessionUrlResponse, error) {
	return &pluginv1.AuthEndSessionUrlResponse{Url: "https://id.example/logout?by=" + s.name}, nil
}

// providerWithChecks implements both services on one type.
type providerWithChecks struct {
	pluginv1.UnimplementedAuthProviderServer
	checksServer
}

func authManifest(connectionTest bool) *pluginv1.PluginManifest {
	metadata, err := structpb.NewStruct(map[string]any{"connection_test": connectionTest})
	if err != nil {
		panic(err)
	}
	return &pluginv1.PluginManifest{
		PluginId: "silo.test",
		Capabilities: []*pluginv1.CapabilityDescriptor{{
			Type:      capability.AuthProvider,
			Id:        "main",
			AuthModes: []string{"oauth2"},
			Metadata:  metadata,
		}},
	}
}

func TestManifestServeConfigRejectsConnectionTestWithoutChecks(t *testing.T) {
	_, err := manifestServeConfig(authManifest(true), CapabilityServers{AuthProvider: onlyAuthProvider{}})
	if err == nil || !strings.Contains(err.Error(), "WithAuthProviderChecks") {
		t.Fatalf("manifestServeConfig = %v, want an error naming WithAuthProviderChecks", err)
	}
}

func TestManifestServeConfigAllowsMissingChecksWithoutConnectionTest(t *testing.T) {
	if _, err := manifestServeConfig(authManifest(false), CapabilityServers{AuthProvider: onlyAuthProvider{}}); err != nil {
		t.Fatalf("manifestServeConfig = %v, want nil", err)
	}
}

func TestManifestServeConfigRegistersExplicitChecks(t *testing.T) {
	cfg, err := manifestServeConfig(authManifest(true),
		CapabilityServers{AuthProvider: onlyAuthProvider{}},
		WithAuthProviderChecks(checksServer{name: "explicit"}))
	if err != nil {
		t.Fatalf("manifestServeConfig = %v", err)
	}
	resp, err := serveConfigClient(t, cfg).AuthProviderChecks().EndSessionUrl(context.Background(), &pluginv1.AuthEndSessionUrlRequest{})
	if err != nil {
		t.Fatalf("EndSessionUrl = %v", err)
	}
	if !strings.HasSuffix(resp.GetUrl(), "by=explicit") {
		t.Fatalf("EndSessionUrl url = %q, want the explicit server's", resp.GetUrl())
	}
}

// The explicit option wins over an AuthProvider that also implements the
// checks, and the service is registered once (a second registration panics).
func TestManifestServeConfigExplicitChecksWinOverAuthProvider(t *testing.T) {
	cfg, err := manifestServeConfig(authManifest(true),
		CapabilityServers{AuthProvider: providerWithChecks{checksServer: checksServer{name: "provider"}}},
		WithAuthProviderChecks(checksServer{name: "explicit"}))
	if err != nil {
		t.Fatalf("manifestServeConfig = %v", err)
	}
	resp, err := serveConfigClient(t, cfg).AuthProviderChecks().EndSessionUrl(context.Background(), &pluginv1.AuthEndSessionUrlRequest{})
	if err != nil {
		t.Fatalf("EndSessionUrl = %v", err)
	}
	if !strings.HasSuffix(resp.GetUrl(), "by=explicit") {
		t.Fatalf("EndSessionUrl url = %q, want the explicit server's", resp.GetUrl())
	}
}

func TestManifestServeConfigFallsBackToAuthProviderChecks(t *testing.T) {
	cfg, err := manifestServeConfig(authManifest(true),
		CapabilityServers{AuthProvider: providerWithChecks{checksServer: checksServer{name: "provider"}}})
	if err != nil {
		t.Fatalf("manifestServeConfig = %v", err)
	}
	resp, err := serveConfigClient(t, cfg).AuthProviderChecks().EndSessionUrl(context.Background(), &pluginv1.AuthEndSessionUrlRequest{})
	if err != nil {
		t.Fatalf("EndSessionUrl = %v", err)
	}
	if !strings.HasSuffix(resp.GetUrl(), "by=provider") {
		t.Fatalf("EndSessionUrl url = %q, want the AuthProvider server's", resp.GetUrl())
	}
}

// peerServer answers AuthenticatePeer with its name as the subject, so tests
// can tell which registered server handled the call.
type peerServer struct {
	pluginv1.UnimplementedNetworkIdentityAuthServer
	name string
}

func (s peerServer) AuthenticatePeer(_ context.Context, req *pluginv1.AuthenticatePeerRequest) (*pluginv1.AuthenticateResponse, error) {
	return &pluginv1.AuthenticateResponse{ExternalSubject: s.name + "|" + req.GetPeerAddress()}, nil
}

// providerWithPeer implements AuthProvider and NetworkIdentityAuth on one type.
type providerWithPeer struct {
	pluginv1.UnimplementedAuthProviderServer
	peerServer
}

func networkAuthManifest() *pluginv1.PluginManifest {
	return &pluginv1.PluginManifest{
		PluginId: "silo.test",
		Capabilities: []*pluginv1.CapabilityDescriptor{
			{
				Type: capability.NetworkAccessProvider,
				Id:   "overlay",
				NetworkAccessProvider: &pluginv1.NetworkAccessProviderDescriptor{
					Provider: "tailscale", DisplayName: "Tailscale",
				},
			},
			{Type: capability.AuthProvider, Id: "tailscale", AuthModes: []string{"network"}},
		},
	}
}

func TestManifestServeConfigRejectsNetworkModeWithoutService(t *testing.T) {
	_, err := manifestServeConfig(networkAuthManifest(), CapabilityServers{AuthProvider: onlyAuthProvider{}})
	if err == nil || !strings.Contains(err.Error(), "WithNetworkIdentityAuth") {
		t.Fatalf("manifestServeConfig = %v, want an error naming WithNetworkIdentityAuth", err)
	}
}

func TestManifestServeConfigRegistersExplicitNetworkIdentityAuth(t *testing.T) {
	cfg, err := manifestServeConfig(networkAuthManifest(),
		CapabilityServers{AuthProvider: providerWithPeer{peerServer: peerServer{name: "provider"}}},
		WithNetworkIdentityAuth(peerServer{name: "explicit"}))
	if err != nil {
		t.Fatalf("manifestServeConfig = %v", err)
	}
	resp, err := serveConfigClient(t, cfg).NetworkIdentityAuth().AuthenticatePeer(context.Background(),
		&pluginv1.AuthenticatePeerRequest{PeerAddress: "100.64.0.7"})
	if err != nil {
		t.Fatalf("AuthenticatePeer = %v", err)
	}
	if got, want := resp.GetExternalSubject(), "explicit|100.64.0.7"; got != want {
		t.Fatalf("AuthenticatePeer subject = %q, want %q", got, want)
	}
}

func TestManifestServeConfigFallsBackToAuthProviderNetworkIdentity(t *testing.T) {
	cfg, err := manifestServeConfig(networkAuthManifest(),
		CapabilityServers{AuthProvider: providerWithPeer{peerServer: peerServer{name: "provider"}}})
	if err != nil {
		t.Fatalf("manifestServeConfig = %v", err)
	}
	resp, err := serveConfigClient(t, cfg).NetworkIdentityAuth().AuthenticatePeer(context.Background(),
		&pluginv1.AuthenticatePeerRequest{PeerAddress: "100.64.0.7"})
	if err != nil {
		t.Fatalf("AuthenticatePeer = %v", err)
	}
	if got, want := resp.GetExternalSubject(), "provider|100.64.0.7"; got != want {
		t.Fatalf("AuthenticatePeer subject = %q, want %q", got, want)
	}
}
