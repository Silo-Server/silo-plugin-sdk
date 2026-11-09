package runtimehost_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtimehost"
)

type hostAPIServer struct {
	pluginv1.UnimplementedHostAPICredentialsServer
	resp *pluginv1.GetHostAPICredentialResponse
	err  error
}

func (s *hostAPIServer) GetHostAPICredential(context.Context, *pluginv1.GetHostAPICredentialRequest) (*pluginv1.GetHostAPICredentialResponse, error) {
	return s.resp, s.err
}

func TestHostAPICredential(t *testing.T) {
	for _, tc := range []struct {
		name    string
		server  pluginv1.HostAPICredentialsServer // nil: a host that predates the service
		wantErr error
	}{
		{name: "granted", server: &hostAPIServer{resp: &pluginv1.GetHostAPICredentialResponse{
			Token: "sa_x", Scopes: []string{"admin:monitor:read"}, BaseUrl: "http://127.0.0.1:8080",
		}}},
		{name: "not approved", server: &hostAPIServer{err: status.Error(codes.FailedPrecondition, "awaiting approval")}, wantErr: runtimehost.ErrHostAPINotGranted},
		{name: "empty token", server: &hostAPIServer{resp: &pluginv1.GetHostAPICredentialResponse{}}, wantErr: runtimehost.ErrHostAPINotGranted},
		{name: "older host", server: nil, wantErr: runtimehost.ErrHostAPIUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := runtimehost.NewClient(dialHostAPI(t, tc.server))
			cred, err := client.HostAPICredential(context.Background())
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("HostAPICredential() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil || cred.Token != "sa_x" || cred.BaseURL != "http://127.0.0.1:8080" || len(cred.Scopes) != 1 {
				t.Fatalf("HostAPICredential() = %+v, %v", cred, err)
			}
		})
	}
}

// dialHostAPI serves RuntimeHost and, unless server is nil, HostAPICredentials
// on one connection, as a host does.
func dialHostAPI(t *testing.T, server pluginv1.HostAPICredentialsServer) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	g := grpc.NewServer()
	pluginv1.RegisterRuntimeHostServer(g, &pluginv1.UnimplementedRuntimeHostServer{})
	if server != nil {
		pluginv1.RegisterHostAPICredentialsServer(g, server)
	}
	go func() { _ = g.Serve(lis) }()
	t.Cleanup(g.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
