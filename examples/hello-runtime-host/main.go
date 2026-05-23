package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	goruntime "runtime"

	"github.com/hashicorp/go-hclog"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	publicmanifest "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
	sdkruntime "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtimedefault"
)

//go:embed manifest.json
var manifestJSON []byte

// runtimeServer embeds runtimedefault.Server so it inherits the BindHostBroker
// handler that wires the host's broker stream ID into the SDK's plugin-side
// singleton. The plugin author only writes GetManifest (and Configure if
// needed).
type runtimeServer struct {
	runtimedefault.Server
	manifest *pluginv1.PluginManifest
}

func (s *runtimeServer) GetManifest(_ context.Context, _ *pluginv1.GetManifestRequest) (*pluginv1.GetManifestResponse, error) {
	return &pluginv1.GetManifestResponse{Manifest: s.manifest}, nil
}

type scheduledTaskServer struct {
	pluginv1.UnimplementedScheduledTaskServer
	logger hclog.Logger
}

func (s *scheduledTaskServer) Run(ctx context.Context, _ *pluginv1.RunScheduledTaskRequest) (*pluginv1.RunScheduledTaskResponse, error) {
	host := sdkruntime.Host()
	if host == nil {
		s.logger.Warn("runtimehost not bound; skipping publish")
		return &pluginv1.RunScheduledTaskResponse{}, nil
	}
	if err := host.PublishEvent(ctx, "ping", map[string]any{"message": "hello"}); err != nil {
		s.logger.Error("publish event", "err", err)
		return nil, err
	}
	return &pluginv1.RunScheduledTaskResponse{}, nil
}

func main() {
	logger := hclog.New(&hclog.LoggerOptions{Name: "hello-runtime-host"})

	manifest, err := loadManifest()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load manifest: %v\n", err)
		os.Exit(1)
	}

	sdkruntime.Serve(sdkruntime.ServeConfig{
		Logger: logger,
		Servers: sdkruntime.CapabilityServers{
			Runtime:       &runtimeServer{manifest: manifest},
			ScheduledTask: &scheduledTaskServer{logger: logger},
		},
	})
}

func loadManifest() (*pluginv1.PluginManifest, error) {
	manifest, err := publicmanifest.Load(manifestJSON)
	if err != nil {
		return nil, fmt.Errorf("load embedded manifest: %w", err)
	}

	executablePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}
	binaryData, err := os.ReadFile(executablePath)
	if err != nil {
		return nil, fmt.Errorf("read executable %q: %w", executablePath, err)
	}
	checksum := sha256.Sum256(binaryData)
	manifest.Checksum = hex.EncodeToString(checksum[:])
	if len(manifest.GetSupportedPlatforms()) == 0 {
		manifest.SupportedPlatforms = []*pluginv1.SupportedPlatform{
			{Os: goruntime.GOOS, Arch: goruntime.GOARCH},
		}
	}

	return manifest, nil
}
