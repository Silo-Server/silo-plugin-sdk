package manifest_test

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

// manifestWith returns a minimal valid manifest with extra inserted as
// top-level JSON members (each followed by a comma).
func manifestWith(extra string) []byte {
	return []byte(`{
	  "plugin_id": "silo.example",
	  "version": "1.0.0",
	  ` + extra + `
	  "capabilities": [{"type": "scheduled_task.v1", "id": "example"}]
	}`)
}

func TestLoadReadsStayRunning(t *testing.T) {
	for name, tc := range map[string]struct {
		extra string
		want  bool
	}{
		"absent": {extra: "", want: false},
		"false":  {extra: `"stay_running": false,`, want: false},
		"true":   {extra: `"stay_running": true,`, want: true},
		// The runtime manifest subcommand renders JSON names.
		"json name": {extra: `"stayRunning": true,`, want: true},
	} {
		t.Run(name, func(t *testing.T) {
			loaded, err := manifest.Load(manifestWith(tc.extra))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got := loaded.GetStayRunning(); got != tc.want {
				t.Fatalf("GetStayRunning() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLoadRejectsNonBooleanStayRunning(t *testing.T) {
	for name, value := range map[string]string{
		"string": `"true"`,
		"number": `1`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := manifest.Load(manifestWith(`"stay_running": ` + value + `,`)); err == nil {
				t.Fatal("Load() error = nil, want a decode error")
			}
		})
	}
}

func TestStayRunningSurvivesManifestRender(t *testing.T) {
	loaded := manifest.MustLoad(manifestWith(`"stay_running": true,`))
	// Same options as the runtime package's manifest subcommand.
	rendered, err := protojson.MarshalOptions{Indent: "  "}.Marshal(loaded)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	reloaded, err := manifest.Load(rendered)
	if err != nil {
		t.Fatalf("Load(rendered) error = %v", err)
	}
	if !reloaded.GetStayRunning() {
		t.Fatalf("stay_running lost in render:\n%s", rendered)
	}
}
