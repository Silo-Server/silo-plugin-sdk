package manifest_test

import (
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	publicmanifest "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

func TestValidateSubscriptions(t *testing.T) {
	for _, tc := range []struct {
		name          string
		typ           string
		subscriptions []string
		wantErr       string
	}{
		{name: "catalog and plugin events", typ: "event_consumer.v1", subscriptions: []string{"session.started", "plugin.silo.pushover.sent"}},
		// A newer SDK's event loads on an older host, which ignores it.
		{name: "event this SDK doesn't know", typ: "event_consumer.v1", subscriptions: []string{"media.added_someday"}},
		{name: "duplicate", typ: "event_consumer.v1", subscriptions: []string{"scan.failed", "scan.failed"}, wantErr: "listed twice"},
		{name: "not an event name", typ: "event_consumer.v1", subscriptions: []string{"Session Started"}, wantErr: "not an event name"},
		{name: "single segment", typ: "event_consumer.v1", subscriptions: []string{"scan"}, wantErr: "not an event name"},
		{name: "wrong capability type", typ: "scheduled_task.v1", subscriptions: []string{"scan.failed"}, wantErr: "require type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := &pluginv1.PluginManifest{
				PluginId: "silo.test", Version: "1.0.0",
				Capabilities: []*pluginv1.CapabilityDescriptor{{Type: tc.typ, Id: "events", Subscriptions: tc.subscriptions}},
			}
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
