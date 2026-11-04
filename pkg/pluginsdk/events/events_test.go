package events_test

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/events"
)

func TestCatalogEntriesAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range events.Catalog() {
		if !events.ValidName(d.Name) || seen[d.Name] {
			t.Errorf("catalog name %q is malformed or repeated", d.Name)
		}
		seen[d.Name] = true
		if !d.Access.Valid() {
			t.Errorf("%s: access %q", d.Name, d.Access)
		}
		if err := json.Unmarshal([]byte(`{}`), d.NewPayload()); err != nil {
			t.Errorf("%s: payload type doesn't decode: %v", d.Name, err)
		}
	}
}

func TestAccessOf(t *testing.T) {
	for name, want := range map[string]events.Access{
		events.CatalogItemChanged:   events.AccessPublic,
		events.SessionStarted:       events.AccessAdmin,
		events.NotificationCreated:  events.AccessUserData,
		"plugin.silo.pushover.sent": events.AccessPublic,
	} {
		if got, ok := events.AccessOf(name); !ok || got != want {
			t.Errorf("AccessOf(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
	// Internal names and a bare prefix are not deliverable.
	for _, name := range []string{"user_state.changed", "scan_complete", "plugin."} {
		if _, ok := events.AccessOf(name); ok {
			t.Errorf("AccessOf(%q) reported a deliverable event", name)
		}
	}
}

func TestProjectKeepsOnlyPublicFields(t *testing.T) {
	raw := []byte(`{"id":"j1","job_type":"library_refresh","status":"failed",
		"request_payload":{"secret":"x"},"result_payload":{"internal":1},
		"requested_at":"2026-11-04T10:00:00Z","updated_at":"2026-11-04T10:01:00Z"}`)
	out, err := events.Project(events.JobFailed, raw)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if _, leaked := got["request_payload"]; leaked {
		t.Fatalf("internal field reached the public payload: %s", out)
	}
	if got["job_type"] != "library_refresh" || got["status"] != "failed" {
		t.Fatalf("public fields lost: %s", out)
	}
}

func TestProjectKeepsNestedAPIObjects(t *testing.T) {
	// session is an /api/v2 object governed by that contract: every field
	// passes through, not only the ones PlaybackSession reads.
	raw := []byte(`{"session_id":"s1","seq":3,"at":"2026-11-04T10:00:00.123Z","state":"playing",
		"session":{"session_id":"s1","user_id":"7","media_title":"Heat","routing_egress":"direct"}}`)
	out, err := events.Project(events.SessionStarted, raw)
	if err != nil {
		t.Fatal(err)
	}
	var event events.SessionEvent
	if err := json.Unmarshal(out, &event); err != nil {
		t.Fatal(err)
	}
	var session map[string]any
	if err := json.Unmarshal(event.Session, &session); err != nil || session["routing_egress"] != "direct" {
		t.Fatalf("nested session = %s, %v", event.Session, err)
	}
	typed, err := event.DecodeSession()
	if err != nil || typed.UserID != "7" || typed.MediaTitle != "Heat" {
		t.Fatalf("DecodeSession() = %+v, %v", typed, err)
	}
	if history, err := event.DecodeHistory(); history != nil || err != nil {
		t.Fatalf("DecodeHistory() = %+v, %v; want nil for a started event", history, err)
	}
}

func TestProjectRejectsNamesOutsideTheCatalog(t *testing.T) {
	if _, err := events.Project("user_state.changed", []byte(`{}`)); err == nil {
		t.Fatal("Project accepted an internal event")
	}
	raw := []byte(`{"anything":true}`)
	if out, err := events.Project("plugin.silo.test.ping", raw); err != nil || string(out) != string(raw) {
		t.Fatalf("plugin event changed: %s, %v", out, err)
	}
}

func TestDecode(t *testing.T) {
	payload, err := structpb.NewStruct(map[string]any{
		"id": "d1", "type": "episode.available", "user_id": 7, "profile_id": "p1",
		"season_number": 2, "reason_flags": map[string]any{"favorite": true},
		"created_at": "2026-11-04T10:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	var n events.Notification
	if err := events.Decode(payload, &n); err != nil {
		t.Fatal(err)
	}
	if n.UserID != 7 || n.ProfileID != "p1" || n.SeasonNumber == nil || *n.SeasonNumber != 2 || string(n.ReasonFlags) != `{"favorite":true}` {
		t.Fatalf("Decode() = %+v", n)
	}
	if err := events.Decode(nil, &n); err == nil {
		t.Fatal("Decode(nil) succeeded")
	}
}
