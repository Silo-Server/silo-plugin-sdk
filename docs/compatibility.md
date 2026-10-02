# Compatibility and Versioning

## Scope

`silo-plugin-sdk` is the public build-time contract for Go plugin authors.

This repository is released as a semver-governed Go module. Third-party plugins and first-party consumers should depend on tagged releases, not on sibling repo checkouts or workspace-only overrides.

The compatibility boundary includes:

- protobuf messages and gRPC services under `pkg/pluginproto/silo/plugin/v1`
- runtime bootstrap behavior in `pkg/pluginsdk/runtime`
- manifest helpers in `pkg/pluginsdk/manifest`
- config validation helpers in `pkg/pluginsdk/config`
- generic capability metadata conversion helpers in `pkg/pluginsdk/convert`
- canonical image-variant strings in `pkg/pluginsdk/imagevariant`

## Versioning Rules

- Treat the SDK as a semver boundary.
- Publish semver tags from this repository and consume those tags from downstream repos.
- Prefer additive protobuf evolution.
- Avoid renaming or removing protobuf fields, services, or enum values in `v1`.
- Keep plugin capability expansion additive: new functionality should arrive as new capability families or additive fields, not breaking rewrites of existing ones.
- First-party consumers should not merge code that depends on new SDK packages or symbols until the required SDK tag exists.

## Consumer Rules

- `Silo`, `silo-plugin-tvdb`, and `silo-plugin-tmdb` should pin released SDK tags in `go.mod`.
- CI and release pipelines should build with `GOWORK=off` and without checking out this repo as a sibling source dependency.
- Local `go.work` files and temporary `replace` directives are acceptable for development, but they must not be committed as the release path.

## Open Vocabularies

Some contract fields carry an open string vocabulary rather than an enum, so
Silo can add values without a breaking protobuf change. Plugins must tolerate
values they do not recognize.

Image variants (`ResolveImageURLRequest.variant`,
`ResolveImageURLsRequest.variant`, `ResolveCatalogImageURLsRequest.variant`) are
the current example. The canonical values are exported from
`pkg/pluginsdk/imagevariant`: `card`, `featured`, `large`, `full`, `original`,
listed smallest to largest. `large` (~780px posters and stills, ~1280px logos
and backdrops) was added between `featured` and `full` once Silo gained
client-selectable image sizes; adding it is an additive change and does not
require a plugin update.

A plugin receiving an unknown variant MUST degrade gracefully to its nearest
supported size — or its default size — and MUST NOT return an error. Returning
an error turns a slightly-wrong image size into a missing image. Use a `switch`
with a `default` arm rather than an exhaustive match, and do not assume the
constants shipped in any given SDK tag are the complete set. The same rule
applies to any future open vocabulary added to `v1`.

`DownloadProgress.phase` runs the other way: plugins send it and the host reads
it. The current values are `queued`, `downloading`, `paused`, `stalled`,
`importing`, and `import_blocked`. Hosts treat a value they do not recognize as
`downloading`, so a phase added later degrades to a plain progress display
rather than an error. An empty `phase` is not a new value: plugins must always
set one, as the `TargetStatus.progress` rules below describe.

`CapabilityDescriptor.auth_modes` is open too. The known values are
`password`, `oauth2` and, since v0.23.0, `network`. Hosts ignore modes they do
not recognize, and `manifest.Validate` accepts any non-empty, non-duplicate
mode that is not a case variant of a known one, so a plugin built against a
newer SDK still loads on a host that validates with an older one. A plugin that
lists a newer mode should also list a mode older hosts understand. `network` is
the exception: it must stand alone, so a host that predates it treats the
capability as a password provider, and the plugin refuses every password.

`CheckAccountResponse.status` is an enum that plugins send and hosts read.
Hosts treat `CHECK_ACCOUNT_STATUS_UNSPECIFIED`, a value they do not recognize,
and a gRPC error other than `Unimplemented` like
`CHECK_ACCOUNT_STATUS_UNAVAILABLE`: log, keep the account as it is, and retry
later. None of them counts as `ACTIVE`, so an empty response or a status added
later never keeps an account alive by accident. An `AuthManagedRole` value the
host does not recognize leaves the account's role unchanged. A refused refresh
token (OIDC `invalid_grant`) is `NOT_PERMITTED` only when the token is known to
be inside its lifetime, because the host revokes every session and deletes the
account's API keys on `NOT_PERMITTED`. A refused token that may have expired is
`UNSUPPORTED`.

## Additive Services

