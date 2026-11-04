package convert_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/convert"
)

// The host lists destination kinds from stored capability records, so the
// descriptor, its form included, must survive the round trip.
func TestNotificationChannelDescriptorRoundTrip(t *testing.T) {
	descriptor := &pluginv1.NotificationChannelDescriptor{
		DisplayName: "ntfy",
		Description: "Push to an ntfy topic.",
		DestinationSchema: &pluginv1.ConfigSchema{
			Key:        "destination",
			JsonSchema: `{"type":"object","properties":{"token":{"type":"string"}}}`,
			AdminForm: &pluginv1.AdminFormDescriptor{Fields: []*pluginv1.AdminFormField{{
				Key: "token", Label: "Access token", Secret: true,
			}}},
		},
	}
	records, err := convert.CapabilityRecordsFromManifest(&pluginv1.PluginManifest{Capabilities: []*pluginv1.CapabilityDescriptor{{
		Type: "notification_channel.v1", Id: "ntfy", NotificationChannel: descriptor,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := convert.DecodeCapability(records[0])
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(decoded.GetNotificationChannel(), descriptor) {
		t.Fatalf("decoded descriptor = %v, want %v", decoded.GetNotificationChannel(), descriptor)
	}
}
