package manifest_test

import (
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	publicmanifest "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

func TestLoadNotificationChannel(t *testing.T) {
	manifest, err := publicmanifest.Load([]byte(`{
	  "plugin_id":"silo.ntfy", "version":"1.0.0", "silo_api_version":"v1",
	  "capabilities":[{
	    "type":"notification_channel.v1", "id":"ntfy",
	    "notification_channel":{
	      "display_name":"ntfy",
	      "destination_schema":{
	        "key":"destination",
	        "json_schema":"{\"type\":\"object\",\"properties\":{\"topic\":{\"type\":\"string\"},\"token\":{\"type\":\"string\"}}}",
	        "admin_form":{"fields":[{"key":"topic","label":"Topic"},{"key":"token","label":"Access token","secret":true}]}
	      }
	    }
	  }]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	channel := manifest.GetCapabilities()[0].GetNotificationChannel()
	if channel.GetDisplayName() != "ntfy" || len(channel.GetDestinationSchema().GetAdminForm().GetFields()) != 2 {
		t.Fatalf("notification channel descriptor = %v", channel)
	}
}

func TestValidateNotificationChannel(t *testing.T) {
	schema := &pluginv1.ConfigSchema{Key: "destination", JsonSchema: `{"type":"object","properties":{"topic":{"type":"string"}}}`}
	for _, tc := range []struct {
		name       string
		capability *pluginv1.CapabilityDescriptor
		wantErr    string
	}{
		{name: "missing descriptor", capability: &pluginv1.CapabilityDescriptor{Type: "notification_channel.v1", Id: "ntfy"}, wantErr: "descriptor is required"},
		{name: "missing display name", capability: &pluginv1.CapabilityDescriptor{Type: "notification_channel.v1", Id: "ntfy",
			NotificationChannel: &pluginv1.NotificationChannelDescriptor{DestinationSchema: schema}}, wantErr: "display_name"},
		{name: "missing schema", capability: &pluginv1.CapabilityDescriptor{Type: "notification_channel.v1", Id: "ntfy",
			NotificationChannel: &pluginv1.NotificationChannelDescriptor{DisplayName: "ntfy"}}, wantErr: "destination_schema"},
		{name: "form field not in schema", capability: &pluginv1.CapabilityDescriptor{Type: "notification_channel.v1", Id: "ntfy",
			NotificationChannel: &pluginv1.NotificationChannelDescriptor{DisplayName: "ntfy", DestinationSchema: &pluginv1.ConfigSchema{
				Key: "destination", JsonSchema: schema.JsonSchema,
				AdminForm: &pluginv1.AdminFormDescriptor{Fields: []*pluginv1.AdminFormField{{Key: "server"}}},
			}}}, wantErr: "destination schema"},
		{name: "nested object field", capability: channelWithSchema(`{"type":"object","properties":{"auth":{"type":"object","properties":{"token":{"type":"string"}}}}}`), wantErr: "scalar"},
		{name: "array field", capability: channelWithSchema(`{"type":"object","properties":{"tags":{"type":"array"}}}`), wantErr: "type string, integer"},
		{name: "ref", capability: channelWithSchema(`{"type":"object","properties":{"topic":{"$ref":"#/$defs/topic"}}}`), wantErr: "plain scalars"},
		{name: "conditional", capability: channelWithSchema(`{"type":"object","properties":{"topic":{"type":"string"}},"allOf":[{"required":["topic"]}]}`), wantErr: "flat list"},
		{name: "required names no field", capability: channelWithSchema(`{"type":"object","properties":{"topic":{"type":"string"}},"required":["server"]}`), wantErr: "isn't a field"},
		{name: "id not a slug", capability: &pluginv1.CapabilityDescriptor{Type: "notification_channel.v1", Id: "Ntfy Push",
			NotificationChannel: &pluginv1.NotificationChannelDescriptor{DisplayName: "ntfy", DestinationSchema: schema}}, wantErr: "slug"},
		{name: "descriptor on another type", capability: &pluginv1.CapabilityDescriptor{Type: "scheduled_task.v1", Id: "task",
			NotificationChannel: &pluginv1.NotificationChannelDescriptor{DisplayName: "ntfy", DestinationSchema: schema}}, wantErr: "requires type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := publicmanifest.Validate(&pluginv1.PluginManifest{PluginId: "silo.test", Version: "1.0.0",
				Capabilities: []*pluginv1.CapabilityDescriptor{tc.capability}})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func channelWithSchema(jsonSchema string) *pluginv1.CapabilityDescriptor {
	return &pluginv1.CapabilityDescriptor{Type: "notification_channel.v1", Id: "ntfy",
		NotificationChannel: &pluginv1.NotificationChannelDescriptor{DisplayName: "ntfy",
			DestinationSchema: &pluginv1.ConfigSchema{Key: "destination", JsonSchema: jsonSchema}}}
}
