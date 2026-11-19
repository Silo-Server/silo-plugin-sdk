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
  `description`, `default` and `enum`, `minimum`/`maximum` on integer and
  number fields, and `format` and `minLength`/`maxLength`/`pattern` on string
  fields, plus a top-level `required` list. `additionalProperties` may only
  be `false` or absent. `$schema`, `$ref`, `allOf`/`anyOf`/`oneOf`/`not`,
  conditionals, nested objects and arrays are refused, because people who
  aren't administrators fill the form in on every client. Silo compiles the
  schema as JSON Schema draft 2020-12, the draft it uses for all plugin
  configuration.
- `admin_form` is required and is one flat list of at least one field: one
  form field per schema field, and every form field names a schema field.
  Clients render only the form. See [Destination form](#destination-form).
  `manifest.ValidateDestinationSchema` checks all of this.
- Silo checks each submitted setting against the schema before calling the
  plugin: only declared fields; the value has the declared type; `required`
  fields present when a destination is created (a stored secret left blank
  on an edit counts as present); `enum`; `minLength`, `maxLength` and
  `pattern` on strings; `minimum` and `maximum` on numbers. It doesn't check
  `format` (in draft 2020-12 it's only an annotation) or anything else, so
  check whatever else matters in `ValidateDestination`.
- `pattern` is Go RE2 syntax (Go's `regexp`: no lookahead, lookbehind or
  backreferences), and install refuses a pattern that doesn't compile.
  Clients may use it to check input as the user types; the host's check on
  save is the one that counts.
- Mark credentials `secret`: the host never returns them, and on an edit the
  user leaves them blank to keep the stored value.
- A manifest can declare several `notification_channel.v1` capabilities. One
  server answers for all of them; each request carries its `capability_id`.

## Destination form

A destination form is one flat list of fields. Every Silo client has to
render it for people who aren't administrators, so it allows only what a
flat list of scalar fields needs.

| Control | Schema field type |
|---|---|
| `ADMIN_FORM_CONTROL_TEXT` | `string` |
| `ADMIN_FORM_CONTROL_PASSWORD` | `string` |
| `ADMIN_FORM_CONTROL_NUMBER` | `integer` or `number` |
| `ADMIN_FORM_CONTROL_SWITCH` | `boolean` |
| `ADMIN_FORM_CONTROL_SELECT`, with static `options` | `string`, `integer` or `number` |

A field must set `key`, `label` and `control`, and may set `description`,
`placeholder`, `required`, `secret` (string fields only), `default_value`
and, on a `SELECT` only, `options`.

Every form must also be one a client can render and Silo can save, so
install refuses a form unless:

- each field's `key` is exactly its schema property's name: a letter
  followed by up to 63 letters, digits, underscores or hyphens, used once
- a field's `required` matches whether the schema's `required` list names
  it
- the schema compiles as Silo compiles it, and each `pattern` is valid Go
  RE2
- `minimum` is at most `maximum`, `minLength` is at most `maxLength`, and
  lengths aren't negative; an integer field's bounds hold at least one whole
  number
- an integer value (an option, a default, or what a user enters) is at most
  9007199254740991 in either direction, because settings travel as JSON and
  protobuf numbers. Use a string field for a larger identifier, such as a
  chat ID
- a `secret` field uses the `PASSWORD` or `TEXT` control
- every option value parses as the field's type (an integer field's options
  are integers), and no two options have the same value
- a field with `enum` uses a `SELECT`, and its options are exactly the
  `enum` values. Every option passes the field's other checks. Without
  `enum`, Silo accepts any value of the field's type that passes its
  checks, not only the options; declare `enum` to hold it to them.
- a `default` in the schema and a `default_value` on the form are each a
  value of the field's type that passes the field's checks and, on a
  `SELECT`, is one of the options

Refused:

- `sections` (the generic form doesn't need them: without sections it
  renders the fields in order) and `submit_label`
- `show_when`, `dynamic_options` and `exclusive_group_field`. A destination
  has no options lookup; the admin `ListConfigOptions` call is for
  administrators only.
- `validation`: put `minimum`, `maximum`, `minLength`, `maxLength` and
  `pattern` in the JSON schema, where Silo checks them
- `ADMIN_FORM_CONTROL_TEXTAREA`, `multiline`, `rows`,
  `ADMIN_FORM_CONTROL_MULTI_SELECT` and an unset control
- a `SELECT` without options, and `options` on any other control

Loosening these rules later is additive; tightening them would break
published plugins, so they start narrow.

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
service whether a user key exists, is fine. Silo gives the call 10 seconds,
counted once it has reached the plugin; starting a stopped plugin comes
first and isn't part of them. When the plugin doesn't answer in time,
returns a gRPC error, or isn't running, the save fails with `503` (the
plugin isn't available) and `Retry-After: 30`.

A valid answer doesn't mean the settings get saved: Silo checks them first
and saves afterwards, and the save can still fail, for example on the
per-profile destination limit or because another edit landed first. So
`ValidateDestination` must not create remote state, such as a subscription
or a webhook on the service, that would be a problem to leak.

### SendNotification

Silo calls it once per message and destination, possibly from different
servers, and again after a failure. A server claims a send for five minutes;
if that server dies, another one sends it again once the claim runs out, so
a message can arrive twice. `delivery_key` is the same on every attempt:
pass it to the service as an idempotency key where it has one.

The message arrives rendered: `title`, `body`, `url` (opens the item in
Silo; empty when the server has no public URL set) and `image_url` (a
poster, which can expire; empty when there is none). `details` carries the
same fields as the `Notification` type in `pkg/pluginsdk/events` (the
`notification.created` event's payload; identifiers are strings), for
plugins that render their own text. `type` is `episode.available`,
`watchlist.available`, a `request.*` type, or `test` for a test message,
whose `details` are empty. The vocabulary is open.

Map the service's answer onto an outcome:

| Outcome | Use for | Silo then |
|---|---|---|
| `DELIVERED` (or an empty response) | The service accepted the message. | Records the success. |
| `RETRY` | A timeout, a rate limit, a 5xx. Set `retry_after_seconds` when the service says how long to wait. | Tries again later with growing delays. After ten attempts over about a day the message is dropped, and the destination may be disabled (see below). `retry_after_seconds` makes the next wait at least that long, up to a host cap of six hours; it never shortens the schedule. |
| `FAILED` | This message can't be sent, but the destination works. | Drops the message. |
| `DESTINATION_INVALID` | The service definitely refused the settings, such as a revoked token or a deleted topic. | Drops the message and disables the destination at once, until its owner turns it back on. |

`DESTINATION_INVALID` takes effect on the first answer, so return it only when
the service's own response says the settings are wrong. A 401 from a proxy or
CDN, a timeout or a 5xx is `RETRY`.

Because of that cap, a plugin whose service asks for a longer wait, such as
a daily quota that resets in 20 hours, is called again after six hours. It
should then answer `RETRY` again with the wait that remains.

Silo allows 10 seconds to reach the plugin, starting it when needed, and
then 15 seconds for the call. A gRPC error your plugin returns, or no answer
within those 15 seconds, counts as `RETRY`: an ordinary failed attempt,
which retries and can lead to the destination being disabled under the "no
successful send" rule below. Only a failure to reach the plugin counts as
the plugin being unavailable (see below). Put a short reason in `message`;
the destination's owner sees it. Never include secrets.

Silo disables a destination in two cases only:

- the plugin answers `DESTINATION_INVALID`, or
- a message runs out of retries (ten attempts over about a day) after failed
  attempts, and the destination had no successful send in the meantime. One
  message that keeps failing while others get through doesn't disable it.

A message that is still waiting 36 hours after it was queued, for example
after several long `retry_after_seconds` waits, is dropped without another
send. That alone doesn't disable the destination.

A failure to reach the plugin is the plugin being unavailable, not the
destination failing: the plugin is off, removed or not running, the
connection fails or closes (its process exited during the call), or the
call ends with gRPC `Unavailable`. Silo retries such a message like any
other and drops it once it's older than the retry window, but leaves the
destination enabled, so it works again once the plugin is back. Silo also
doesn't disable a destination whose owner saved new settings while the
failing send ran.

A disabled destination is turned off and gets nothing until its owner
turns it back on. Saving new settings clears the reason shown and the
failure count but leaves the destination off.

### What a plugin isn't told

- **Destination changes.** Silo doesn't tell the plugin when a destination
  is edited, disabled or deleted. Every call carries the destination's
  current settings, so keep no per-destination state that would need
  cleaning up.
- **The recipient's language.** Requests carry no locale. Silo renders
  `title` and `body`; a plugin that renders its own text from `details`
  has no way to pick the recipient's language.

## Compatibility

A Silo server whose SDK predates `notification_channel.v1` refuses to
install or load a plugin that declares it: its manifest validation rejects
the whole plugin with `plugin capability "<id>": unknown type
"notification_channel.v1"`. The same happens to a plugin that declares any
capability type the host's SDK doesn't know, so put a notification channel
in its own plugin, not in one that must keep working on older servers.

## Network requests

The host's webhook client refuses private and loopback addresses. A plugin
makes its own requests, so it has to apply the same care: users who aren't
administrators fill in destination settings, so don't let a setting choose
the host a plugin connects to. Fix the service's URL in the plugin, or take
it from the plugin's administrator settings, as
[`examples/hello-notification-channel`](../examples/hello-notification-channel)
does.
