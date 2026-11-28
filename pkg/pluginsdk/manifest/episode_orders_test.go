package manifest_test

import (
	"strings"
	"testing"

	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

func metadataManifest(capabilityJSON string) []byte {
	return []byte(`{
		"plugin_id": "test.metadata",
		"version": "0.1.0",
		"checksum": "0",
		"silo_api_version": "v1",
		"capabilities": [` + capabilityJSON + `]
	}`)
}

func TestEpisodeOrdersMetadata(t *testing.T) {
	cases := []struct {
		name       string
		capability string
		want       bool
	}{
		{"absent", `{"type": "metadata_provider.v1", "id": "series"}`, false},
		{"true", `{"type": "metadata_provider.v1", "id": "series", "metadata": {"episode_orders": true}}`, true},
		{"false", `{"type": "metadata_provider.v1", "id": "series", "metadata": {"episode_orders": false}}`, false},
		// Only metadata providers serve GetSeasons and GetEpisodes.
		{"other capability", `{"type": "http_routes.v1", "id": "ui", "metadata": {"episode_orders": true}}`, false},
	}
	for _, tc := range cases {
		m, err := manifest.Load(metadataManifest(tc.capability))
		if err != nil {
			t.Fatalf("%s: Load = %v", tc.name, err)
		}
		if got := manifest.MetadataProviderSupportsEpisodeOrders(m.GetCapabilities()[0]); got != tc.want {
			t.Errorf("%s: MetadataProviderSupportsEpisodeOrders = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestLoadRejectsNonBooleanEpisodeOrders(t *testing.T) {
	for _, value := range []string{`"true"`, `1`, `null`, `{}`} {
		raw := metadataManifest(`{"type": "metadata_provider.v1", "id": "series", "metadata": {"episode_orders": ` + value + `}}`)
		_, err := manifest.Load(raw)
		if err == nil || !strings.Contains(err.Error(), "must be a boolean") {
			t.Errorf("episode_orders %s: Load = %v, want boolean error", value, err)
		}
	}
}
