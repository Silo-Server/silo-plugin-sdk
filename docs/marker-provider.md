# Marker providers (`marker_provider.v1`)

A marker provider supplies intro, credits, recap, and preview markers for
movies and episodes, and can accept markers the host submits. The host owns
local detection, provider order, the contribution settings, and when to fetch
or submit. The plugin talks to its marker service.

## Manifest

Declare the capability with free-form `metadata`. The host reads the keys
below from it; there is no typed descriptor.

```json
{
  "type": "marker_provider.v1",
  "id": "introdb",
  "display_name": "TheIntroDB",
  "metadata": {
    "required_external_ids": ["tmdb"],
    "default_fetch_priority": 100
  }
}
```

| Key | Type | Default | Meaning |
|---|---|---|---|
| `default_fetch_priority` | number | next free | Fetch priority the provider starts with when first configured. The host asks providers in ascending order. |
| `supports_contribution` | bool | true | `false` makes the provider fetch-only, and the host never calls `SubmitMarker`. |
| `required_external_ids` | list of strings | none | External ID keys (`tmdb`, `tvdb`, `imdb`) the title must have before the host submits to the provider. All of them are required. |
| `default_contribute_enabled` | bool | false | Since v0.29.0. Contribution starts on when the provider is first configured. |
| `default_contribute_auto_local` | bool | false | Since v0.29.0. Automatic sharing of local detections starts on. |
| `default_contribute_min_confidence` | number, 0–1 | 0.95 | Since v0.29.0. Starting minimum confidence for automatic sharing. |
| `supports_amend` | bool | false | Since v0.29.0. The provider replaces a server's earlier submission for the same file and kind, so the host may resubmit when the marker changes. |
| `supports_absent` | bool | false | Since v0.29.0. The provider accepts `absent` submissions. |

The defaults apply only when the host first creates the provider's
configuration, and an administrator's later changes win. A host may ignore
`default_contribute_enabled` and `default_contribute_auto_local` when its
administrator has chosen not to share by default. Hosts that predate a key
ignore it, so declaring one does not raise the plugin's minimum host version.

## Fetching

`FetchMarkersRequest` names the item by its title IDs (`external_ids`) and,
for an episode, by season and episode number, `episode_external_ids`,
`episode_title`, and `episode_numbering`. The comments in
[`marker_provider.proto`](../proto/silo/plugin/v1/marker_provider.proto) give
the exact rules.

Since v0.29.0 the request also carries:

- `file_hash`: the file's OpenSubtitles hash (16 lowercase hex characters),
  or empty when the host has none. It identifies exact bytes only.
- `file_size`: the file size in bytes, or 0 when unknown.
- `accepts_absent`: the host understands `MarkerSegment.absent`.

`file_hash` is the raw OpenSubtitles hash, and it identifies a file across
every server that holds the same bytes. A plugin must not send it off the
machine unhashed. Derive a service-specific value from it first, for example
a truncated SHA-256 of the hash.

A `MarkerSegment` with `absent` set asserts that the item has no segment of
that kind. Its `start_seconds` and `end_seconds` are unset, and `confidence`
applies to the absence. A provider must not return absent segments unless
`accepts_absent` is true: a host older than the field reads a segment without
bounds as covering the whole file.

## Submitting

`SubmitMarkerRequest` names the item the same way as `FetchMarkersRequest`
and carries one segment with its bounds. Since v0.29.0 it also carries:

- `file_hash` and `file_size`, as in `FetchMarkersRequest`, with the same
  rule about `file_hash`.
- `source`: `scanner` for a local detection, including chapters, or `manual`
  for a marker a person edited. Empty from older hosts.
- `algorithm`: the host's algorithm ID for the marker, such as
  `chromaprint:dialogue:v4`, or `manual:v1`.
- `confidence`: the host's confidence in the marker, 0–1.
- `absent`: the submission says the item has no segment of this kind, and
  the bounds are unset. Hosts send it only to providers that declare
  `supports_absent`.

## Compatibility

All the v0.29.0 fields are additive. A host older than v0.29.0 leaves them
empty: no file facts, `accepts_absent` false, and no `source`, `algorithm`,
`confidence`, or `absent` on submissions. Providers must handle an empty
`file_hash` and a `file_size` of 0, and treat a submission with an empty
`source` as one whose origin is unknown.
