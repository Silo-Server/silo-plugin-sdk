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
| `default_contribute_enabled` | bool | false | Contribution starts on when the provider is first configured. |
| `default_contribute_auto_local` | bool | false | Automatic sharing of local detections starts on. |
| `default_contribute_min_confidence` | number, 0–1 | 0.95 | Starting minimum confidence for automatic sharing. |
| `supports_amend` | bool | false | The provider replaces a server's earlier submission for the same file and kind, so the host may resubmit when the marker changes. |
| `supports_absent` | bool | false | The provider accepts `absent` submissions. |

The defaults apply only when the host first creates the provider's
configuration, and an administrator's later changes win. A host may ignore
`default_contribute_enabled` and `default_contribute_auto_local` when its
administrator has chosen not to share by default. Hosts that predate a key
ignore it, so declaring one does not raise the plugin's minimum host version.
The SDK does not read these keys, so declaring one needs no particular SDK
version. Only hosts that implement a key honour it.

## Fetching

`FetchMarkersRequest` names the item by its title IDs (`external_ids`) and,
for an episode, by season and episode number, `episode_external_ids`,
`episode_title`, and `episode_numbering`. The comments in
[`marker_provider.proto`](../proto/silo/plugin/v1/marker_provider.proto) give
the exact rules.

Since v0.30.0 the request also carries:

- `file_hash`: the file's OpenSubtitles hash (16 lowercase hex characters),
  or empty when the host has none. The hash covers the file size and the
  first and last 64 KiB, so a remux or re-encode gets a new hash. It is a
  fingerprint, not proof that two files hold the same bytes: different files
  can share one.
- `file_size`: the file size in bytes, or 0 when unknown.
- `accepts_absent`: the host understands `MarkerSegment.absent`.

`file_hash` is the raw OpenSubtitles hash, and it identifies a file across
every server that holds the same bytes. A plugin must not send it off the
machine unhashed. Hash it before sending, for example
`hex(sha256(file_hash))[0:16]` as Silo's marker service does. An unsalted
hash like this hides the raw value, but anyone who holds the OpenSubtitles
hash can compute it, and every service that uses the same scheme gets the
same identifier. Use a service-specific salt or HMAC key when the service
must not be linkable to others.

A `MarkerSegment` with `absent` set asserts that the item has no segment of
that kind. Its `start_seconds` and `end_seconds` are unset, and `confidence`
applies to the absence. A provider must not return absent segments unless
`accepts_absent` is true: a host older than the field reads a segment without
bounds as covering the whole file.

## Submitting

`SubmitMarkerRequest` names the item the same way as `FetchMarkersRequest`
and carries one segment with its bounds. Since v0.30.0 it also carries:

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

## Reporting

A segment can be wrong, and the viewer watching it knows first. A provider that
accepts reports serves the separate `MarkerReporting` service, and the host
exposes one operation of its own for it, so every client offers the same control
without knowing which provider supplied the markers.

- `MarkerSegment.reportable` (added in v0.33.0) says, per segment, that the
  provider would accept a report for it now. Set it only when that is true: a
  provider that reports through a shared service credential sets it only while
  the operator has configured that credential. The host draws no report control
  for a segment that is not reportable.
- `ReportMarkerRequest` carries the same identity a fetch carries, including
  `episode_numbering`, plus the segment kind. It carries no bounds: a report is
  a complaint that a segment is wrong, not a replacement to store. Its
  `season_number` is `optional`, and presence means what it means everywhere
  else in this contract: unset is "the host could not place the episode in a
  season", an explicit zero is Specials, and a movie leaves it unset. A provider
  that cannot tell those apart may report the wrong episode, so refuse instead of
  guessing.
- `ReportMarkerResponse.outcome` is reported, already reported, or refused.
  already reported is settled, not a failure: a service that keys one report
  per account per item answers it for a repeat, and the viewer is told the
  segment is already known about. A refusal carries `reason`, an open
  vocabulary, and `detail`, words for the viewer or the administrator.
- Faults the provider classes as errors use gRPC status codes, as `SubmitMarker`
  does: InvalidArgument for a request it will never accept, FailedPrecondition
  for missing provider state such as an unconfigured credential, and
  ResourceExhausted with RetryInfo when the caller should try later.

Register the service with `runtime.WithMarkerReporting`, or implement
`MarkerReportingServer` on the same type as your `MarkerProviderServer` and the
runtime registers it for you. A provider that does not serve it answers
Unimplemented, which hosts read as "this provider cannot report", not as an
error: no client is offered a report control for it.

## Compatibility

All the v0.30.0 fields are additive. A host older than v0.30.0 leaves them
empty: no file facts, `accepts_absent` false, and no `source`, `algorithm`,
`confidence`, or `absent` on submissions. Providers must handle an empty
`file_hash` and a `file_size` of 0, and treat a submission with an empty
`source` as one whose origin is unknown.

Reporting is additive too. `MarkerProvider` and `CapabilityServers` are
unchanged: the report RPC is a separate service so a provider that implements the
released interface directly still compiles. A plugin built before v0.33.0 sends
no `reportable` and serves no `MarkerReporting`, so a host that has both offers
no report control for it; a host built before it ignores the service and reads
`reportable` as false. A host must not offer a report for a segment that came
from a provider it cannot call: treat a provider that does not serve
`MarkerReporting` as unable to report, whatever `reportable` says.
