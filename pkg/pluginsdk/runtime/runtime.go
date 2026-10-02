package runtime

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtimehost"
)

const (
	ProtocolVersion  = 1
	MagicCookieKey   = "SILO_PLUGIN"
	MagicCookieValue = "silo-rpc-plugin-v1"
	PluginSetName    = "silo"
)

type CapabilityServers struct {
	Runtime           pluginv1.RuntimeServer
	MetadataProvider  pluginv1.MetadataProviderServer
	ImageResolver     pluginv1.ImageResolverServer
	MarkerProvider    pluginv1.MarkerProviderServer
	MediaAnalyzer     pluginv1.MediaAnalyzerServer
	ScheduledTask     pluginv1.ScheduledTaskServer
	ScanSource        pluginv1.ScanSourceServer
	RequestRouter     pluginv1.RequestRouterServer
	EventConsumer     pluginv1.EventConsumerServer
	AuthProvider      pluginv1.AuthProviderServer
	HttpRoutes        pluginv1.HttpRoutesServer
	WatchSyncProvider pluginv1.WatchSyncProviderServer
	// NetworkAccessProvider was added in v0.16.0. Plugins declaring
	// network_access_provider.v1 register it here; the host treats them as
	// resident and starts them at boot.
	NetworkAccessProvider pluginv1.NetworkAccessProviderServer
}

// Client wraps the gRPC connection to a plugin and provides typed accessors
// for plugin capabilities. The broker field is populated on the host side when
// the client is created via GRPCClient; it is nil when the client is
// constructed directly with NewClient.
type Client struct {
	conn   *grpc.ClientConn
	broker *plugin.GRPCBroker
}

type ServeConfig struct {
	Plugins plugin.PluginSet
	Logger  hclog.Logger
	Servers CapabilityServers
}

func HandshakeConfig() plugin.HandshakeConfig {
	return plugin.HandshakeConfig{
		ProtocolVersion:  ProtocolVersion,
		MagicCookieKey:   MagicCookieKey,
		MagicCookieValue: MagicCookieValue,
	}
}

func DefaultGRPCServer(opts []grpc.ServerOption) *grpc.Server {
	return grpc.NewServer(opts...)
}

func DefaultPluginSet(servers CapabilityServers) plugin.PluginSet {
	return plugin.PluginSet{
		PluginSetName: &GRPCPlugin{Servers: servers},
	}
}

// DefaultPluginSetWithWatchSyncDeviceAuthorization returns the default plugin
// set plus the separate device-authorization service. Keeping that server out
// of CapabilityServers preserves the released v0.12 unkeyed struct shape.
func DefaultPluginSetWithWatchSyncDeviceAuthorization(
	servers CapabilityServers,
	deviceAuthorization pluginv1.WatchSyncDeviceAuthorizationServiceServer,
) plugin.PluginSet {
	return pluginSetWithOptionalServices(servers, optionalServices{
		watchSyncDeviceAuthorization: deviceAuthorization,
	})
}

// optionalServices are services added after the released CapabilityServers
// shape. They reach the plugin set through ServeManifest options instead of
// new struct fields.
type optionalServices struct {
	watchSyncDeviceAuthorization pluginv1.WatchSyncDeviceAuthorizationServiceServer
	authProviderChecks           pluginv1.AuthProviderChecksServer
	networkIdentityAuth          pluginv1.NetworkIdentityAuthServer
}

func pluginSetWithOptionalServices(servers CapabilityServers, optional optionalServices) plugin.PluginSet {
	// Compare fields one by one: == on the struct would panic for a server
	// whose dynamic type is not comparable.
	if optional.watchSyncDeviceAuthorization == nil && optional.authProviderChecks == nil &&
		optional.networkIdentityAuth == nil {
		return DefaultPluginSet(servers)
	}
	return plugin.PluginSet{
		PluginSetName: &grpcPluginWithOptionalServices{
			GRPCPlugin: &GRPCPlugin{Servers: servers},
			optional:   optional,
		},
	}
}

func NewClient(conn *grpc.ClientConn) *Client {
	return &Client{conn: conn}
}

