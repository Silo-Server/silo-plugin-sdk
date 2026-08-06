package convert_test

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/convert"
)

func TestCapabilityRecordsFromManifestRoundTrips(t *testing.T) {
	metadata, err := structpb.NewStruct(map[string]any{
		"provider": "example",
		"priority": float64(5),
	})
	if err != nil {
		t.Fatalf("structpb.NewStruct() returned error: %v", err)
	}

	manifest := &pluginv1.PluginManifest{
		Capabilities: []*pluginv1.CapabilityDescriptor{
			{
				Type:        "metadata_provider.v1",
				Id:          "example",
				DisplayName: "Example",
				Description: "Example provider",
				Subscriptions: []string{
					"catalog.updated",
				},
				ConfigSchema: []*pluginv1.ConfigSchema{
					{
						Key:         "connection",
						Title:       "Connection",
						Description: "API key",
						JsonSchema:  `{"type":"object"}`,
						Required:    false,
						AdminForm: &pluginv1.AdminFormDescriptor{
							SubmitLabel: "Connect",
							Fields: []*pluginv1.AdminFormField{{
								Key: "base_url", Label: "Server URL",
								Control:  pluginv1.AdminFormControl_ADMIN_FORM_CONTROL_TEXT,
								Required: true,
							}},
						},
					},
				},
				Metadata: metadata,
			},
		},
	}

	records, err := convert.CapabilityRecordsFromManifest(manifest)
	if err != nil {
		t.Fatalf("CapabilityRecordsFromManifest() returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	recordedSchema := records[0].Metadata["config_schema"].([]map[string]any)[0]
	for key, want := range map[string]any{
		"key":         "connection",
		"title":       "Connection",
		"description": "API key",
		"json_schema": `{"type":"object"}`,
		"required":    false,
	} {
		if got, present := recordedSchema[key]; !present || got != want {
			t.Fatalf("config_schema[%q] = %#v, present=%v, want %#v", key, got, present, want)
		}
	}
	recordedSchema["future_field"] = true

	decoded, err := convert.DecodeCapability(records[0])
	if err != nil {
		t.Fatalf("DecodeCapability() returned error: %v", err)
	}
	if got := decoded.GetDisplayName(); got != "Example" {
		t.Fatalf("display_name = %q, want Example", got)
	}
	if got := decoded.GetConfigSchema()[0].GetKey(); got != "connection" {
		t.Fatalf("config_schema key = %q, want connection", got)
	}
	if !proto.Equal(decoded.GetConfigSchema()[0], manifest.GetCapabilities()[0].GetConfigSchema()[0]) {
		t.Fatalf("config_schema round trip = %#v, want %#v", decoded.GetConfigSchema()[0], manifest.GetCapabilities()[0].GetConfigSchema()[0])
	}
	if got := decoded.GetMetadata().AsMap()["provider"]; got != "example" {
		t.Fatalf("metadata provider = %#v, want example", got)
	}
}
