package manifest_test

import (
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	publicmanifest "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

func TestValidateScheduledTaskDefaultTrigger(t *testing.T) {
	for _, tc := range []struct {
		name    string
		typ     string
		trigger *pluginv1.ScheduledTaskTrigger
		wantErr string
	}{
		{name: "interval", typ: "scheduled_task.v1", trigger: &pluginv1.ScheduledTaskTrigger{Type: "interval", IntervalMs: 300_000}},
		{name: "daily", typ: "scheduled_task.v1", trigger: &pluginv1.ScheduledTaskTrigger{Type: "daily", TimeOfDay: "03:30"}},
		{name: "weekly", typ: "scheduled_task.v1", trigger: &pluginv1.ScheduledTaskTrigger{Type: "weekly", TimeOfDay: "18:00", DayOfWeek: 0}},
		{name: "startup", typ: "scheduled_task.v1", trigger: &pluginv1.ScheduledTaskTrigger{Type: "startup"}},
		// A newer SDK's trigger type loads on an older host, which uses its default.
		{name: "type this SDK doesn't know", typ: "scheduled_task.v1", trigger: &pluginv1.ScheduledTaskTrigger{Type: "cron"}},
		{name: "no type", typ: "scheduled_task.v1", trigger: &pluginv1.ScheduledTaskTrigger{IntervalMs: 300_000}, wantErr: "type is required"},
		{name: "interval too short", typ: "scheduled_task.v1", trigger: &pluginv1.ScheduledTaskTrigger{Type: "interval", IntervalMs: 5_000}, wantErr: "interval_ms"},
		{name: "daily without time", typ: "scheduled_task.v1", trigger: &pluginv1.ScheduledTaskTrigger{Type: "daily"}, wantErr: "time_of_day"},
		{name: "daily at 24:00", typ: "scheduled_task.v1", trigger: &pluginv1.ScheduledTaskTrigger{Type: "daily", TimeOfDay: "24:00"}, wantErr: "time_of_day"},
		{name: "weekly on day 7", typ: "scheduled_task.v1", trigger: &pluginv1.ScheduledTaskTrigger{Type: "weekly", TimeOfDay: "18:00", DayOfWeek: 7}, wantErr: "day_of_week"},
		{name: "descriptor on another type", typ: "event_consumer.v1", trigger: &pluginv1.ScheduledTaskTrigger{Type: "startup"}, wantErr: "requires type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := publicmanifest.Validate(&pluginv1.PluginManifest{PluginId: "silo.test", Version: "1.0.0",
				Capabilities: []*pluginv1.CapabilityDescriptor{{Type: tc.typ, Id: "task",
					ScheduledTask: &pluginv1.ScheduledTaskDescriptor{DefaultTrigger: tc.trigger}}}})
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