func (c *Client) Conn() *grpc.ClientConn {
	return c.conn
}

// Broker returns the gRPC broker that was injected when the client was
// dispensed via go-plugin on the host side. It is nil when the client was
// constructed directly via NewClient (e.g. in unit tests or plugin-side code).
func (c *Client) Broker() *plugin.GRPCBroker {
	return c.broker
}

func (c *Client) Runtime() pluginv1.RuntimeClient {
	return pluginv1.NewRuntimeClient(c.conn)
}

func (c *Client) MetadataProvider() pluginv1.MetadataProviderClient {
	return pluginv1.NewMetadataProviderClient(c.conn)
}

func (c *Client) ImageResolver() pluginv1.ImageResolverClient {
	return pluginv1.NewImageResolverClient(c.conn)
}

func (c *Client) MarkerProvider() pluginv1.MarkerProviderClient {
	return pluginv1.NewMarkerProviderClient(c.conn)
}

func (c *Client) MediaAnalyzer() pluginv1.MediaAnalyzerClient {
	return pluginv1.NewMediaAnalyzerClient(c.conn)
}

func (c *Client) ScheduledTask() pluginv1.ScheduledTaskClient {
	return pluginv1.NewScheduledTaskClient(c.conn)
}

func (c *Client) ScanSource() pluginv1.ScanSourceClient {
	return pluginv1.NewScanSourceClient(c.conn)
}

func (c *Client) RequestRouter() pluginv1.RequestRouterClient {
	return pluginv1.NewRequestRouterClient(c.conn)
}

func (c *Client) EventConsumer() pluginv1.EventConsumerClient {
	return pluginv1.NewEventConsumerClient(c.conn)
}

func (c *Client) AuthProvider() pluginv1.AuthProviderClient {
	return pluginv1.NewAuthProviderClient(c.conn)
}

// AuthProviderChecks returns the client for the connection-test and
// account-check RPCs added in v0.22.0. Plugins built before then answer
// Unimplemented.
func (c *Client) AuthProviderChecks() pluginv1.AuthProviderChecksClient {
	return pluginv1.NewAuthProviderChecksClient(c.conn)
}

// NetworkIdentityAuth returns the client for overlay-peer sign-in, added in
// v0.23.0. Plugins that do not declare the "network" auth mode answer
// Unimplemented.
func (c *Client) NetworkIdentityAuth() pluginv1.NetworkIdentityAuthClient {
	return pluginv1.NewNetworkIdentityAuthClient(c.conn)
}

func (c *Client) HttpRoutes() pluginv1.HttpRoutesClient {
	return pluginv1.NewHttpRoutesClient(c.conn)
}

func (c *Client) WatchSyncProvider() pluginv1.WatchSyncProviderClient {
	return pluginv1.NewWatchSyncProviderClient(c.conn)
}

func (c *Client) WatchSyncDeviceAuthorization() pluginv1.WatchSyncDeviceAuthorizationServiceClient {
	return pluginv1.NewWatchSyncDeviceAuthorizationServiceClient(c.conn)
}

func (c *Client) NetworkAccessProvider() pluginv1.NetworkAccessProviderClient {
	return pluginv1.NewNetworkAccessProviderClient(c.conn)
}

type GRPCPlugin struct {
	plugin.Plugin
	Servers CapabilityServers
}

type grpcPluginWithOptionalServices struct {
	*GRPCPlugin
	optional optionalServices
}

func (p *grpcPluginWithOptionalServices) GRPCServer(
	broker *plugin.GRPCBroker,
	server *grpc.Server,
) error {
	if err := p.registerServers(broker, server, p.optional); err != nil {
		return err
	}
	if p.optional.watchSyncDeviceAuthorization != nil {
		pluginv1.RegisterWatchSyncDeviceAuthorizationServiceServer(server, p.optional.watchSyncDeviceAuthorization)
	}
	return nil
}

func (p *GRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, server *grpc.Server) error {
	return p.registerServers(broker, server, optionalServices{})
}

