package manifest_test

import (
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	publicmanifest "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

func TestLoadRequestRouterDeclaresSeasonSupport(t *testing.T) {
	raw := []byte(`{
	  "plugin_id":"silo.requests-arr", "version":"1.0.0", "silo_api_version":"v1",
	  "capabilities":[{
	    "type":"request_router.v1", "id":"arr", "display_name":"Arr",
	    "request_router":{"supports_seasons":true}
	  }]
	}`)
	manifest, err := publicmanifest.Load(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.GetCapabilities()[0].GetRequestRouter().GetSupportsSeasons() {
		t.Fatal("supports_seasons = false, want true")
	}
}

func TestLoadRequestRouterDeclaresDownloadProgress(t *testing.T) {
	raw := []byte(`{
	  "plugin_id":"silo.requests-arr", "version":"1.0.0", "silo_api_version":"v1",
	  "capabilities":[{
	    "type":"request_router.v1", "id":"arr", "display_name":"Arr",
	    "request_router":{"supports_seasons":true, "reports_download_progress":true}
	  }]
	}`)
	manifest, err := publicmanifest.Load(raw)
	if err != nil {
		t.Fatal(err)
	}
	descriptor := manifest.GetCapabilities()[0].GetRequestRouter()
	if !descriptor.GetReportsDownloadProgress() {
		t.Fatal("reports_download_progress = false, want true")
	}
	if !descriptor.GetSupportsSeasons() {
		t.Fatal("supports_seasons = false, want true")
	}
}

// A manifest written before reports_download_progress existed declares no
// progress reporting, so the host keeps its regular reconcile cadence.
func TestLoadRequestRouterWithoutProgressFlagReportsNoDownloadProgress(t *testing.T) {
	raw := []byte(`{
	  "plugin_id":"silo.requests-arr", "version":"1.0.0", "silo_api_version":"v1",
	  "capabilities":[{
	    "type":"request_router.v1", "id":"arr", "display_name":"Arr",
	    "request_router":{"supports_seasons":true}
	  }]
	}`)
	manifest, err := publicmanifest.Load(raw)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.GetCapabilities()[0].GetRequestRouter().GetReportsDownloadProgress() {
		t.Fatal("reports_download_progress = true, want false")
	}
}

// A manifest written before the descriptor existed stays valid and declares no
// season support, so the host keeps today's whole-series behaviour.
func TestLoadRequestRouterWithoutDescriptorDeclaresNoSeasonSupport(t *testing.T) {
	raw := []byte(`{
	  "plugin_id":"silo.requests-arr", "version":"1.0.0", "silo_api_version":"v1",
	  "capabilities":[{"type":"request_router.v1", "id":"arr", "display_name":"Arr"}]
	}`)
	manifest, err := publicmanifest.Load(raw)
	if err != nil {
		t.Fatal(err)
	}
	capability := manifest.GetCapabilities()[0]
	if capability.GetRequestRouter() != nil {
		t.Fatalf("request router descriptor = %#v, want absent", capability.GetRequestRouter())
	}
}

func TestValidateRequestRouterRejectsDescriptorOnOtherType(t *testing.T) {
	for _, descriptor := range []*pluginv1.RequestRouterDescriptor{
		{SupportsSeasons: true},
		{ReportsDownloadProgress: true},
	} {
		manifest := &pluginv1.PluginManifest{
			PluginId: "silo.invalid", Version: "1.0.0",
			Capabilities: []*pluginv1.CapabilityDescriptor{{
				Type: "scheduled_task.v1", Id: "task",
				RequestRouter: descriptor,
			}},
		}
		if err := publicmanifest.Validate(manifest); err == nil {
			t.Fatalf("expected request router descriptor %v on a scheduled task to fail", descriptor)
		}
	}
}

func TestLoadRequestRouterDeclaresWording(t *testing.T) {
	raw := []byte(`{
	  "plugin_id":"silo.requests-seerr", "version":"1.0.0", "silo_api_version":"v1",
	  "capabilities":[{
	    "type":"request_router.v1", "id":"seerr", "display_name":"Seerr",
	    "request_router":{"wording":{
	      "step":"Processing",
	      "queued":{"label":"Sent to Seerr","detail":"Seerr takes it from here."},
	      "downloading":{"label":"Processing"}
	    }}
	  }]
	}`)
	manifest, err := publicmanifest.Load(raw)
	if err != nil {
		t.Fatal(err)
	}
	wording := manifest.GetCapabilities()[0].GetRequestRouter().GetWording()
	if wording.GetStep() != "Processing" {
		t.Fatalf("step = %q, want Processing", wording.GetStep())
	}
	if wording.GetQueued().GetLabel() != "Sent to Seerr" || wording.GetQueued().GetDetail() != "Seerr takes it from here." {
		t.Fatalf("queued = %v, want the declared label and detail", wording.GetQueued())
	}
	if wording.GetDownloading().GetLabel() != "Processing" || wording.GetDownloading().GetDetail() != "" {
		t.Fatalf("downloading = %v, want only the declared label", wording.GetDownloading())
	}
}

func TestValidateRequestRouterWordingRejectsUnfitValues(t *testing.T) {
	long := strings.Repeat("a", publicmanifest.MaxRequestWordingLabelRunes+1)
	longDetail := strings.Repeat("a", publicmanifest.MaxRequestWordingDetailRunes+1)
	for name, wording := range map[string]*pluginv1.RequestRouterWording{
		"long step":           {Step: long},
		"long label":          {Queued: &pluginv1.RequestStatusWording{Label: long}},
		"long detail":         {Downloading: &pluginv1.RequestStatusWording{Detail: longDetail}},
		"two lines":           {Queued: &pluginv1.RequestStatusWording{Detail: "Sent.\nWaiting."}},
		"padded label":        {Downloading: &pluginv1.RequestStatusWording{Label: " Processing"}},
		"trailing space step": {Step: "Processing "},
	} {
		manifest := &pluginv1.PluginManifest{
			PluginId: "silo.requests-seerr", Version: "1.0.0",
			Capabilities: []*pluginv1.CapabilityDescriptor{{
				Type: "request_router.v1", Id: "seerr",
				RequestRouter: &pluginv1.RequestRouterDescriptor{Wording: wording},
			}},
		}
		if err := publicmanifest.Validate(manifest); err == nil {
			t.Errorf("%s: expected wording %v to fail validation", name, wording)
		}
	}
}

// Limits count characters, not bytes, so accented words fit.
func TestValidateRequestRouterWordingCountsCharacters(t *testing.T) {
	wording := &pluginv1.RequestRouterWording{Step: strings.Repeat("é", publicmanifest.MaxRequestWordingLabelRunes)}
	if err := publicmanifest.ValidateRequestRouterWording(wording); err != nil {
		t.Fatalf("ValidateRequestRouterWording() error = %v, want nil", err)
	}
}
