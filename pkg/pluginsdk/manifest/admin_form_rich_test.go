package manifest_test

import (
	"testing"

	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

func TestLoadAcceptsRichAdminForm(t *testing.T) {
	raw := []byte(`{
	  "plugin_id": "silo.example",
	  "version": "1.0.0",
	  "silo_api_version": "v1",
	  "capabilities": [{
	    "type": "request_router.v1", "id": "arr", "display_name": "X", "description": "Y",
	    "config_schema": [{
	      "key": "connection",
	      "title": "Connection",
	      "json_schema": "{\"type\":\"object\",\"properties\":{\"service_kind\":{\"type\":\"string\"},\"tags\":{\"type\":\"array\",\"items\":{\"type\":\"integer\"}},\"is_default\":{\"type\":\"boolean\"}}}",
	      "admin_form": {
	        "fields": [
	          {"key":"service_kind","label":"Service","control":"ADMIN_FORM_CONTROL_SELECT","options":[{"value":"radarr","label":"Radarr"}]},
	          {"key":"tags","label":"Tags","control":"ADMIN_FORM_CONTROL_MULTI_SELECT","dynamic_options":true},
	          {"key":"is_default","label":"Default","control":"ADMIN_FORM_CONTROL_SWITCH","show_when":[{"field":"service_kind","equals":["radarr"]}],"validation":{"max_length":0}}
	        ],
	        "sections": [{"key":"main","title":"Main","field_keys":["service_kind","tags","is_default"]}]
	      }
	    }]
	  }]
	}`)
	m, err := manifest.Load(raw)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	cap := m.GetCapabilities()[0]
	form := cap.GetConfigSchema()[0].GetAdminForm()
	if len(form.GetSections()) != 1 {
		t.Fatalf("expected 1 section, got %d", len(form.GetSections()))
	}
	if got := form.GetFields()[1].GetControl(); got.String() != "ADMIN_FORM_CONTROL_MULTI_SELECT" {
		t.Fatalf("tags control = %v, want MULTI_SELECT", got)
	}
	if !form.GetFields()[1].GetDynamicOptions() {
		t.Fatal("tags should declare dynamic_options")
	}
	if len(form.GetFields()[2].GetShowWhen()) != 1 {
		t.Fatal("is_default should carry a show_when condition")
	}
}