// resolveAuthProviderChecks returns the AuthProviderChecks server to register:
// the explicit one from WithAuthProviderChecks, else the AuthProvider server
// when it also implements AuthProviderChecksServer, else nil.
func resolveAuthProviderChecks(
	authProvider pluginv1.AuthProviderServer,
	explicit pluginv1.AuthProviderChecksServer,
) pluginv1.AuthProviderChecksServer {
	if explicit != nil {
		return explicit
	}
	if checks, ok := authProvider.(pluginv1.AuthProviderChecksServer); ok {
		return checks
	}
	return nil
}

// resolveNetworkIdentityAuth returns the NetworkIdentityAuth server to
// register: the explicit one from WithNetworkIdentityAuth, else the
// AuthProvider server when it also implements NetworkIdentityAuthServer, else
// nil.
func resolveNetworkIdentityAuth(
	authProvider pluginv1.AuthProviderServer,
	explicit pluginv1.NetworkIdentityAuthServer,
) pluginv1.NetworkIdentityAuthServer {
	if explicit != nil {
		return explicit
	}
	if network, ok := authProvider.(pluginv1.NetworkIdentityAuthServer); ok {
		return network
	}
	return nil
}

func (p *GRPCPlugin) registerServers(
	broker *plugin.GRPCBroker,
	server *grpc.Server,
	optional optionalServices,
) error {
	pluginHost.setBroker(broker)
	if p.Servers.Runtime == nil {
		return fmt.Errorf("runtime server is required")
	}

	pluginv1.RegisterRuntimeServer(server, p.Servers.Runtime)
	if p.Servers.MetadataProvider != nil {
		pluginv1.RegisterMetadataProviderServer(server, p.Servers.MetadataProvider)
	}
	if p.Servers.ImageResolver != nil {
		pluginv1.RegisterImageResolverServer(server, p.Servers.ImageResolver)
	}
	if p.Servers.MarkerProvider != nil {
		pluginv1.RegisterMarkerProviderServer(server, p.Servers.MarkerProvider)
	}
	if p.Servers.MediaAnalyzer != nil {
		pluginv1.RegisterMediaAnalyzerServer(server, p.Servers.MediaAnalyzer)
	}
	if p.Servers.ScheduledTask != nil {
		pluginv1.RegisterScheduledTaskServer(server, p.Servers.ScheduledTask)
	}
	if p.Servers.ScanSource != nil {
		pluginv1.RegisterScanSourceServer(server, p.Servers.ScanSource)
	}
	if p.Servers.RequestRouter != nil {
		pluginv1.RegisterRequestRouterServer(server, p.Servers.RequestRouter)
	}
	if p.Servers.EventConsumer != nil {
		pluginv1.RegisterEventConsumerServer(server, p.Servers.EventConsumer)
	}
	if p.Servers.AuthProvider != nil {
		pluginv1.RegisterAuthProviderServer(server, p.Servers.AuthProvider)
	}
	// AuthProviderChecks lives in its own service so the released
	// AuthProviderServer interface stays unchanged.
	if checks := resolveAuthProviderChecks(p.Servers.AuthProvider, optional.authProviderChecks); checks != nil {
		pluginv1.RegisterAuthProviderChecksServer(server, checks)
	}
	if network := resolveNetworkIdentityAuth(p.Servers.AuthProvider, optional.networkIdentityAuth); network != nil {
		pluginv1.RegisterNetworkIdentityAuthServer(server, network)
	}
	if p.Servers.HttpRoutes != nil {
		pluginv1.RegisterHttpRoutesServer(server, p.Servers.HttpRoutes)
	}
	if p.Servers.WatchSyncProvider != nil {
		pluginv1.RegisterWatchSyncProviderServer(server, p.Servers.WatchSyncProvider)
	}
	if p.Servers.NetworkAccessProvider != nil {
		pluginv1.RegisterNetworkAccessProviderServer(server, p.Servers.NetworkAccessProvider)
	}
	return nil
}

func (p *GRPCPlugin) GRPCClient(_ context.Context, broker *plugin.GRPCBroker, conn *grpc.ClientConn) (interface{}, error) {
	return &Client{conn: conn, broker: broker}, nil
}

