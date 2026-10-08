package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	publicmanifest "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
)

const (
	scopeFile    = pluginv1.ScanSourceChangeScope_SCAN_SOURCE_CHANGE_SCOPE_FILE
	scopeSubtree = pluginv1.ScanSourceChangeScope_SCAN_SOURCE_CHANGE_SCOPE_SUBTREE
)

func poll(t *testing.T, changesFile, marker string) *pluginv1.PollChangesResponse {
	t.Helper()
	resp, err := scanSourceServer{}.PollChanges(context.Background(), &pluginv1.PollChangesRequest{
		CapabilityId: "changelog",
		Marker:       marker,
		SourceConfig: map[string]string{changesFileKey: changesFile},
	})
	if err != nil {
		t.Fatalf("PollChanges(marker=%q) error = %v", marker, err)
	}
	return resp
}

func writeLog(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// offset returns the marker for the end of the given complete lines.
func offset(lines ...string) string {
	n := 0
	for _, line := range lines {
		n += len(line) + 1
	}
	return strconv.Itoa(n)
}

func TestPollChangesFirstPollStartsFromNow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "changes.log")
	old := []string{"/media/movies/Old (2001)/Old.mkv", "/media/movies/Older (1999)/Older.mkv"}
	writeLog(t, path, old...)

	resp := poll(t, path, "")
	if len(resp.GetChanges()) != 0 {
		t.Fatalf("first poll replayed %d changes, want none", len(resp.GetChanges()))
	}
	if got, want := resp.GetNextMarker(), offset(old...); got != want {
		t.Fatalf("next_marker = %q, want %q", got, want)
	}
}

func TestPollChangesReturnsLinesSinceMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "changes.log")
	lines := []string{
		"/media/movies/Old (2001)/Old.mkv",
		"",
		"/media/movies/New (2024)/New.mkv",
		"/media/tv/Gone Show/",
	}
	writeLog(t, path, lines...)

	resp := poll(t, path, offset(lines[0]))
	want := []*pluginv1.ScanSourceChange{
		{SourcePath: "/media/movies/New (2024)/New.mkv", Scope: scopeFile},
		{SourcePath: "/media/tv/Gone Show", Scope: scopeSubtree},
	}
	got := resp.GetChanges()
	if len(got) != len(want) {
		t.Fatalf("got %d changes, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].GetSourcePath() != want[i].GetSourcePath() || got[i].GetScope() != want[i].GetScope() {
			t.Fatalf("change %d = {%q %v}, want {%q %v}", i,
				got[i].GetSourcePath(), got[i].GetScope(), want[i].GetSourcePath(), want[i].GetScope())
		}
	}
	end := offset(lines...)
	if got := resp.GetNextMarker(); got != end {
		t.Fatalf("next_marker = %q, want %q", got, end)
	}

	// Polling again with the returned marker yields nothing new and keeps the
	// position, so a held marker re-reads the same window.
	again := poll(t, path, resp.GetNextMarker())
	if len(again.GetChanges()) != 0 || again.GetNextMarker() != end {
		t.Fatalf("repeat poll = %d changes, marker %q; want 0 changes, marker %q",
			len(again.GetChanges()), again.GetNextMarker(), end)
	}
}

func TestPollChangesLeavesAnUnfinishedLineForTheNextPoll(t *testing.T) {
	path := filepath.Join(t.TempDir(), "changes.log")
	first := "/media/movies/A (2020)/A.mkv"
	if err := os.WriteFile(path, []byte(first+"\n/media/movies/B (2021)/B"), 0o600); err != nil {
		t.Fatal(err)
	}

	resp := poll(t, path, "0")
	if len(resp.GetChanges()) != 1 || resp.GetChanges()[0].GetSourcePath() != first {
		t.Fatalf("changes = %v, want only the complete line", resp.GetChanges())
	}
	if got, want := resp.GetNextMarker(), offset(first); got != want {
		t.Fatalf("next_marker = %q, want %q (before the unfinished line)", got, want)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(".mkv\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	next := poll(t, path, resp.GetNextMarker())
	if len(next.GetChanges()) != 1 || next.GetChanges()[0].GetSourcePath() != "/media/movies/B (2021)/B.mkv" {
		t.Fatalf("changes = %v, want the finished line", next.GetChanges())
	}
}

func TestPollChangesResyncsAfterForeignMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "changes.log")
	line := "/media/movies/A (2020)/A.mkv"
	writeLog(t, path, line)

	for _, marker := range []string{"-1", "not-a-number"} {
		resp := poll(t, path, marker)
		if len(resp.GetChanges()) != 0 {
			t.Fatalf("marker %q replayed %d changes, want none", marker, len(resp.GetChanges()))
		}
		if got, want := resp.GetNextMarker(), offset(line); got != want {
			t.Fatalf("marker %q: next_marker = %q, want %q", marker, got, want)
		}
	}
}

