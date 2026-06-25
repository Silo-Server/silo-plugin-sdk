package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

func TestLoadAcceptsRequestRouterCapability(t *testing.T) {
	raw := []byte(`{
	  "plugin_id": "silo.example",
	  "version": "1.0.0",
	  "silo_api_version": "v1",
	  "capabilities": [
	    {"type": "request_router.v1", "id": "default", "display_name": "X", "description": "Y"}
	  ]
	}`)
	m, err := manifest.Load(raw)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if len(m.GetCapabilities()) != 1 {
		t.Fatalf("expected 1 capability, got %d", len(m.GetCapabilities()))
	}
	if got := m.GetCapabilities()[0].GetType(); got != "request_router.v1" {
		t.Fatalf("capability type = %q, want request_router.v1", got)
	}
}

func TestLoadAcceptsImageResolverCapability(t *testing.T) {
	raw := []byte(`{
	  "plugin_id": "silo.example",
	  "version": "1.0.0",
	  "silo_api_version": "v1",
	  "capabilities": [
	    {
	      "type": "image_resolver.v1",
	      "id": "tmdb",
	      "display_name": "TMDB Images",
	      "metadata": {
	        "schemes": ["tmdb"],
	        "priority": 100
	      }
	    }
	  ]
	}`)
	m, err := manifest.Load(raw)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if got := m.GetCapabilities()[0].GetType(); got != "image_resolver.v1" {
		t.Fatalf("capability type = %q, want image_resolver.v1", got)
	}
	if got := m.GetCapabilities()[0].GetMetadata().GetFields()["priority"].GetNumberValue(); got != 100 {
		t.Fatalf("capability metadata priority = %v, want 100", got)
	}
}

func TestLoadRejectsUnknownCapabilityType(t *testing.T) {
	raw := []byte(`{
	  "plugin_id": "silo.example",
	  "version": "1.0.0",
	  "silo_api_version": "v1",
	  "capabilities": [
	    {"type": "banana.v9", "id": "w"}
	  ]
	}`)
	if _, err := manifest.Load(raw); err == nil {
		t.Fatal("expected error for unknown capability type, got nil")
	}
}

func TestLoadFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	data := []byte(`{
  "plugin_id": "example.plugin",
  "version": "1.0.0",
  "capabilities": [
    {
      "type": "scheduled_task.v1",
      "id": "hello"
    }
  ]
}`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("os.WriteFile(%q) returned error: %v", path, err)
	}

	loaded, err := manifest.LoadFromDisk(path)
	if err != nil {
		t.Fatalf("LoadFromDisk(%q) returned error: %v", path, err)
	}

	if got := loaded.GetPluginId(); got != "example.plugin" {
		t.Fatalf("plugin_id = %q, want example.plugin", got)
	}
	if got := loaded.GetCapabilities()[0].GetId(); got != "hello" {
		t.Fatalf("capability id = %q, want hello", got)
	}
}