// pluginHostState is a process-singleton holding the plugin-side broker and
// the host-assigned broker stream ID for RuntimeHost. The singleton model is
// appropriate because go-plugin runs exactly one plugin per process.
//
// The dialed *runtimehost.Client is cached: silo's bindRuntimeHost
// registers ONE go-plugin broker stream and AcceptAndServe handles a single
// connection on it (multiplexing many gRPC RPCs). Re-dialing per call would
// open a fresh stream the host isn't listening on, causing every call after
// the first to hang on its way to a timeout. Holding one client and reusing
// it keeps all RPCs on the original stream.
type pluginHostState struct {
	mu       sync.Mutex
	broker   *plugin.GRPCBroker
	brokerID uint32
	client   *runtimehost.Client
}

var pluginHost = &pluginHostState{}

func (s *pluginHostState) setBroker(b *plugin.GRPCBroker) {
	s.mu.Lock()
	s.broker = b
	// Invalidate stream state tied to a previous broker.
	s.brokerID = 0
	s.client = nil
	s.mu.Unlock()
}

// setBrokerID records the host-assigned stream and dials it at once.
//
// The dial cannot wait for the first Host() call: go-plugin's broker keeps
// the connection info the host sent for a stream for only five seconds
// (GRPCBroker.timeoutWait), and the host sends it from its AcceptAndServe
// just before invoking BindHostBroker. A plugin whose first host call comes
// later than that, which is the normal case for a resident plugin that idles
// until an admin connects it, would find the stream expired and every
// Host() call would return nil for the life of the process. Dialing here
// pins the connection while the window is open; runtimehost calls then
// multiplex over it.
func (s *pluginHostState) setBrokerID(id uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.brokerID = id
	// new stream id → drop any cached client
	s.client = nil
	s.dialLocked()
}

// dialLocked connects to the bound stream if it has not been connected yet.
// The caller holds s.mu.
func (s *pluginHostState) dialLocked() {
	if s.client != nil || s.broker == nil || s.brokerID == 0 {
		return
	}
	conn, err := s.broker.Dial(s.brokerID)
	if err != nil {
		return
	}
	s.client = runtimehost.NewClient(conn)
}

func (s *pluginHostState) host() *runtimehost.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dialLocked()
	return s.client
}

// SetHostBrokerID stores the broker stream ID assigned by the host. The
// generated runtimedefault.Server's BindHostBroker handler calls this when the
// host invokes Runtime.BindHostBroker. Plugin authors do not call this
// directly when they embed runtimedefault.Server.
func SetHostBrokerID(id uint32) { pluginHost.setBrokerID(id) }

// Host returns a runtimehost.Client connected to the silo host. Returns
// nil before the host has invoked Runtime.BindHostBroker (i.e. very briefly
// during plugin startup) or if the broker dial failed at bind time.
// Capability handlers should treat nil as transient and either skip or
// surface a temporary error.
//
// The first successful call dials the host broker stream and caches the
// *runtimehost.Client; later calls reuse the same client.
func Host() *runtimehost.Client { return pluginHost.host() }

func Serve(cfg ServeConfig) {
	// Handle "manifest" subcommand: print the plugin manifest as JSON and exit.
	if len(os.Args) > 1 && os.Args[1] == "manifest" {
		if cfg.Servers.Runtime == nil {
			fmt.Fprintln(os.Stderr, "runtime server is required to retrieve manifest")
			os.Exit(1)
		}
		resp, err := cfg.Servers.Runtime.GetManifest(context.Background(), &pluginv1.GetManifestRequest{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get manifest: %v\n", err)
			os.Exit(1)
		}
		marshaler := protojson.MarshalOptions{Indent: "  "}
		data, err := marshaler.Marshal(resp.GetManifest())
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to encode manifest: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		os.Exit(0)
	}

	plugins := cfg.Plugins
	if len(plugins) == 0 {
		plugins = DefaultPluginSet(cfg.Servers)
	}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: HandshakeConfig(),
		Plugins:         plugins,
		GRPCServer:      plugin.DefaultGRPCServer,
		Logger:          cfg.Logger,
	})
}