func TestPollChangesReadsATruncatedLogFromTheStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "changes.log")
	old := []string{"/media/movies/Old (2001)/Old.mkv", "/media/movies/Older (1999)/Older.mkv"}
	writeLog(t, path, old...)
	marker := poll(t, path, "").GetNextMarker()

	// Truncate the log, as the README suggests for rotation, then append one
	// line before the next poll.
	added := "/media/movies/New (2024)/New.mkv"
	writeLog(t, path, added)

	resp := poll(t, path, marker)
	if len(resp.GetChanges()) != 1 || resp.GetChanges()[0].GetSourcePath() != added {
		t.Fatalf("changes = %v, want only %q", resp.GetChanges(), added)
	}
	if got, want := resp.GetNextMarker(), offset(added); got != want {
		t.Fatalf("next_marker = %q, want %q", got, want)
	}
}

func TestPollChangesReadsATruncatedLogThatGrewPastTheMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "changes.log")
	writeLog(t, path, "/media/movies/Old (2001)/Old.mkv")
	marker := poll(t, path, "").GetNextMarker()

	// Truncate the log, then append more than the old marker's length before
	// the next poll, so the file is no shorter than it was.
	added := []string{"/media/movies/New Movie (2025)/New Movie (2025).mkv", "/media/tv/Show/S01E01.mkv"}
	writeLog(t, path, added...)

	resp := poll(t, path, marker)
	got := resp.GetChanges()
	if len(got) != len(added) {
		t.Fatalf("changes = %v, want %q", got, added)
	}
	for i := range added {
		if got[i].GetSourcePath() != added[i] {
			t.Fatalf("change %d = %q, want %q", i, got[i].GetSourcePath(), added[i])
		}
	}
	if got, want := resp.GetNextMarker(), offset(added...); got != want {
		t.Fatalf("next_marker = %q, want %q", got, want)
	}
}

func TestPollChangesMissingFileIsEmptyLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.log")
	for _, marker := range []string{"", "0"} {
		resp := poll(t, path, marker)
		if len(resp.GetChanges()) != 0 {
			t.Fatalf("marker %q: got %d changes, want none", marker, len(resp.GetChanges()))
		}
		if got := resp.GetNextMarker(); got != "0" {
			t.Fatalf("marker %q: next_marker = %q, want a non-empty %q", marker, got, "0")
		}
	}

	// Once the other tool writes, the next poll reports what it wrote.
	line := "/media/movies/A (2020)/A.mkv"
	writeLog(t, path, line)
	resp := poll(t, path, "0")
	if len(resp.GetChanges()) != 1 || resp.GetChanges()[0].GetSourcePath() != line {
		t.Fatalf("changes = %v, want only %q", resp.GetChanges(), line)
	}
}

func TestPollChangesFailsWhileALogItReadIsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "changes.log")
	lines := []string{"/media/movies/Old (2001)/Old.mkv", "/media/movies/Older (1999)/Older.mkv"}
	writeLog(t, path, lines...)
	marker := poll(t, path, "").GetNextMarker()

	if err := os.Rename(path, path+".away"); err != nil {
		t.Fatal(err)
	}
	_, err := scanSourceServer{}.PollChanges(context.Background(), &pluginv1.PollChangesRequest{
		Marker:       marker,
		SourceConfig: map[string]string{changesFileKey: path},
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("poll with the log missing = %v, want a not-found error", err)
	}

	// The host keeps the marker after an error. When the file returns, the
	// next poll must not replay it.
	if err := os.Rename(path+".away", path); err != nil {
		t.Fatal(err)
	}
	resp := poll(t, path, marker)
	if len(resp.GetChanges()) != 0 || resp.GetNextMarker() != marker {
		t.Fatalf("poll after the log returned = %d changes, marker %q; want 0 changes, marker %q",
			len(resp.GetChanges()), resp.GetNextMarker(), marker)
	}
}

func TestPollChangesRequiresChangesFile(t *testing.T) {
	_, err := scanSourceServer{}.PollChanges(context.Background(), &pluginv1.PollChangesRequest{})
	if err == nil || !strings.Contains(err.Error(), changesFileKey) {
		t.Fatalf("PollChanges without %s = %v, want an error naming the key", changesFileKey, err)
	}
}

func TestManifestDeclaresScanSource(t *testing.T) {
	manifest, err := publicmanifest.Load(manifestJSON)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	caps := manifest.GetCapabilities()
	if len(caps) != 1 || caps[0].GetType() != "scan_source.v1" {
		t.Fatalf("capabilities = %v, want one scan_source.v1", caps)
	}
	block, ok := caps[0].GetMetadata().AsMap()["scan_source"].(map[string]any)
	if !ok {
		t.Fatal("capability metadata has no scan_source descriptor")
	}
	form, _ := block["config_form"].(map[string]any)
	fields, _ := form["fields"].([]any)
	if len(fields) != 1 || fields[0].(map[string]any)["key"] != changesFileKey {
		t.Fatalf("config_form fields = %v, want one %q field", fields, changesFileKey)
	}
}
