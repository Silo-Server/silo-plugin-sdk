// Package events is the catalog of host events an event_consumer.v1
// capability can subscribe to: their names, who their payloads are about, and
// the Go types their payloads decode into.
//
// The catalog is the public contract. A host delivers only the events listed
// here, and plugin events (names starting with PluginEventPrefix). Payloads
// change additively: a field is never renamed, removed, or given a new
// meaning, so decoding into the types in this package keeps working as hosts
// add fields. See docs/events.md.
package events

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"google.golang.org/protobuf/types/known/structpb"
)

// Access says who an event's payload is about, and so what an administrator
// approves before the host delivers it to a plugin.
type Access string

const (
	// AccessPublic events describe the catalog and libraries, or come from
	// plugins. They never name an account or profile.
	AccessPublic Access = "public"
	// AccessAdmin events describe server operations, such as playback
	// activity, scans and jobs. They can name accounts, profiles, devices and
	// network addresses, the way the admin activity views do.
	AccessAdmin Access = "admin"
	// AccessUserData events carry one profile's personal data, such as the
	// notifications Silo adds to its inbox.
	AccessUserData Access = "user_data"
)

// Valid reports whether a is one of the access classes this SDK knows.
func (a Access) Valid() bool {
	switch a {
	case AccessPublic, AccessAdmin, AccessUserData:
		return true
	}
	return false
}

// Event names in the catalog.
const (
	// CatalogLibraryChanged: a scan or refresh changed a library. Payload
	// LibraryChanged.
	CatalogLibraryChanged = "catalog.library.changed"
	// CatalogItemChanged: one catalog item's metadata, artwork or files
	// changed. Payload ItemChanged.
	CatalogItemChanged = "catalog.item.changed"

	// SessionStarted, SessionPaused, SessionResumed and SessionStopped follow
	// a playback session. Payload SessionEvent.
	SessionStarted = "session.started"
	SessionPaused  = "session.paused"
	SessionResumed = "session.resumed"
	SessionStopped = "session.stopped"

	// ScanStarted, ScanCompleted, ScanFailed and ScanCancelled follow a
	// library scan. Payload ScanEvent.
	ScanStarted   = "scan.started"
	ScanCompleted = "scan.completed"
	ScanFailed    = "scan.failed"
	ScanCancelled = "scan.cancelled"

	// JobCreated, JobCompleted, JobFailed and JobCancelled follow an admin
	// job, such as a refresh or an export. Payload JobEvent.
	JobCreated   = "job.created"
	JobCompleted = "job.completed"
	JobFailed    = "job.failed"
	JobCancelled = "job.cancelled"

	// NotificationCreated: Silo added a notification to a profile's inbox,
	// such as a new episode or a request update. Payload Notification.
	//
	// Provisional: a plugin that delivers notifications should provide a
	// notification_channel.v1 destination instead. This event may leave the
	// catalog before SDK 1.0.
	NotificationCreated = "notification.created"
)

// PluginEventPrefix starts the name of every event a plugin publishes. The
// host stamps it as plugin.<publisher plugin_id>.<name>, so a plugin cannot
// publish a host event. Plugin events are AccessPublic.
const PluginEventPrefix = "plugin."

// Definition describes one catalog event.
type Definition struct {
	Name        string
	Access      Access
	Description string
	newPayload  func() any
}

// NewPayload returns a pointer to a new value of the event's payload type,
// for json.Unmarshal or Decode.
func (d Definition) NewPayload() any { return d.newPayload() }

func define[T any](name string, access Access, description string) Definition {
	return Definition{Name: name, Access: access, Description: description, newPayload: func() any { return new(T) }}
}

