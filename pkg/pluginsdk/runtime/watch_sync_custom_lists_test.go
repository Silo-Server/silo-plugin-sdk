package runtime

import (
	"context"
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/capability"
)

// customLists answers ListCustomLists with the capability it was asked for,
// so a test can tell the request reached it.
type customLists struct {
	pluginv1.UnimplementedWatchSyncCustomListServiceServer
}

func (customLists) ListCustomLists(_ context.Context, req *pluginv1.WatchSyncCustomListServiceListCustomListsRequest) (*pluginv1.WatchSyncCustomListServiceListCustomListsResponse, error) {
	return &pluginv1.WatchSyncCustomListServiceListCustomListsResponse{Lists: []*pluginv1.WatchSyncCustomList{{
		Url: "https://lists.example/" + req.GetContext().GetCapabilityId(),
	}}}, nil
}

func customListsManifest(provides bool) *pluginv1.PluginManifest {
	return &pluginv1.PluginManifest{
		PluginId: "silo.test",
		Capabilities: []*pluginv1.CapabilityDescriptor{{
			Type:              capability.WatchSyncProvider,
			Id:                "lists",
			WatchSyncProvider: &pluginv1.WatchSyncProviderDescriptor{ProvidesCustomLists: provides},
		}},
	}
}

func TestManifestServeConfigServesWatchSyncCustomLists(t *testing.T) {
	cfg, err := manifestServeConfig(customListsManifest(true), CapabilityServers{}, WithWatchSyncCustomLists(customLists{}))
	if err != nil {
		t.Fatalf("manifestServeConfig = %v", err)
	}
	resp, err := serveConfigClient(t, cfg).WatchSyncCustomLists().ListCustomLists(context.Background(),
		&pluginv1.WatchSyncCustomListServiceListCustomListsRequest{Context: &pluginv1.WatchSyncAuthenticatedContext{CapabilityId: "lists"}})
	if err != nil || len(resp.GetLists()) != 1 || resp.GetLists()[0].GetUrl() != "https://lists.example/lists" {
		t.Fatalf("ListCustomLists = %v, %v", resp, err)
	}
}

// Hosts call the service only for a capability that sets the flag, so the
// flag without a server would fail every call.
func TestManifestServeConfigRejectsCustomListsFlagWithoutServer(t *testing.T) {
	for name, option := range map[string]ServeManifestOption{
		"no option": nil,
		"typed nil": WithWatchSyncCustomLists((*customLists)(nil)),
	} {
		_, err := manifestServeConfig(customListsManifest(true), CapabilityServers{}, option)
		if err == nil || !strings.Contains(err.Error(), "WithWatchSyncCustomLists") {
			t.Errorf("%s: manifestServeConfig = %v, want an error naming WithWatchSyncCustomLists", name, err)
		}
	}
}

// A server without the flag is never called, which hides a plugin that forgot
// to declare it.
func TestManifestServeConfigRejectsCustomListsServerWithoutFlag(t *testing.T) {
	_, err := manifestServeConfig(customListsManifest(false), CapabilityServers{}, WithWatchSyncCustomLists(customLists{}))
	if err == nil || !strings.Contains(err.Error(), "provides_custom_lists") {
		t.Fatalf("manifestServeConfig = %v, want an error naming provides_custom_lists", err)
	}
	if _, err := manifestServeConfig(customListsManifest(false), CapabilityServers{}); err != nil {
		t.Fatalf("manifestServeConfig without the flag or a server = %v, want nil", err)
	}
}
