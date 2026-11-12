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

func TestLoadRequestRouterDeclaresOptionalCredentials(t *testing.T) {
	raw := []byte(`{
	  "plugin_id":"silo.requests-local", "version":"1.0.0", "silo_api_version":"v1",
	  "capabilities":[{
	    "type":"request_router.v1", "id":"local", "display_name":"Local",
	    "request_router":{"api_key_optional":true, "base_url_optional":true}
	  }]
	}`)
	manifest, err := publicmanifest.Load(raw)
	if err != nil {
		t.Fatal(err)
	}
	descriptor := manifest.GetCapabilities()[0].GetRequestRouter()
	if !descriptor.GetApiKeyOptional() {
		t.Fatal("api_key_optional = false, want true")
	}
	if !descriptor.GetBaseUrlOptional() {
		t.Fatal("base_url_optional = false, want true")
	}
}

// A manifest that leaves the flags out, or has no descriptor, needs an API key
// and a base URL, so routers written before the flags existed are unchanged.
func TestLoadRequestRouterWithoutCredentialFlagsRequiresBoth(t *testing.T) {
	for name, raw := range map[string]string{
		"other flags":   `"request_router":{"supports_seasons":true, "reports_download_progress":true}`,
		"no descriptor": `"display_name":"Arr"`,
	} {
		t.Run(name, func(t *testing.T) {
			manifest, err := publicmanifest.Load([]byte(`{
			  "plugin_id":"silo.requests-arr", "version":"1.0.0", "silo_api_version":"v1",
			  "capabilities":[{"type":"request_router.v1", "id":"arr", ` + raw + `}]
			}`))
			if err != nil {
				t.Fatal(err)
			}
			descriptor := manifest.GetCapabilities()[0].GetRequestRouter()
			if descriptor.GetApiKeyOptional() || descriptor.GetBaseUrlOptional() {
				t.Fatalf("descriptor = %v, want an API key and a base URL required", descriptor)
			}
		})
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
		"line separator":      {Queued: &pluginv1.RequestStatusWording{Detail: "Sent.\u2028Waiting."}},
		"paragraph separator": {Step: "Pro\u2029cessing"},
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

// A target's reported wording follows the manifest's limits.
func TestValidateRequestStatusWording(t *testing.T) {
	if err := publicmanifest.ValidateRequestStatusWording(&pluginv1.RequestStatusWording{Label: "Not out yet", Detail: "Radarr adds it once it's out."}); err != nil {
		t.Fatalf("fit wording: %v", err)
	}
	if err := publicmanifest.ValidateRequestStatusWording(nil); err != nil {
		t.Fatalf("absent wording: %v", err)
	}
	for _, wording := range []*pluginv1.RequestStatusWording{
		{Label: strings.Repeat("a", publicmanifest.MaxRequestWordingLabelRunes+1)},
		{Detail: "Out soon.\nMaybe."},
		{Detail: "Out soon.\u2028Maybe."},
		{Label: "Not\u2029out"},
	} {
		if err := publicmanifest.ValidateRequestStatusWording(wording); err == nil {
			t.Errorf("wording %v: want an error", wording)
		}
	}
}

// requesterOptionsManifest is a router whose form has a dynamic select, a
// dynamic select over an integer, a plain select, a chooser select and a switch, with the given
// requester_options.
func requesterOptionsManifest(options string) []byte {
	return []byte(`{
	  "plugin_id":"silo.requests-seerr", "version":"1.0.0", "silo_api_version":"v1",
	  "capabilities":[{
	    "type":"request_router.v1", "id":"seerr", "display_name":"Seerr",
	    "request_router":{"requester_options":[` + options + `]},
	    "config_schema":[{"key":"connection", "title":"Seerr", "json_schema":"{\"type\":\"object\",\"properties\":{\"profile\":{\"type\":\"string\"},\"mode\":{\"type\":\"string\"},\"chooser\":{\"type\":\"string\"},\"allow\":{\"type\":\"boolean\"},\"level\":{\"type\":\"integer\"},\"name\":{\"type\":\"string\"}}}", "admin_form":{"fields":[
	      {"key":"profile", "label":"Quality profile", "control":"ADMIN_FORM_CONTROL_SELECT", "dynamic_options":true},
	      {"key":"level", "label":"Level", "control":"ADMIN_FORM_CONTROL_SELECT", "dynamic_options":true},
	      {"key":"mode", "label":"Mode", "control":"ADMIN_FORM_CONTROL_SELECT", "options":[{"value":"a", "label":"A"}]},
	      {"key":"chooser", "label":"Chosen by", "control":"ADMIN_FORM_CONTROL_SELECT", "options":[{"value":"admin", "label":"Admin"}, {"value":"requester", "label":"Requester"}, {"value":"approver", "label":"Admin on approval"}]},
	      {"key":"allow", "label":"Let requesters choose", "control":"ADMIN_FORM_CONTROL_SWITCH"},
	      {"key":"name", "label":"Name", "control":"ADMIN_FORM_CONTROL_TEXT"}
	    ]}}]
	  }]
	}`)
}

func TestLoadRequestRouterDeclaresRequesterOptions(t *testing.T) {
	manifest, err := publicmanifest.Load(requesterOptionsManifest(`
	  {"field":"profile", "media_type":"movie", "label":"Quality", "chooser_field":"chooser"},
	  {"field":"profile", "media_type":"series"}`))
	if err != nil {
		t.Fatal(err)
	}
	options := manifest.GetCapabilities()[0].GetRequestRouter().GetRequesterOptions()
	if len(options) != 2 {
		t.Fatalf("requester_options = %v, want 2", options)
	}
	first := options[0]
	if first.GetField() != "profile" || first.GetMediaType() != "movie" || first.GetLabel() != "Quality" || first.GetChooserField() != "chooser" {
		t.Fatalf("first option = %v", first)
	}
	if options[1].GetLabel() != "" || options[1].GetChooserField() != "" {
		t.Fatalf("second option = %v, want no label and no chooser", options[1])
	}
}

func TestLoadRequestRouterRejectsBadRequesterOptions(t *testing.T) {
	for name, c := range map[string]struct{ options, want string }{
		"no field":             {`{"media_type":"movie"}`, "field is required"},
		"unknown field":        {`{"field":"folder"}`, "must be a select with dynamic_options"},
		"static select":        {`{"field":"mode"}`, "must be a select with dynamic_options"},
		"not a select":         {`{"field":"name"}`, "must be a select with dynamic_options"},
		"numeric select":       {`{"field":"level"}`, "must be a string property"},
		"unknown media type":   {`{"field":"profile", "media_type":"tv"}`, "media_type"},
		"listed twice":         {`{"field":"profile", "media_type":"movie"}, {"field":"profile", "media_type":"movie"}`, "more than once"},
		"all after one":        {`{"field":"profile", "media_type":"movie"}, {"field":"profile"}`, "more than once"},
		"one after all":        {`{"field":"profile"}, {"field":"profile", "media_type":"series"}`, "more than once"},
		"long label":           {`{"field":"profile", "label":"A label that is far too long to fit"}`, "exceeds 24 characters"},
		"padded label":         {`{"field":"profile", "label":" Quality"}`, "leading or trailing whitespace"},
		"unknown chooser":      {`{"field":"profile", "chooser_field":"nope"}`, "must be a select that lists its options"},
		"chooser is a switch":  {`{"field":"profile", "chooser_field":"allow"}`, "must be a select that lists its options"},
		"chooser is dynamic":   {`{"field":"profile", "chooser_field":"profile"}`, "must be a select that lists its options"},
		"chooser other values": {`{"field":"profile", "chooser_field":"mode"}`, `option "a" must be`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := publicmanifest.Load(requesterOptionsManifest(c.options))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Load error = %v, want one containing %q", err, c.want)
			}
		})
	}
}