var catalog = []Definition{
	define[LibraryChanged](CatalogLibraryChanged, AccessPublic, "A scan or refresh changed a library."),
	define[ItemChanged](CatalogItemChanged, AccessPublic, "A catalog item's metadata, artwork or files changed."),

	define[SessionEvent](SessionStarted, AccessAdmin, "A playback session started."),
	define[SessionEvent](SessionPaused, AccessAdmin, "A playback session paused."),
	define[SessionEvent](SessionResumed, AccessAdmin, "A paused playback session resumed."),
	define[SessionEvent](SessionStopped, AccessAdmin, "A playback session ended."),

	define[ScanEvent](ScanStarted, AccessAdmin, "A library scan started."),
	define[ScanEvent](ScanCompleted, AccessAdmin, "A library scan finished."),
	define[ScanEvent](ScanFailed, AccessAdmin, "A library scan failed."),
	define[ScanEvent](ScanCancelled, AccessAdmin, "A library scan was cancelled."),

	define[JobEvent](JobCreated, AccessAdmin, "An admin job was queued."),
	define[JobEvent](JobCompleted, AccessAdmin, "An admin job finished."),
	define[JobEvent](JobFailed, AccessAdmin, "An admin job failed."),
	define[JobEvent](JobCancelled, AccessAdmin, "An admin job was cancelled."),

	define[Notification](NotificationCreated, AccessUserData, "Silo added a notification to a profile's inbox."),
}

var byName = func() map[string]Definition {
	out := make(map[string]Definition, len(catalog))
	for _, d := range catalog {
		out[d.Name] = d
	}
	return out
}()

// Catalog returns every catalog event, in a stable order.
func Catalog() []Definition { return slices.Clone(catalog) }

// Lookup returns the catalog event named name. Plugin events aren't in the
// catalog; use AccessOf for them.
func Lookup(name string) (Definition, bool) {
	d, ok := byName[name]
	return d, ok
}

// IsPluginEvent reports whether name is an event a plugin published: the
// prefix, then a publisher plugin_id and an event name, both nonempty.
func IsPluginEvent(name string) bool {
	rest, ok := strings.CutPrefix(name, PluginEventPrefix)
	i := strings.LastIndexByte(rest, '.')
	return ok && i > 0 && i < len(rest)-1
}

// AccessOf returns the access class of a catalog or plugin event. It reports
// false for any other name: the host doesn't deliver it.
func AccessOf(name string) (Access, bool) {
	if d, ok := byName[name]; ok {
		return d.Access, true
	}
	if IsPluginEvent(name) {
		return AccessPublic, true
	}
	return "", false
}

// namePattern is the shape of an event name: dot-separated lowercase
// segments. Plugin IDs may contain hyphens, so segments may too.
var namePattern = regexp.MustCompile(`^[a-z0-9_-]+(?:\.[a-z0-9_-]+)+$`)

// ValidName reports whether name has the shape of an event name. It does
// not check the catalog; AccessOf does.
func ValidName(name string) bool {
	return len(name) <= 256 && namePattern.MatchString(name)
}

// Decode decodes an event payload, as HandleEventRequest carries it, into
// out, which is usually a pointer to one of this package's payload types.
// Payload integers fit in 53 bits, which a protobuf Value holds exactly.
func Decode(payload *structpb.Struct, out any) error {
	if payload == nil {
		return fmt.Errorf("events: payload is empty")
	}
	raw, err := json.Marshal(payload.AsMap())
	if err != nil {
		return fmt.Errorf("events: encode payload: %w", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("events: decode payload: %w", err)
	}
	return nil
}

// Project rewrites a catalog event's JSON payload into its public shape: the
// fields of the event's payload type, and nothing else. Hosts call it before
// delivering an event so internal fields don't reach plugins. Plugin events
// pass through unchanged. Any other name is an error.
func Project(name string, raw []byte) ([]byte, error) {
	if IsPluginEvent(name) {
		return raw, nil
	}
	d, ok := byName[name]
	if !ok {
		return nil, fmt.Errorf("events: %q is not a catalog event", name)
	}
	payload := d.NewPayload()
	if err := json.Unmarshal(raw, payload); err != nil {
		return nil, fmt.Errorf("events: decode %s payload: %w", name, err)
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("events: encode %s payload: %w", name, err)
	}
	return out, nil
}
