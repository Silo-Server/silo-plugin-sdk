# RuntimeHost.v1

`RuntimeHost.v1` is the gRPC service the silo host exposes to plugins. It
inverts the usual capability flow: instead of the host calling into the
plugin, the plugin calls back into the host.

## Available RPCs (v1)

| RPC | Purpose |
|---|---|
| `PublishEvent(name, payload)` | Publish onto the host's event bus. The host stamps `plugin.<plugin_id>.` in front of `name` server-side. |
| `PublishEventTo(target_plugin_id, name, payload)` | Publish an event addressed to one installed plugin by stable `plugin_id`. |
| `PublishEventToInstallation(target_installation_id, name, payload)` | Publish an event addressed to one specific plugin installation. |
| `GetHostInfo()` | Return public-safe host URLs for callbacks and plugin-served links. |
| `ListLibraries(user_id)` | Return libraries (optionally scoped to a user). |
| `ValidateProfileCredential(username, password)` | Validate credentials through the host and return the resolved user/profile identity. |
| `CheckMediaPresence(provider, media_type, ids)` | Batched lookup: which external IDs already exist in the host catalog. v1 supports provider="tmdb" only. |
| `ListInstalledPlugins()` | Return installed plugins and their advertised capabilities. |
| `SetGlobalConfigEntry(key, value)` | Persist a plugin-owned global config entry for the calling plugin installation. |
| `ListLibraryMedia(filters)` | Return public-safe catalog media rows for browse/search workflows. |
| `GetCatalogStats(library_ids)` | Return aggregate public-safe catalog counts. |
| `ResolveCatalogImageURLs(paths, variant)` | Resolve stored catalog image paths into host-generated browser URL targets. |
| `MintScopedStream(request)` | Mint a short-lived stream grant for plugin-owned public access workflows. |
| `CallPluginHTTP(request)` | Invoke another installed plugin's HTTP route through the host control plane. |

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

### Profile credential validation

Protocol plugins sometimes receive only a username and password from a
third-party client instead of a browser bearer token. Use
`ValidateProfileCredential` to delegate that check to the host:

```go
host := sdkruntime.Host()
if host == nil { return }

credential, err := host.ValidateProfileCredential(ctx, "alex#kids", "password#1234")
if err != nil { return }

userID := credential.UserID
profileID := credential.ProfileID // empty means the user's primary profile
```

Do not validate or persist Silo account passwords inside a plugin. Treat the
password string as transient input and avoid logging it.

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

Catalog image fields are host-owned opaque paths. Pass poster or backdrop paths
back unchanged to resolve them into browser targets:

```go
urls, err := host.ResolveCatalogImageURLs(ctx, []string{item.PosterURL, item.BackdropURL}, "thumbnail")
if err != nil { return }
posterURL := urls[item.PosterURL]
```

Pass an empty variant to use the host default. Unresolved paths are omitted from
the returned map, which is keyed by the original input path. Treat returned URLs
as opaque host-generated browser targets.

### Peer discovery and calls

Use capability constants and discovery helpers instead of hard-coded strings
when finding another plugin:

```go
import (
    "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/capability"
    "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtimehost"
)

plugins, err := host.ListInstalledPluginsByCapability(ctx, capability.RequestRouter)
if err != nil || len(plugins) == 0 {
    return
}

var response struct {
    Accepted bool `json:"accepted"`
}
err = host.CallPluginJSON(ctx, runtimehost.CallPluginJSONRequest{
    InstallationID: int(plugins[0].GetInstallationId()),
    Path:           "/api/request",
    Request:        map[string]any{"title": "The Matrix"},
    Response:       &response,
})
```

`CallPluginJSON` sets JSON request/response headers, marshals the request
body, decodes the response into `Response`, and returns `*runtimehost.HTTPStatusError`
for HTTP status codes 400 and above. The lower-level `CallPluginHTTP` remains
available when a plugin needs custom content types or raw bytes.

### Addressing events

`PublishEvent` broadcasts to every matching `event_consumer.v1` subscription.
`PublishEventTo` restricts delivery to subscribers from a specific `plugin_id`.
`PublishEventToInstallation` is the most precise form: use it after
`ListInstalledPlugins` when multiple installations of the same plugin may
exist.

All plugin-published event names are still host-stamped as
`plugin.<caller_plugin_id>.<event_name>` so plugins cannot forge core host
events.

## End-to-end example

See `examples/hello-runtime-host/` for a minimal plugin that publishes an
event each time its scheduled task fires.

Future RuntimeHost additions should remain additive where possible so existing
plugins continue to run against newer hosts.
