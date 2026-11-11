# Host events

An `event_consumer.v1` capability lists event names in its manifest
`subscriptions`, and the host calls `EventConsumer.HandleEvent` for each one
(see [runtime-host.md](runtime-host.md#receiving-events) for delivery
guarantees). This page lists the events a host delivers and their payloads.
The Go names and payload types are in
[`pkg/pluginsdk/events`](../pkg/pluginsdk/events).

A host delivers two kinds of events:

- **Catalog events**, listed below.
- **Plugin events**, named `plugin.<publisher plugin_id>.<name>`. The host
  stamps the prefix when a plugin calls `PublishEvent`, so no plugin can
  publish a catalog event.

The host delivers no other event name. A subscription to one is ignored.

## Decoding payloads

`HandleEventRequest.payload` is a JSON object. Decode it into the event's
payload type:

```go
import "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/events"

func (s *server) HandleEvent(ctx context.Context, req *pluginv1.HandleEventRequest) (*pluginv1.HandleEventResponse, error) {
	switch req.GetEventName() {
	case events.SessionStarted:
		var event events.SessionEvent
		if err := events.Decode(req.GetPayload(), &event); err != nil {
			return &pluginv1.HandleEventResponse{}, nil // malformed: ignore, don't retry
		}
		session, _ := event.DecodeSession()
		// ...
	}
	return &pluginv1.HandleEventResponse{}, nil
}
```

Integers in payloads, such as counts and season numbers, fit in 53 bits, the
range a JSON number and a `google.protobuf.Value` hold exactly. Identifiers
are always strings.

To check that every subscription names an event a host delivers, call
`manifest.SubscriptionProblems` from the plugin's tests.

Payloads change additively:

- A field is never renamed, removed, retyped or given a new meaning. New fields
  arrive only with an SDK release that adds them to the payload type, since a
  host projects every payload through `events.Project`. Ignore fields you
  don't know.
- A change that can't be made additively ships as a new event name; the old
  event keeps its shape.
- An event never moves to a different access class.
- A field the host can leave out is a pointer (or an `omitempty` string), so
  "absent" never reads as a zero ID, `0` or the zero time.

Vocabularies such as a job's `job_type` or a notification's `type` are open:
treat an unknown value as something you don't handle.

## Access

Each catalog event has an access class, which says whose data the payload
carries. A host may, in a later release, ask an administrator to approve the
`admin` and `user_data` classes before it delivers them, so subscribe only to
the events the plugin uses.

| Access | Carries |
|---|---|
| `public` | The catalog and libraries. Never names an account or profile. Plugin events are `public`. |
| `admin` | Server operations: playback activity, scans and jobs. Can name accounts, profiles, devices and network addresses, as the admin activity views do. |
| `user_data` | One profile's personal data, such as the notifications in its inbox. |

## Catalog

| Event | Access | Payload | When |
|---|---|---|---|
| `catalog.library.changed` | public | `LibraryChanged` | A scan or refresh changed a library. |
| `catalog.item.changed` | public | `ItemChanged` | A catalog item's metadata, artwork or files changed. |
| `session.started` | admin | `SessionEvent` | A playback session started. |
| `session.paused` | admin | `SessionEvent` | A playback session paused. |
| `session.resumed` | admin | `SessionEvent` | A paused session resumed. |
| `session.stopped` | admin | `SessionEvent` | A playback session ended. |
| `scan.started` | admin | `ScanEvent` | A library scan started. |
| `scan.completed` | admin | `ScanEvent` | A library scan finished. |
| `scan.failed` | admin | `ScanEvent` | A library scan failed. |
| `scan.cancelled` | admin | `ScanEvent` | A library scan was cancelled. |
| `job.created` | admin | `JobEvent` | An admin job was queued. |
| `job.completed` | admin | `JobEvent` | An admin job finished. |
| `job.failed` | admin | `JobEvent` | An admin job failed. |
| `job.cancelled` | admin | `JobEvent` | An admin job was cancelled. |
| `notification.created` | user_data | `Notification` | Silo added a notification to a profile's inbox. Provisional, see below. |

Notes on specific payloads:

- **`SessionEvent`** carries the live session (`session`, on started, paused
  and resumed) or the history entry the end recorded (`history`, on stopped).
  These are the `/api/v2` `AdminPlaybackSession` and
  `AdminPlaybackHistoryEntry` objects, with every field that schema defines.
  `DecodeSession` and `DecodeHistory` read the common ones. A session's
  events can arrive out of order; `seq` orders them.
- **`Notification`** identifies its recipient by `user_id` and `profile_id`
  together. Profile IDs repeat across accounts.
- **`notification.created` is provisional.** A plugin that sends a profile's
  notifications somewhere else (Pushover, ntfy and the like) should provide a
  `notification_channel.v1` destination, which gets the host's preferences,
  queue and retries. This event may leave the catalog before SDK 1.0.
- Every identifier is a JSON string, as in `/api/v2`: `library_id`,
  `user_id`, `created_by_user_id`, `content_id`, `profile_id`. Don't parse or
  compare them as numbers.

## Hosts

A host delivers only the events above and plugin events, after
`events.Project` rewrites each payload into its public shape. Adding an event
to the catalog, or a field to a payload type, is an additive SDK change.
