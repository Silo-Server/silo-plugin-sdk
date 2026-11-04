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

Payloads change additively. A field is never renamed, removed, or given a new
meaning, and a host can add fields at any time, so ignore fields you don't
know. Vocabularies such as a job's `job_type` or a notification's `type` are
open: treat an unknown value as something you don't handle.

## Access

Each catalog event has an access class, which says whose data the payload
carries. A host shows the classes a plugin subscribes to when an administrator
installs it, and an upgrade that subscribes to a new class waits for an
administrator to approve it.

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
| `notification.created` | user_data | `Notification` | Silo added a notification to a profile's inbox. |

Notes on specific payloads:

- **`SessionEvent`** carries the live session (`session`, on started, paused
  and resumed) or the history entry the end recorded (`history`, on stopped).
  These are the `/api/v2` `AdminPlaybackSession` and
  `AdminPlaybackHistoryEntry` objects, with every field that schema defines.
  `DecodeSession` and `DecodeHistory` read the common ones. A session's
  events can arrive out of order; `seq` orders them.
- **`Notification`** identifies its recipient by `user_id` and `profile_id`
  together. Profile IDs repeat across accounts.
- IDs at the top level of a payload are JSON numbers (`library_id`,
  `user_id`) or strings (`content_id`, `profile_id`) as listed. Objects taken
  from `/api/v2` use that API's string IDs.

## Hosts

A host delivers only the events above and plugin events, after
`events.Project` rewrites each payload into its public shape. Adding an event
to the catalog, or a field to a payload type, is an additive SDK change.
