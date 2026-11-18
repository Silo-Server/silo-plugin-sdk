# Notification channels

A `notification_channel.v1` capability adds a kind of notification
destination to Silo, such as a push service or a chat app. Users add a
destination of that kind in their notification settings, next to webhooks and
email, and Silo sends it the notifications the profile receives: new
episodes, request updates, and titles from the profile's watchlist that
arrive in the library.

The plugin does two things: it checks a destination's settings, and it sends
one message. Silo does the rest:

- which notifications a profile gets, and each destination's two switches:
  `notify_next_up` (new episodes, type `episode.available`) and
  `notify_requests` (request updates, `request.*`, and watchlist arrivals,
  `watchlist.available`)
- a limit of 30 new-episode and watchlist messages a minute per profile
  (request updates aren't limited). Each API server counts on its own, so a
  cluster of several servers can send a profile more.
- a durable delivery queue, so a notification survives restarts and a server
  going away mid-send
- retries with backoff, and dropping a message after the last retry
- failure counts, shown to the destination's owner, and disabling a
  destination whose settings stopped working (see the outcomes below)
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
- `destination_schema` is a `ConfigSchema` with a flat form: one object of
  string, integer, number or boolean fields, each with optional `title`,
  `description`, `default`, `enum`, `format`, `minimum`/`maximum` and
  `minLength`/`maxLength`/`pattern`, plus a top-level `required` list.
  `additionalProperties` may only be `false` or absent. `$ref`,
  `allOf`/`anyOf`/`oneOf`/`not`, conditionals, nested objects and arrays are
  refused, because people who aren't administrators fill the form in on every
  client. Every schema field needs an `admin_form` field and every form field
  must name a schema field; clients render only the form.
  `manifest.ValidateDestinationSchema` checks all of this.
- Silo checks each submitted setting against the schema before calling the
  plugin: only declared fields; the value has the declared type; `required`
  fields present when a destination is created (a stored secret left blank
  on an edit counts as present); `enum`; `minLength`, `maxLength` and
  `pattern` on strings; `minimum` and `maximum` on numbers. It doesn't check
  `format` or anything else, so check whatever else matters in
  `ValidateDestination`.
- Mark credentials `secret`: the host never returns them, and on an edit the
  user leaves them blank to keep the stored value.
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
service whether a user key exists, is fine. Silo waits 10 seconds. When the
plugin doesn't answer in time, returns a gRPC error, or isn't running, the
save fails with `503` (the plugin isn't available) and `Retry-After: 30`.

### SendNotification

Silo calls it once per message and destination, possibly from different
servers, and again after a failure. A server claims a send for five minutes;
if that server dies, another one sends it again once the claim runs out, so
a message can arrive twice. `delivery_key` is the same on every attempt:
pass it to the service as an idempotency key where it has one.

The message arrives rendered: `title`, `body`, `url` (opens the item in
Silo) and `image_url` (a poster, which can expire). `details` carries the
same fields as the `Notification` type in `pkg/pluginsdk/events` (the
`notification.created` event's payload; identifiers are strings), for
plugins that render their own text. `type` is `episode.available`,
`watchlist.available`, a `request.*` type, or `test` for a test message,
whose `details` are empty. The vocabulary is open.

Map the service's answer onto an outcome:

| Outcome | Use for | Silo then |
|---|---|---|
| `DELIVERED` (or an empty response) | The service accepted the message. | Records the success. |
| `RETRY` | A timeout, a rate limit, a 5xx. Set `retry_after_seconds` when the service says how long to wait. | Tries again later with growing delays, for about a day, then drops the message and disables the destination (see below). `retry_after_seconds` makes the next wait at least that long, up to 6 hours; it never shortens the schedule. |
| `FAILED` | This message can't be sent, but the destination works. | Drops the message. |
| `DESTINATION_INVALID` | The service definitely refused the settings, such as a revoked token or a deleted topic. | Drops the message and disables the destination at once, until its owner edits it. |

`DESTINATION_INVALID` takes effect on the first answer, so return it only when
the service's own response says the settings are wrong. A 401 from a proxy or
CDN, a timeout or a 5xx is `RETRY`.

A gRPC error, or no answer within 15 seconds, counts as `RETRY`. Put a short
reason in `message`; the destination's owner sees it. Never include secrets.

Silo disables a destination in two cases only: the plugin answers
`DESTINATION_INVALID`, or sends keep failing until the retry schedule runs
out. When the last attempt failed because the plugin was off, removed or
unreachable, Silo drops the message but leaves the destination enabled, so
it works again once the plugin is back. Silo also doesn't disable a
destination whose owner saved new settings while the failing send ran. A
disabled destination gets nothing until its owner edits or re-enables it.

## Network requests

The host's webhook client refuses private and loopback addresses. A plugin
makes its own requests, so it has to apply the same care: users who aren't
administrators fill in destination settings, so don't let a setting choose
the host a plugin connects to. Fix the service's URL in the plugin, or take
it from the plugin's administrator settings, as
[`examples/hello-notification-channel`](../examples/hello-notification-channel)
does.
