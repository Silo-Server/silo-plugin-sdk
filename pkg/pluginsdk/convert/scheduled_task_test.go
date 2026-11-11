package convert_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/convert"
)

// The host reads a task's default schedule from its stored capability
// record, so the descriptor must survive the round trip, including on a node
// whose SDK predates fields a newer one stored.
func TestScheduledTaskDescriptorRoundTrip(t *testing.T) {
	descriptor := &pluginv1.ScheduledTaskDescriptor{DefaultTrigger: &pluginv1.ScheduledTaskTrigger{
		Type: "weekly", TimeOfDay: "18:00", DayOfWeek: 6,
	}}
	records, err := convert.CapabilityRecordsFromManifest(&pluginv1.PluginManifest{Capabilities: []*pluginv1.CapabilityDescriptor{{
		Type: "scheduled_task.v1", Id: "digest", ScheduledTask: descriptor,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	stored := records[0].Metadata["scheduled_task"].(map[string]any)
	stored["future_field"] = true
	stored["default_trigger"].(map[string]any)["future_option"] = "x"
	decoded, err := convert.DecodeCapability(records[0])
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(decoded.GetScheduledTask(), descriptor) {
		t.Fatalf("decoded descriptor = %v, want %v", decoded.GetScheduledTask(), descriptor)
	}
}
