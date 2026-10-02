package runtime

import (
	"context"
	"fmt"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

// manifestRuntime is the default Runtime capability server installed by
// ServeManifest: it answers GetManifest with the embedded manifest, passes
// Configure to the WithConfigure callback (a no-op without one), and wires the
// host broker so runtime.Host() works.
//
// BindHostBroker is implemented inline (calling the same-package SetHostBrokerID)
// rather than by embedding runtimedefault.Server, which would create a
// runtime -> runtimedefault -> runtime import cycle.
type manifestRuntime struct {
	pluginv1.UnimplementedRuntimeServer
	manifest  *pluginv1.PluginManifest
	configure ConfigureFunc
}

// ConfigureFunc receives the plugin's saved configuration when the host calls
// Runtime.Configure. The host calls it once per process start, before it
// routes capability calls, and restarts the plugin when an operator saves new
// settings. Entries are keyed by manifest config key; secret fields carry
// plaintext.
//
// Configure must succeed when settings are empty or incomplete: a fresh
// install sends no entries, and the host must still be able to start the
// plugin to answer TestConnection or to report the problem at sign-in. Store
// whatever arrives and return an error only for input the plugin cannot store
// at all, such as a value it cannot decode. A returned error fails Configure,
// and the host abandons the start. Report missing or invalid settings, such as
// an empty issuer, as failed TestConnection steps and, at sign-in, as
// AUTH_DENIAL_PROVIDER_UNAVAILABLE or a gRPC error.
//
// The host bounds the call with a short control timeout, so parse and store
// settings here and leave network calls to the capability RPCs.
type ConfigureFunc func(ctx context.Context, entries []*pluginv1.ConfigEntry) error

type serveManifestOptions struct {
	watchSyncDeviceAuthorization pluginv1.WatchSyncDeviceAuthorizationServiceServer
	authProviderChecks           pluginv1.AuthProviderChecksServer
	networkIdentityAuth          pluginv1.NetworkIdentityAuthServer
	configure                    ConfigureFunc
}

// ServeManifestOption extends ServeManifestWithOptions, for example with an
// extra service or a Configure callback, without changing the released
// CapabilityServers or ServeConfig struct layouts.
type ServeManifestOption func(*serveManifestOptions)

// WithWatchSyncDeviceAuthorization registers the separate device-code service
// for a watch-sync provider that advertises DEVICE_CODE authentication.
func WithWatchSyncDeviceAuthorization(
	server pluginv1.WatchSyncDeviceAuthorizationServiceServer,
) ServeManifestOption {
	return func(options *serveManifestOptions) {
		options.watchSyncDeviceAuthorization = server
	}
}

// WithAuthProviderChecks registers the separate AuthProviderChecks service
// (TestConnection, CheckAccount, EndSessionUrl) for an auth provider. Without
// it, the runtime registers the service only when the AuthProvider server
// itself implements AuthProviderChecksServer.
func WithAuthProviderChecks(server pluginv1.AuthProviderChecksServer) ServeManifestOption {
	return func(options *serveManifestOptions) {
		options.authProviderChecks = server
	}
}

// WithNetworkIdentityAuth registers the separate NetworkIdentityAuth service
// (AuthenticatePeer) for an auth provider that declares the "network" auth
// mode. Without it, the runtime registers the service only when the
// AuthProvider server itself implements NetworkIdentityAuthServer.
func WithNetworkIdentityAuth(server pluginv1.NetworkIdentityAuthServer) ServeManifestOption {
	return func(options *serveManifestOptions) {
		options.networkIdentityAuth = server
	}
}

// WithConfigure delivers Runtime.Configure entries to fn, so a plugin served by
// ServeManifestWithOptions can read its settings without writing its own
// Runtime server. See ConfigureFunc for what fn may reject.
func WithConfigure(fn ConfigureFunc) ServeManifestOption {
	return func(options *serveManifestOptions) {
		options.configure = fn
	}
}

func (s *manifestRuntime) GetManifest(context.Context, *pluginv1.GetManifestRequest) (*pluginv1.GetManifestResponse, error) {
	return &pluginv1.GetManifestResponse{Manifest: s.manifest}, nil
}

func (s *manifestRuntime) Configure(ctx context.Context, req *pluginv1.ConfigureRequest) (*pluginv1.ConfigureResponse, error) {
	if s.configure != nil {
		if err := s.configure(ctx, req.GetConfig()); err != nil {
			return nil, err
		}
	}
	return &pluginv1.ConfigureResponse{}, nil
}

func (s *manifestRuntime) BindHostBroker(_ context.Context, req *pluginv1.BindHostBrokerRequest) (*pluginv1.BindHostBrokerResponse, error) {
	SetHostBrokerID(req.GetBrokerId())
	return &pluginv1.BindHostBrokerResponse{}, nil
}

// ServeManifest loads + checksums the embedded manifest, installs the default
// manifestRuntime as the Runtime server, and serves the given capability
// servers (the caller supplies only the non-Runtime servers). It never returns;
// a fatal manifest error panics, matching a misbuilt plugin's old main().
func ServeManifest(manifestBytes []byte, version string, servers CapabilityServers) {
	ServeManifestWithOptions(manifestBytes, version, servers)
}

// ServeManifestWithOptions is ServeManifest plus options that add services or
// a Configure callback introduced after the released CapabilityServers shape.
// Besides manifest errors, it panics when an auth_provider.v1 capability
// declares "connection_test": true but no AuthProviderChecks server would be
// registered, or declares the "network" auth mode but no NetworkIdentityAuth
// server would be registered.
func ServeManifestWithOptions(
	manifestBytes []byte,
	version string,
	servers CapabilityServers,
	options ...ServeManifestOption,
) {
	m, err := manifest.LoadWithChecksum(manifestBytes, version)
	if err != nil {
		panic(err)
	}
	cfg, err := manifestServeConfig(m, servers, options...)
	if err != nil {
		panic(err)
	}
	Serve(cfg)
}

// manifestServeConfig resolves the options and builds what
// ServeManifestWithOptions serves. It is split out so tests can check the
// wiring without starting a plugin process.
func manifestServeConfig(
	m *pluginv1.PluginManifest,
	servers CapabilityServers,
	options ...ServeManifestOption,
) (ServeConfig, error) {
	var resolved serveManifestOptions
	for _, option := range options {
		if option != nil {
			option(&resolved)
		}
	}
	if resolveAuthProviderChecks(servers.AuthProvider, resolved.authProviderChecks) == nil {
		for _, descriptor := range m.GetCapabilities() {
			if manifest.AuthProviderSupportsConnectionTest(descriptor) {
				return ServeConfig{}, fmt.Errorf(
					"plugin capability %q declares %q but no AuthProviderChecks server is registered; pass runtime.WithAuthProviderChecks",
					descriptor.GetId(), manifest.AuthProviderConnectionTestKey)
			}
		}
	}
	if resolveNetworkIdentityAuth(servers.AuthProvider, resolved.networkIdentityAuth) == nil {
		for _, descriptor := range m.GetCapabilities() {
			if manifest.AuthProviderUsesNetworkIdentity(descriptor) {
				return ServeConfig{}, fmt.Errorf(
					"plugin capability %q declares auth mode %q but no NetworkIdentityAuth server is registered; pass runtime.WithNetworkIdentityAuth",
					descriptor.GetId(), manifest.AuthModeNetwork)
			}
		}
	}
	servers.Runtime = &manifestRuntime{manifest: m, configure: resolved.configure}
	return ServeConfig{
		Servers: servers,
		Plugins: pluginSetWithOptionalServices(servers, optionalServices{
			watchSyncDeviceAuthorization: resolved.watchSyncDeviceAuthorization,
			authProviderChecks:           resolved.authProviderChecks,
			networkIdentityAuth:          resolved.networkIdentityAuth,
		}),
	}, nil
}
