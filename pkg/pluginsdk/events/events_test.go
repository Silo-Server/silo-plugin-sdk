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
	// Internal names, a bare prefix, and a plugin name without both a
	// publisher and an event name are not deliverable.
	for _, name := range []string{"user_state.changed", "scan_complete", "plugin.", "plugin.foo", "plugin.foo."} {
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
	// A job the server queued itself has no creator: the field is absent,
	// not a zero ID.
	if _, present := got["created_by_user_id"]; present {
		t.Fatalf("absent creator encoded as a value: %s", out)
	}
}

func TestProjectKeepsOnlyCatalogFieldsOfNestedObjects(t *testing.T) {
	// A nested object is allowlisted like the payload itself: a field the
	// host's own schema has and the catalog type lacks doesn't reach plugins.
	raw := []byte(`{"session_id":"s1","seq":3,"at":"2026-11-04T10:00:00.123Z","state":"playing",
		"session":{"session_id":"s1","user_id":"7","media_title":"Heat","client_user_agent":"UA/1","routing_egress":"direct"}}`)
	out, err := events.Project(events.SessionStarted, raw)
	if err != nil {
		t.Fatal(err)
	}
	var event struct {
		Session map[string]any `json:"session"`
		History map[string]any `json:"history"`
	}
	if err := json.Unmarshal(out, &event); err != nil {
		t.Fatal(err)
	}
	if event.Session["user_id"] != "7" || event.Session["media_title"] != "Heat" {
		t.Fatalf("session = %v, want its catalog fields", event.Session)
	}
	for _, internal := range []string{"client_user_agent", "routing_egress"} {
		if _, leaked := event.Session[internal]; leaked {
			t.Errorf("session carries %q, which PlaybackSession doesn't declare", internal)
		}
	}
	if event.History != nil {
		t.Fatalf("history = %v, want none on a started event", event.History)
	}

	raw = []byte(`{"id":"d1","type":"request.declined","user_id":"7","profile_id":"p1","created_at":"2026-11-04T10:00:00Z",
		"reason_flags":{"request_id":"r1","tmdb_id":603,"media_type":"movie","reason":"Not available","internal_note":"x"}}`)
	out, err = events.Project(events.NotificationCreated, raw)
	if err != nil {
		t.Fatal(err)
	}
	var notification struct {
		ReasonFlags map[string]any `json:"reason_flags"`
	}
	if err := json.Unmarshal(out, &notification); err != nil {
		t.Fatal(err)
	}
	flags := notification.ReasonFlags
	if flags["request_id"] != "r1" || flags["tmdb_id"] != float64(603) || flags["reason"] != "Not available" {
		t.Fatalf("reason_flags = %v, want the request's catalog fields", flags)
	}
	if _, leaked := flags["internal_note"]; leaked {
		t.Errorf("reason_flags carries a field NotificationReason doesn't declare: %v", flags)
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
		"id": "d1", "type": "episode.available", "user_id": "7", "profile_id": "p1",
		"season_number": 2, "reason_flags": map[string]any{"next_up": true},
		"created_at": "2026-11-04T10:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	var n events.Notification
	if err := events.Decode(payload, &n); err != nil {
		t.Fatal(err)
	}
	if n.UserID != "7" || n.ProfileID != "p1" || n.SeasonNumber == nil || *n.SeasonNumber != 2 || n.ReasonFlags == nil || !n.ReasonFlags.NextUp {
		t.Fatalf("Decode() = %+v", n)
	}
	if err := events.Decode(nil, &n); err == nil {
		t.Fatal("Decode(nil) succeeded")
	}
}
