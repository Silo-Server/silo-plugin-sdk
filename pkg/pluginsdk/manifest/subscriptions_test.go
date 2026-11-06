package manifest_test

import (
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	publicmanifest "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

func TestSubscriptionProblems(t *testing.T) {
	for _, tc := range []struct {
		name          string
		typ           string
		subscriptions []string
		want          string
	}{
		{name: "catalog and plugin events", typ: "event_consumer.v1", subscriptions: []string{"session.started", "plugin.silo.pushover.sent"}},
		{name: "duplicate", typ: "event_consumer.v1", subscriptions: []string{"scan.failed", "scan.failed"}, want: "listed twice"},
		{name: "internal name", typ: "event_consumer.v1", subscriptions: []string{"scan_complete"}, want: "not an event name"},
		{name: "not in the catalog", typ: "event_consumer.v1", subscriptions: []string{"user_state.changed"}, want: "not in the event catalog"},
		{name: "wrong capability type", typ: "scheduled_task.v1", subscriptions: []string{"scan.failed"}, want: "need type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := &pluginv1.PluginManifest{
				PluginId: "silo.test", Version: "1.0.0",
				Capabilities: []*pluginv1.CapabilityDescriptor{{Type: tc.typ, Id: "events", Subscriptions: tc.subscriptions}},
			}
			// Validate keeps accepting what it accepted before the catalog.
			if err := publicmanifest.Validate(manifest); err != nil {
				t.Fatalf("Validate() = %v; subscriptions must not stop a manifest from loading", err)
			}
			problems := publicmanifest.SubscriptionProblems(manifest)
			if tc.want == "" {
				if len(problems) != 0 {
					t.Fatalf("SubscriptionProblems() = %v, want none", problems)
				}
				return
			}
			if len(problems) == 0 || !strings.Contains(strings.Join(problems, "\n"), tc.want) {
				t.Fatalf("SubscriptionProblems() = %v, want one containing %q", problems, tc.want)
			}
		})
	}
}