Adding an RPC to a released service adds a method to its Go server interface,
which breaks plugins that implement the interface directly rather than
embedding the generated `Unimplemented...Server`. New RPCs for a released
capability therefore go in a separate service, such as
`WatchSyncDeviceAuthorizationService`, `AuthProviderChecks` or
`NetworkIdentityAuth`. A plugin that
does not register the new service answers `Unimplemented`, and hosts treat that
as "not supported".

## Presence-Sensitive Optional Fields

Some contract fields use proto3 `optional` because absence and zero have
different meanings. Current examples are `WatchSyncEvent.list_position`,
`GetImagesRequest.season_number`, and `ImageRecord.season_number`.

Consumers must check presence (`nil` pointer in Go or `HasField` in reflective
APIs), never infer it from the numeric value. Calling
`GetSeasonNumber() != 0` silently conflates "no season scope" with "Specials
(season zero) requested."

`AuthenticateResponse.email_verified` (added in v0.22.0) is `optional bool`:
unset means the provider did not say, and an explicit `false` means the
provider reports the address unverified. Check presence before trusting an
email for account matching.

`AuthenticateResponse.refresh_state` is a message field. Unset keeps the state
the host stored for the account; an explicitly empty Struct clears it. The
same rule applies to `CheckAccountResponse.account.refresh_state`.

Watch-sync rating fields (`WatchSyncEvent.rating` and
`WatchSyncRemoteRatingState.rating`) are deliberately plain `int32`. Valid
ratings run from 1 to 10, so zero never carries a rating and no presence check
is needed.

`RequestDescriptor.seasons` is a repeated field, so it has no presence: an
empty list means the whole series, and season `0` in a non-empty list means
Specials. A plugin built before the field existed decodes it as an unknown
field and fulfils the whole series. Because the plugin cannot say so on the
wire, the host decides who may receive a season-only request from the
manifest's `RequestRouterDescriptor.supports_seasons` flag instead. An absent
descriptor means the flag is false.

`TargetStatus.progress` is a message field, so its presence is the `nil` check.
Unset means the target has nothing in flight, and the host clears the progress
it stored for it. A set `progress` must carry a `phase`: the host counts one
with an empty `phase` and a `bytes_total` of 0 as unset. With a `phase`, a
`bytes_total` of 0 means at least one of the target's distinct downloads has no
known size yet. `bytes_left` is then 0 too, so the host shows no percentage
rather than an overstated one.
`estimated_completion` is unset when no download has an estimate. Plugins
built before the field existed never set it, so the host refreshes progress
every minute only when the manifest declares
`RequestRouterDescriptor.reports_download_progress`, and then only for a
`downloading` target whose last status carried `progress`. The host ignores
`progress` from a plugin that does not declare the flag. The regular reconcile
pass notices progress first, and a target
whose `progress` comes back unset returns to that cadence.

`RequestRouterDescriptor.wording` is a message field read from the manifest.
Hosts built before it existed ignore it: every SDK version drops unknown fields
when it loads a manifest, and from v0.13.1 also when it decodes stored
capability metadata, so only a host older than v0.13.1 that reads metadata a
newer host stored could reject it. An absent wording, or any empty value in it,
means the host's own neutral words. Plugins can therefore declare wording
without raising their minimum host version.

A season-scoped `GetImagesRequest` is a scope, not a guarantee. Plugins that
can filter by season should do so, and plugins should populate
`ImageRecord.season_number` whenever the season is known. Hosts must bucket and
verify images by the per-image field rather than assume a filtered response.

## Runtime Compatibility

- `silo_api_version` is the coarse runtime compatibility gate between Silo and a plugin binary.
- Host installs should reject incompatible API versions before runtime startup.
- A plugin binary should return the same manifest shape that Silo installs, except that binaries may compute their checksum dynamically at runtime.
- From this version, `convert.DecodeCapability` ignores fields and enum values it does not know, so a server node built on this SDK or later, in a mixed-version cluster sharing one database, still loads capability metadata written by a newer SDK; it only loses the parts added after its own SDK version. Nodes built on an earlier SDK decode strictly and reject such metadata, so upgrade every node past them before installing plugins that publish newer fields.

## Go Support

The supported public authoring path today is Go-only.

The protobuf and gRPC contracts are the long-term compatibility source of truth, but non-Go authoring is not an official support target in this release.

## Self-Describing Binary Guidance

If a plugin should be installable by direct binary upload:

- embed a manifest template in the binary
- compute the executable checksum at runtime
- return that populated manifest from `Runtime.GetManifest`

This keeps the plugin installable without requiring external repo state at upload time.
