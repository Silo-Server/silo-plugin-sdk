# RuntimeHost.v1

`RuntimeHost.v1` is the gRPC service the silo host exposes to plugins. It
inverts the usual capability flow: instead of the host calling into the
plugin, the plugin calls back into the host.

## Available RPCs (v1)

Silo implements these RPCs:

| RPC | Purpose |
|---|---|
| `PublishEvent(name, payload)` | Publish onto the host's event bus. The host stamps `plugin.<plugin_id>.` in front of `name` server-side. |
| `PublishEventTo(target_plugin_id, name, payload)` | Publish an event addressed to one installed plugin by stable `plugin_id`. |
| `GetHostInfo()` | Return public-safe host URLs for callbacks and plugin-served links, plus host role, listeners, and the ingress token for network access providers. |
| `ListLibraries(user_id)` | Return libraries (optionally scoped to a user). |
| `CheckMediaPresence(provider, media_type, ids)` | Batched lookup: which external IDs already exist in the host catalog. v1 supports provider="tmdb" only. |
| `ListInstalledPlugins()` | Return installed plugins and their advertised capabilities. |
| `SetGlobalConfigEntry(key, value)` | Persist a plugin-owned global config entry for the calling plugin installation. |
| `ReadInstanceState(key)` | Read one key of the calling instance's encrypted, host-scoped state. |
| `WriteInstanceState(key, value)` | Write one key of that state. Key ≤ 256 bytes, value ≤ 256 KiB, ≤ 256 keys per scope. |
| `ReportNetworkAccessStatus(status)` | Push a `network_access_provider.v1` status change so the host does not poll. Network access providers only; other plugins get `PermissionDenied`. |

### Declared but not implemented

The contract and the `runtimehost` client also declare these RPCs, but Silo
does not implement them yet:

| RPC | Purpose |
|---|---|
| `PublishEventToInstallation(target_installation_id, name, payload)` | Publish an event addressed to one specific plugin installation. |
| `ListLibraryMedia(filters)` | Return public-safe catalog media rows for browse/search workflows. |
| `GetCatalogStats(library_ids)` | Return aggregate public-safe catalog counts. |
| `ResolveCatalogImageURLs(paths, variant)` | Resolve stored catalog image paths into host-generated browser URL targets. |
| `MintScopedStream(request)` | Mint a short-lived stream grant for plugin-owned public access workflows. |
| `CallPluginHTTP(request)` | Invoke another installed plugin's HTTP route through the host control plane. `CallPluginJSON` is built on it. |

A call to one of them that reaches the host fails with the gRPC status
`codes.Unimplemented`. A plugin that uses one has to handle that error, for
example by skipping the feature that needs it:

```go
import (
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

stats, err := host.GetCatalogStats(ctx, nil)
switch {
case status.Code(err) == codes.Unimplemented:
    // This Silo host does not offer the call: skip the feature.
case err != nil:
    return err
default:
    useStats(stats)
}
```

## Using it from a plugin

The SDK exposes `sdkruntime.Host()` as a package-level function. It returns a
`*runtimehost.Client` once the host has bound the plugin's broker stream
(very early in plugin startup). It returns `nil` before binding and may also
return `nil` if the broker dial fails.

To get the broker bound automatically, embed `runtimedefault.Server` in your
plugin's `Runtime` server:

```go
import (
    "context"

    pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
    sdkruntime "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
    "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtimedefault"
)

type runtimeServer struct {
    runtimedefault.Server          // provides BindHostBroker for free
    manifest *pluginv1.PluginManifest
}

func (s *runtimeServer) GetManifest(_ context.Context, _ *pluginv1.GetManifestRequest) (*pluginv1.GetManifestResponse, error) {
    return &pluginv1.GetManifestResponse{Manifest: s.manifest}, nil
}
```

Then from any capability handler:

```go
func (s *myCapability) DoThing(ctx context.Context, ...) (..., error) {
    if host := sdkruntime.Host(); host != nil {
        _ = host.PublishEvent(ctx, "thing.happened", map[string]any{"id": "..."})
    }
    return ...
}
```

Treat a nil `sdkruntime.Host()` result as transient: skip the call, return a
temporary error, or defer work until a later handler invocation. Do not assume
`Configure()` guarantees availability.

### Client caching

The first successful `sdkruntime.Host()` call dials the broker stream and
caches one `*runtimehost.Client`; later calls reuse that client. It is safe for
high-frequency code paths to call `Host()` when needed, or to hold the returned
client if that fits the plugin's structure:

```go
type capability struct {
    host *runtimehost.Client // assigned after Host() returns non-nil
}

func (c *capability) DoThing(ctx context.Context, ...) {
    if c.host != nil {
        _ = c.host.PublishEvent(ctx, ...)
    }
}
```

### Library presence

When rendering a poster grid, batch-check which titles silo already has:

```go
host := sdkruntime.Host()
if host == nil { return }
presence, err := host.CheckMediaPresence(ctx, "tmdb", "movie", []string{"603", "550"})
if err != nil { return }
for _, id := range []string{"603", "550"} {
    if _, present := presence[id]; present {
        // render "✓ In Library" badge
    } else {
        // render "+ Request" button
    }
}
```

### Catalog image URLs

