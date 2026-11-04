package convert_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/convert"
)

// The host reads a task's default schedule from its stored capability
// record, so the descriptor must survive the round trip.
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
	decoded, err := convert.DecodeCapability(records[0])
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(decoded.GetScheduledTask(), descriptor) {
		t.Fatalf("decoded descriptor = %v, want %v", decoded.GetScheduledTask(), descriptor)
	}
}
