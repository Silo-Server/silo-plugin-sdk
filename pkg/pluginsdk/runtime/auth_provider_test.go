package runtime_test

import (
	"context"
	"net"
	"testing"

	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	runtime "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
)

// legacyAuthProvider is an auth server written before v0.22.0: it implements
// only the AuthProvider service.
type legacyAuthProvider struct {
	pluginv1.UnimplementedAuthProviderServer
}

// checkingAuthProvider implements AuthProvider and AuthProviderChecks on one
// type, which is how the SDK expects plugins to add the new RPCs.
type checkingAuthProvider struct {
	pluginv1.UnimplementedAuthProviderServer
	gotConfig []*pluginv1.ConfigEntry
}

func (p *checkingAuthProvider) TestConnection(_ context.Context, req *pluginv1.AuthTestConnectionRequest) (*pluginv1.AuthTestConnectionResponse, error) {
	p.gotConfig = req.GetConfig()
	return &pluginv1.AuthTestConnectionResponse{
		Ok:    true,
		Steps: []*pluginv1.AuthTestStep{{Id: "discovery", Label: "Discovery", Ok: true}},
	}, nil
}

func (p *checkingAuthProvider) CheckAccount(_ context.Context, req *pluginv1.CheckAccountRequest) (*pluginv1.CheckAccountResponse, error) {
	return &pluginv1.CheckAccountResponse{
		Status:  pluginv1.CheckAccountStatus_CHECK_ACCOUNT_STATUS_ACTIVE,
		Account: &pluginv1.AuthenticateResponse{ExternalSubject: req.GetExternalSubject()},
	}, nil
}

func (p *checkingAuthProvider) EndSessionUrl(_ context.Context, req *pluginv1.AuthEndSessionUrlRequest) (*pluginv1.AuthEndSessionUrlResponse, error) {
	return &pluginv1.AuthEndSessionUrlResponse{Url: "https://id.example/logout?post_logout_redirect_uri=" + req.GetPostLogoutRedirectUri()}, nil
}

var _ pluginv1.AuthProviderChecksServer = (*checkingAuthProvider)(nil)

func registeredServices(t *testing.T, servers runtime.CapabilityServers) map[string]grpc.ServiceInfo {
	t.Helper()
	p := runtime.DefaultPluginSet(servers)[runtime.PluginSetName].(plugin.GRPCPlugin)
	srv := grpc.NewServer()
	if err := p.GRPCServer(nil, srv); err != nil {
		t.Fatalf("GRPCServer = %v, want nil", err)
	}
	return srv.GetServiceInfo()
}

func TestGRPCServerRegistersAuthProviderChecksWhenImplemented(t *testing.T) {
	services := registeredServices(t, runtime.CapabilityServers{
		Runtime:      stubRuntime{},
		AuthProvider: &checkingAuthProvider{},
	})
	for _, name := range []string{"silo.plugin.v1.AuthProvider", "silo.plugin.v1.AuthProviderChecks"} {
		if _, ok := services[name]; !ok {
			t.Fatalf("%s not registered; got %v", name, services)
		}
	}
}

func TestGRPCServerSkipsAuthProviderChecksForLegacyServer(t *testing.T) {
	services := registeredServices(t, runtime.CapabilityServers{
		Runtime:      stubRuntime{},
		AuthProvider: legacyAuthProvider{},
	})
	if _, ok := services["silo.plugin.v1.AuthProvider"]; !ok {
		t.Fatalf("AuthProvider not registered; got %v", services)
	}
	if _, ok := services["silo.plugin.v1.AuthProviderChecks"]; ok {
		t.Fatal("AuthProviderChecks registered for a server that does not implement it")
	}
}

// dialPlugin serves the capability servers over an in-memory listener and
// returns the host-side client.
func dialPlugin(t *testing.T, servers runtime.CapabilityServers) *runtime.Client {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	p := runtime.DefaultPluginSet(servers)[runtime.PluginSetName].(plugin.GRPCPlugin)
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
	return runtime.NewClient(conn)
}

func TestAuthProviderChecksRoundTrip(t *testing.T) {
	provider := &checkingAuthProvider{}
	client := dialPlugin(t, runtime.CapabilityServers{Runtime: stubRuntime{}, AuthProvider: provider})
	ctx := context.Background()

	staged := []*pluginv1.ConfigEntry{{Key: "connection"}}
	test, err := client.AuthProviderChecks().TestConnection(ctx, &pluginv1.AuthTestConnectionRequest{Config: staged})
	if err != nil {
		t.Fatalf("TestConnection = %v", err)
	}
	if !test.GetOk() || len(test.GetSteps()) != 1 || test.GetSteps()[0].GetId() != "discovery" {
		t.Fatalf("TestConnection response = %v", test)
	}
	if len(provider.gotConfig) != 1 || provider.gotConfig[0].GetKey() != "connection" {
		t.Fatalf("plugin received config %v, want the staged entry", provider.gotConfig)
	}

	check, err := client.AuthProviderChecks().CheckAccount(ctx, &pluginv1.CheckAccountRequest{ExternalSubject: "sub-1"})
	if err != nil {
		t.Fatalf("CheckAccount = %v", err)
	}
	if check.GetStatus() != pluginv1.CheckAccountStatus_CHECK_ACCOUNT_STATUS_ACTIVE || check.GetAccount().GetExternalSubject() != "sub-1" {
		t.Fatalf("CheckAccount response = %v", check)
	}

	logout, err := client.AuthProviderChecks().EndSessionUrl(ctx, &pluginv1.AuthEndSessionUrlRequest{
		ExternalSubject:       "sub-1",
		PostLogoutRedirectUri: "https://silo.example/login",
	})
	if err != nil {
		t.Fatalf("EndSessionUrl = %v", err)
	}
	if logout.GetUrl() != "https://id.example/logout?post_logout_redirect_uri=https://silo.example/login" {
		t.Fatalf("EndSessionUrl url = %q", logout.GetUrl())
	}
}

// A host calling the new RPCs on a plugin built before v0.22.0 must see
// Unimplemented so it can fall back.
func TestAuthProviderChecksUnimplementedForLegacyPlugin(t *testing.T) {
	client := dialPlugin(t, runtime.CapabilityServers{Runtime: stubRuntime{}, AuthProvider: legacyAuthProvider{}})
	ctx := context.Background()

	if _, err := client.AuthProviderChecks().TestConnection(ctx, &pluginv1.AuthTestConnectionRequest{}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("TestConnection code = %v, want Unimplemented (err %v)", status.Code(err), err)
	}
	if _, err := client.AuthProviderChecks().CheckAccount(ctx, &pluginv1.CheckAccountRequest{}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("CheckAccount code = %v, want Unimplemented (err %v)", status.Code(err), err)
	}
	if _, err := client.AuthProviderChecks().EndSessionUrl(ctx, &pluginv1.AuthEndSessionUrlRequest{}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("EndSessionUrl code = %v, want Unimplemented (err %v)", status.Code(err), err)
	}
}
