package manifest_test

import (
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

func authManifest(capabilityJSON string) []byte {
	return []byte(`{
		"plugin_id": "test.auth",
		"version": "0.1.0",
		"checksum": "0",
		"silo_api_version": "v1",
		"capabilities": [` + capabilityJSON + `]
	}`)
}

// Unknown non-empty modes load so a plugin built against a newer SDK still
// starts on an older host, which ignores modes it does not recognize.
func TestLoadAcceptsKnownAndUnknownAuthModes(t *testing.T) {
	for _, modes := range []string{`[]`, `["password"]`, `["oauth2"]`, `["password", "oauth2"]`, `["saml"]`, `["oauth2", "passkey"]`} {
		raw := authManifest(`{"type": "auth_provider.v1", "id": "main", "auth_modes": ` + modes + `}`)
		if _, err := manifest.Load(raw); err != nil {
			t.Errorf("auth_modes %s: Load = %v, want nil", modes, err)
		}
	}
}

func TestLoadRejectsInvalidAuthModes(t *testing.T) {
	cases := map[string]string{
		`[""]`:                 "empty auth mode",
		`["  "]`:               "empty auth mode",
		`["OAuth2"]`:           `must be spelled "oauth2"`,
		`["Password"]`:         `must be spelled "password"`,
		`["oauth2", "oauth2"]`: "duplicate auth mode",
		`["saml", "saml"]`:     "duplicate auth mode",
	}
	for modes, want := range cases {
		raw := authManifest(`{"type": "auth_provider.v1", "id": "main", "auth_modes": ` + modes + `}`)
		_, err := manifest.Load(raw)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("auth_modes %s: Load = %v, want error containing %q", modes, err, want)
		}
	}
}

// Auth modes are only checked for auth providers; other capability types
// never carried them and keep loading as before.
func TestLoadIgnoresAuthModesOnOtherCapabilities(t *testing.T) {
	raw := authManifest(`{"type": "scheduled_task.v1", "id": "sync", "auth_modes": ["anything"]}`)
	if _, err := manifest.Load(raw); err != nil {
		t.Fatalf("Load = %v, want nil", err)
	}
}

func TestConnectionTestMetadata(t *testing.T) {
	cases := []struct {
		name     string
		metadata string
		want     bool
	}{
		{"absent", ``, false},
		{"true", `, "metadata": {"connection_test": true}`, true},
		{"false", `, "metadata": {"connection_test": false}`, false},
		{"other keys", `, "metadata": {"priority": 1}`, false},
	}
	for _, tc := range cases {
		raw := authManifest(`{"type": "auth_provider.v1", "id": "main", "auth_modes": ["oauth2"]` + tc.metadata + `}`)
		m, err := manifest.Load(raw)
		if err != nil {
			t.Fatalf("%s: Load = %v", tc.name, err)
		}
		if got := manifest.AuthProviderSupportsConnectionTest(m.GetCapabilities()[0]); got != tc.want {
			t.Errorf("%s: AuthProviderSupportsConnectionTest = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestLoadRejectsNonBooleanConnectionTest(t *testing.T) {
	for _, value := range []string{`"true"`, `1`, `null`, `{}`} {
		raw := authManifest(`{"type": "auth_provider.v1", "id": "main", "metadata": {"connection_test": ` + value + `}}`)
		_, err := manifest.Load(raw)
		if err == nil || !strings.Contains(err.Error(), "must be a boolean") {
			t.Errorf("connection_test %s: Load = %v, want boolean error", value, err)
		}
	}
}

func TestConnectionTestMetadataIgnoredOnOtherCapabilities(t *testing.T) {
	raw := authManifest(`{"type": "http_routes.v1", "id": "ui", "metadata": {"connection_test": "yes"}}`)
	m, err := manifest.Load(raw)
	if err != nil {
		t.Fatalf("Load = %v, want nil", err)
	}
	if manifest.AuthProviderSupportsConnectionTest(m.GetCapabilities()[0]) {
		t.Fatal("non-auth capability reported connection test support")
	}
	if manifest.AuthProviderSupportsConnectionTest(&pluginv1.CapabilityDescriptor{}) {
		t.Fatal("empty descriptor reported connection test support")
	}
}

const networkAccessCapability = `{"type": "network_access_provider.v1", "id": "overlay",
	"network_access_provider": {"provider": "tailscale", "display_name": "Tailscale"}}`

func TestLoadAcceptsNetworkAuthWithNetworkAccess(t *testing.T) {
	raw := authManifest(networkAccessCapability + `, {"type": "auth_provider.v1", "id": "tailscale", "auth_modes": ["network"]}`)
	m, err := manifest.Load(raw)
	if err != nil {
		t.Fatalf("Load = %v, want nil", err)
	}
	if !manifest.AuthProviderUsesNetworkIdentity(m.GetCapabilities()[1]) {
		t.Fatal("AuthProviderUsesNetworkIdentity = false, want true")
	}
	if manifest.AuthProviderUsesNetworkIdentity(m.GetCapabilities()[0]) {
		t.Fatal("AuthProviderUsesNetworkIdentity on the network access capability = true, want false")
	}
}

func TestLoadRejectsNetworkAuthWithoutNetworkAccess(t *testing.T) {
	raw := authManifest(`{"type": "auth_provider.v1", "id": "tailscale", "auth_modes": ["network"]}`)
	_, err := manifest.Load(raw)
	if err == nil || !strings.Contains(err.Error(), "network_access_provider.v1") {
		t.Fatalf("Load = %v, want an error naming network_access_provider.v1", err)
	}
}

// A network provider never takes a password or runs a browser flow, so mixing
// modes would let a host route credentials to it.
func TestLoadRejectsNetworkAuthCombinedWithCredentialModes(t *testing.T) {
	for _, modes := range []string{`["network", "password"]`, `["oauth2", "network"]`} {
		raw := authManifest(networkAccessCapability + `, {"type": "auth_provider.v1", "id": "tailscale", "auth_modes": ` + modes + `}`)
		_, err := manifest.Load(raw)
		if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
			t.Errorf("auth_modes %s: Load = %v, want a combination error", modes, err)
		}
	}
}

// auth_modes is an open vocabulary: a mode a later SDK allows beside network
// must not stop this SDK's Validate from loading the plugin.
func TestLoadAcceptsNetworkAuthWithUnknownMode(t *testing.T) {
	raw := authManifest(networkAccessCapability + `, {"type": "auth_provider.v1", "id": "tailscale", "auth_modes": ["network", "network_v2"]}`)
	if _, err := manifest.Load(raw); err != nil {
		t.Fatalf("Load = %v, want nil", err)
	}
}

func TestLoadRejectsMisspelledNetworkMode(t *testing.T) {
	raw := authManifest(networkAccessCapability + `, {"type": "auth_provider.v1", "id": "tailscale", "auth_modes": ["Network"]}`)
	_, err := manifest.Load(raw)
	if err == nil || !strings.Contains(err.Error(), `must be spelled "network"`) {
		t.Fatalf("Load = %v, want a spelling error", err)
	}
}

func TestAuthProviderUsesNetworkIdentityIgnoresOtherTypes(t *testing.T) {
	descriptor := &pluginv1.CapabilityDescriptor{Type: "scheduled_task.v1", Id: "sync", AuthModes: []string{"network"}}
	if manifest.AuthProviderUsesNetworkIdentity(descriptor) {
		t.Fatal("AuthProviderUsesNetworkIdentity = true for a scheduled task, want false")
	}
}