Silo does not implement `ResolveCatalogImageURLs` or `ListLibraryMedia` yet
(see [Declared but not implemented](#declared-but-not-implemented)). The
contract below describes how they are meant to work together.

Catalog image fields are host-owned opaque paths. Pass poster or backdrop paths
back unchanged to resolve them into browser targets:

```go
import "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/imagevariant"

urls, err := host.ResolveCatalogImageURLs(ctx, []string{item.PosterURL, item.BackdropURL}, imagevariant.Card)
if err != nil { return }
posterURL := urls[item.PosterURL]
```

Pass an empty variant to use the host default. Unresolved paths are omitted from
the returned map, which is keyed by the original input path. Treat returned URLs
as opaque host-generated browser targets.

### Image variants

The variant is a semantic size hint, not an exact pixel contract. The canonical
vocabulary, smallest to largest, is exported as constants from
`pkg/pluginsdk/imagevariant`:

| Constant | Value | Intended surface |
| --- | --- | --- |
| `imagevariant.Card` | `card` | grid and list thumbnails, search results, cast rows (~300px posters) |
| `imagevariant.Featured` | `featured` | hero artwork, detail pages, section headers (~500px posters/logos, ~1280px backdrops) |
| `imagevariant.Large` | `large` | high-density displays and client-selected larger sizes (~780px posters/stills, ~1280px logos/backdrops) |
| `imagevariant.Full` | `full` | full-screen and near-source rendering |
| `imagevariant.Original` | `original` | unresized source asset |

`large` was added after the initial vocabulary; the set is open and may grow
again. A plugin that resolves variants itself (for example a metadata provider
implementing `ResolveImageURL` / `ResolveImageURLs`) must treat an unknown
variant as a request for its nearest supported size — or its default — and must
never return an error. Implement the mapping as a `switch` with a `default` arm
so new variants fall through to a sensible size instead of failing a resolve,
which users see as a missing poster.

### Peer discovery and calls

Use capability constants and discovery helpers instead of hard-coded strings
when finding another plugin:

```go
import (
    "slices"

    "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/capability"
    "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtimehost"
)

// The host delivers the event as plugin.<your plugin_id>.title.requested.
subscription := "plugin." + myPluginID + ".title.requested"

plugins, err := host.ListInstalledPluginsByCapability(ctx, capability.EventConsumer)
if err != nil {
    return
}
// PublishEventTo reaches every installation of a plugin_id, so send once per ID.
sent := map[string]bool{}
for _, p := range plugins {
    consumer := runtimehost.Capability(p, capability.EventConsumer)
    if !p.GetEnabled() || !slices.Contains(consumer.GetSubscriptions(), subscription) || sent[p.GetPluginId()] {
        continue
    }
    sent[p.GetPluginId()] = true
    err = host.PublishEventTo(ctx, p.GetPluginId(), "title.requested", map[string]any{
        "title": "The Matrix",
    })
}
```

A peer receives the event only when its `event_consumer.v1` capability
subscribes to that exact name. The host drops an event nobody subscribes to
without returning an error. Events are one-way, so a reply has to come back as
another event.

`CallPluginJSON` and the lower-level `CallPluginHTTP` are meant for
request/response calls to a peer's `http_routes.v1` handler, but Silo does not
implement `CallPluginHTTP` yet, so both fail with `codes.Unimplemented`. When a
host implements it, `CallPluginJSON` sets JSON request/response headers,
marshals the request body, decodes the response into `Response`, and returns
`*runtimehost.HTTPStatusError` for HTTP status codes 400 and above.

### Addressing events

`PublishEvent` broadcasts to every matching `event_consumer.v1` subscription.
`PublishEventTo` restricts delivery to subscribers from a specific `plugin_id`.
`PublishEventToInstallation` is meant to address one specific installation,
but Silo does not implement it yet; use `PublishEventTo`, which reaches every
installation of the target `plugin_id`.

All plugin-published event names are still host-stamped as
`plugin.<caller_plugin_id>.<event_name>` so plugins cannot forge core host
events.

### Receiving events

An `event_consumer.v1` capability lists the event names it wants in its
manifest `subscriptions`, and the host calls `EventConsumer.HandleEvent` for
each matching event. Once Silo has stored an event, it delivers it to each
subscribing installation at least once, however many Silo servers run,
unless every attempt fails:

- One server delivers each attempt. A plugin installation runs on every API
  server, and any of them may make the call. Only that process sees the
  attempt, so a plugin cannot use events to keep state current in every
  process; keep such state where every process reads it.
- An error, or no answer within 10 seconds, asks for another attempt. The host
  retries a few times with growing delays over about 20 minutes, then drops
  the event.
  Return success once the event is handled, including when it is one the
  plugin chooses to ignore.
- A retry carries the same `event_id` as the first attempt, and every
  installation that receives the event sees the same `event_id`. A consumer
  that must not act twice, such as one that sends a notification, skips an
  `event_id` it has handled, or passes it upstream as an idempotency key. A
  retry can reach the plugin's process on another API server, so a record
  kept only in process memory misses some repeats. An empty `event_id` comes
  from an older host and identifies nothing, so never use it as a key.
- Events are not ordered. Two events can arrive in either order, and a retry
  can arrive after later events.
- An event can be lost before Silo stores it, for example when the server
  that produced it stops first.
- Hosts before `event_id` deliver each event once per API server without
  retrying, and leave `event_id` empty.

## End-to-end example

See `examples/hello-runtime-host/` for a minimal plugin that publishes an
event each time its scheduled task fires.

Future RuntimeHost additions should remain additive where possible so existing
plugins continue to run against newer hosts.
