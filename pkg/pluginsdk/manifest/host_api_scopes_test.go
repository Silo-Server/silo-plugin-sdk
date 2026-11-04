package manifest_test

import (
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	publicmanifest "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

func TestValidateHostAPIScopes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		scopes  []string
		wantErr string
	}{
		{name: "none"},
		{name: "known scopes", scopes: []string{"admin:monitor:read", "admin:sessions:control"}},
		// The host's catalog may have scopes this SDK has never heard of.
		{name: "scope this SDK doesn't know", scopes: []string{"admin:requests:read"}},
		{name: "duplicate", scopes: []string{"libraries:read", "libraries:read"}, wantErr: "listed twice"},
		{name: "no segments", scopes: []string{"admin"}, wantErr: "not an API key scope"},
		{name: "uppercase", scopes: []string{"Admin:Monitor:Read"}, wantErr: "not an API key scope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := &pluginv1.PluginManifest{PluginId: "silo.test", Version: "1.0.0", HostApiScopes: tc.scopes}
			err := publicmanifest.Validate(manifest)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadReadsHostAPIScopes(t *testing.T) {
	manifest, err := publicmanifest.Load([]byte(`{"plugin_id":"silo.test","version":"1.0.0","silo_api_version":"v1",
		"host_api_scopes":["admin:monitor:read"],
		"capabilities":[{"type":"scheduled_task.v1","id":"t"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := manifest.GetHostApiScopes(); len(got) != 1 || got[0] != "admin:monitor:read" {
		t.Fatalf("host_api_scopes = %v", got)
	}
}
