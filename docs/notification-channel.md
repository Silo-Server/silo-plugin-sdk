# Notification channels

A `notification_channel.v1` capability adds a kind of notification
destination to Silo, such as a push service or a chat app. Users add a
destination of that kind in their notification settings, next to webhooks and
email, and Silo sends it the notifications the profile receives: new
episodes, and request updates.

The plugin does two things: it checks a destination's settings, and it sends
one message. Silo does the rest:

- which notifications a profile gets, and the per-destination filters
  (favorites, watchlist, continue watching, next up, requests)
- a durable delivery queue, so a notification survives restarts and a server
  going away mid-send
- retries with backoff, and dropping a message after the last retry
- failure counts, shown to the destination's owner, and disabling a
  destination that keeps failing
- test messages from the settings page
- storing each destination's settings encrypted, and never returning secret
  fields through the API

Compare an `event_consumer.v1` plugin subscribed to `notification.created`,
which has to rebuild preferences, retries and deduplication itself.

## Manifest

```json
{
  "type": "notification_channel.v1",
  "id": "ntfy",
  "display_name": "ntfy",
  "notification_channel": {
    "display_name": "ntfy",
    "description": "Push notifications to an ntfy topic.",
    "destination_schema": {
      "key": "destination",
      "json_schema": "{\"type\":\"object\",\"properties\":{\"topic\":{\"type\":\"string\"},\"access_token\":{\"type\":\"string\"}},\"required\":[\"topic\"]}",
      "admin_form": {
        "fields": [
          { "key": "topic", "label": "Topic", "control": "ADMIN_FORM_CONTROL_TEXT", "required": true },
          { "key": "access_token", "label": "Access token", "control": "ADMIN_FORM_CONTROL_PASSWORD", "secret": true }
        ]
      }
    }
  }
}
```

- The capability `id` is a lowercase slug. It's stored with each destination,
  so keep it stable across releases.
- `destination_schema` is a `ConfigSchema`, validated like the plugin's own
  settings. Mark credentials `secret`: the host never returns them, and on an
  edit the user leaves them blank to keep the stored value.
- A manifest can declare several `notification_channel.v1` capabilities. One
  server answers for all of them; each request carries its `capability_id`.

## Serving

Register the service with `runtime.WithNotificationChannel`:

```go
sdkruntime.ServeManifestWithOptions(manifestJSON, version, sdkruntime.CapabilityServers{},
	sdkruntime.WithNotificationChannel(&channel{}),
)
```

`ServeManifestWithOptions` refuses to start when the manifest declares a
`notification_channel.v1` capability and no server is registered.

### ValidateDestination

Silo calls it before saving a destination. Check the settings and return
`valid: false` with a `message` the user can act on. Return a gRPC error only
when the check itself couldn't run. A network check, such as asking the
service whether a user key exists, is fine, but keep it under a few seconds.

### SendNotification

Silo calls it once per message and destination, possibly from different
servers, and again after a failure. `delivery_key` is the same on every
attempt: pass it to the service as an idempotency key where it has one.

The message arrives rendered: `title`, `body`, `url` (opens the item in
Silo) and `image_url` (a poster, which can expire). `details` carries the
notification as the events package's `Notification` type describes it, for
plugins that render their own text. `type` is `test` for a test message.

Map the service's answer onto an outcome:

| Outcome | Use for | Silo then |
|---|---|---|
| `DELIVERED` (or an empty response) | The service accepted the message. | Records the success. |
| `RETRY` | A timeout, a rate limit, a 5xx. Set `retry_after_seconds` when the service says how long to wait. | Tries again later with growing delays, for about a day, then drops the message and disables the destination. |
| `FAILED` | This message can't be sent, but the destination works. | Drops the message. |
| `DESTINATION_INVALID` | The settings no longer work, such as a revoked token. | Drops the message and disables the destination until its owner edits it. |

A gRPC error, or no answer within 15 seconds, counts as `RETRY`. Put a short
reason in `message`; the destination's owner sees it. Never include secrets.

## Network requests

The host's webhook client refuses private and loopback addresses. A plugin
makes its own requests, so it has to apply the same care: users who aren't
administrators fill in destination settings, so don't let a setting choose
the host a plugin connects to. Fix the service's URL in the plugin, or take
it from the plugin's administrator settings, as
[`examples/hello-notification-channel`](../examples/hello-notification-channel)
does.
