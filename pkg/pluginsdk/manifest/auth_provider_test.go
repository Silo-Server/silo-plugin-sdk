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
