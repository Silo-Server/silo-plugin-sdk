# Scan sources (`scan_source.v1`)

A scan source tells Silo Autoscan which paths changed. Silo calls the plugin's
`PollChanges` on a timer, and the plugin answers with the paths that changed
since the last call. Silo does everything else: it rewrites the paths, matches
them to libraries, debounces repeats, queues scans, and records each poll in
the admin Autoscan activity log.

A working reference is in
[`examples/hello-scan-source`](../examples/hello-scan-source). It reads paths
that another tool appends to a change-log file.

## Who does what

| Silo (host) | Plugin |
|---|---|
| Runs the poll timer and enforces the per-source interval | Asks its upstream (an API, a filesystem, a queue) what changed |
| Stores the marker and sends it back on the next poll | Turns the marker into "changes since then" and returns a new marker |
| Stores the operator's per-source settings and connection, and sends them on every poll | Reads `source_config` and `connection` from the request |
| Applies the source's path rewrites | Returns absolute paths as the upstream sees them |
| Matches paths to library folders, debounces, dedupes, and queues scans | Chooses a scope (file or subtree) for each path |
| Records poll results and errors in the activity log | Returns errors written for the operator |

The plugin never starts a scan, never stores a marker, and never sees Silo's
library layout. Keep it stateless: the same capability can back several
sources with different settings, and Silo may restart the plugin process at
any time. Everything a poll needs arrives in the request.

## Declare the capability

Declare a `scan_source.v1` capability and describe its setup in
`metadata.scan_source`. Silo reads this descriptor to build the Add-source
flow without launching the plugin.

```json
{
  "type": "scan_source.v1",
  "id": "changelog",
  "display_name": "Change log file",
  "description": "Reads paths that another tool appends to a change-log file.",
  "metadata": {
    "scan_source": {
      "summary": "Point it at a file that another tool appends changed paths to, one per line.",
      "delivery_modes": ["poll"],
      "connection": "none",
      "config_form": {
        "fields": [
          {
            "key": "changes_file",
            "label": "Change log file",
            "description": "Absolute path of the file to read, as the Silo server sees it.",
            "control": "TEXT",
            "required": true
          }
        ]
      }
    }
  }
}
```

The source picker shows `display_name` as the card title, with `summary`
under it (or `description` when there is no summary).

Every descriptor key is optional. A capability with no `scan_source` block is
treated as poll delivery with an optional connection and no config form. Silo
reads the top-level descriptor keys leniently: a value of the wrong type is
ignored rather than rejected. `config_form` is all or nothing: one field value
of the wrong type (for example `"required": "true"`) drops the whole form.
Check the Add-source flow after you change the descriptor.

| Key | Type | Meaning |
|---|---|---|
| `summary` | string | One sentence for the source picker card. |
| `icon_url` | string | Absolute URL of an icon. Silo accepts and returns it, but the current admin UI does not display it. |
| `delivery_modes` | string array | Only `"poll"` is honoured for plugins. `"webhook"` is reserved for Silo's built-in Sonarr/Radarr receiver and is dropped from plugin descriptors. If no listed mode is understood, Silo falls back to `["poll"]`. |
| `connection` | string | `"none"`: the flow hides the connection step. `"optional"`: the operator may bind a connection. `"required"`: the flow will not create the source until a connection is bound. Missing or unknown values mean `"optional"`. |
| `connection_kinds` | string array | Limits which stored connections the operator can bind, matched against the connection's kind (for example `"sonarr"`, `"radarr"`). Empty means any connection. |
| `emits_native_paths` | bool | The plugin returns paths that Silo can use as-is. The admin UI then does not treat a source with no path rewrites and no configured paths as misconfigured. |
| `config_form` | object | Per-source settings form; see below. |

### Per-source settings (`config_form`)

`config_form` uses the same admin-form shape as plugin config: `fields`, and
optionally `sections` and `submit_label`. Each field takes `key`, `label`,
`description`, `control`, and optionally `placeholder`, `required`,
`default_value`, `options` (for selects), `rows`, `show_when`, and
`validation`.

Write `control` as the short name: `TEXT`, `TEXTAREA`, `NUMBER`, `SWITCH`,
`SELECT`, or `MULTI_SELECT`. Silo passes the name through unchanged, and the
admin UI renders an unrecognized control as a text box.

Silo drops two kinds of field from a source form:

- Secret fields (`"secret": true` or `PASSWORD`). Source settings are stored
  and returned in plain text. Take credentials through a connection instead,
  which Silo stores encrypted.
- Fields with `"dynamic_options": true`. Their options are only filled on the
  plugin settings page, so a source form would show an empty select.

Fields can also set `fill_from` to `"library_paths_movie"` or
`"library_paths_tv"`. The form then offers a button that fills the field with
the paths of the operator's enabled movie or series libraries (mixed
libraries count as both), one per line.
The value arrives in `source_config` as a newline-separated string, so split it
on newlines, not commas.

Put the form in `metadata.scan_source.config_form`. Silo also reads a
capability `config_schema` entry with key `"scan_source"` as a fallback, but
that path stores controls under their protobuf enum names
(`ADMIN_FORM_CONTROL_SELECT`), which the source form does not translate, so
selects and switches render as text boxes. Do not rely on it.

## The `PollChanges` request

```proto
message PollChangesRequest {
  string capability_id = 1;
  string marker = 2;
  ResolvedConnection connection = 3;
  map<string, string> source_config = 4;
}
```

- `capability_id` is the capability being polled, useful when one plugin
  declares several.
- `source_config` holds the values the operator entered in your
  `config_form`, keyed by field `key`. Every value is a string with
  surrounding whitespace trimmed: switches arrive as `"true"` or `"false"`,
  numbers as their decimal text, and multi-selects as comma-separated values.
  A field the operator left blank may be absent or empty, and sources created
  before you added a field will not have it, so handle missing keys.
- `connection` carries `base_url` and `api_key` when the operator bound a
  connection. Silo resolves it on every poll, including connections that reuse
  a Requests integration's credentials, so the plugin never stores
  credentials. With no connection bound, both fields are empty. When the
  descriptor says `connection: "required"`, Silo will not save an enabled
  source without one and normally does not call `PollChanges` for a source
  that has none; it records "No server selected" on the source itself. If
  Silo cannot list the installed scan sources, the poll goes ahead with an
  empty connection, so still return a clear error when a connection you
  require is missing. With `"optional"`, return an error if a poll needs a
  connection it did not get.
  Do not log the request without redacting the key.

## Markers

The marker is your continuation token. Silo never parses it.

- **First poll.** The marker is empty. Start from now: return the current
  position as `next_marker` and no changes. Replaying history here would
  queue a scan for everything the upstream has ever reported.
- **Later polls.** Silo sends back the last marker it stored. Return the
  changes after it and a new marker.
- **Always return a non-empty `next_marker`**, even when nothing changed.
  Silo stores an empty value as "no marker", so the next poll looks like a
  first poll and anything that changed in between is skipped.
- **Make polls repeatable.** Silo keeps the old marker when a poll fails, so
  the same window is read again. Two polls with the same marker should return
  the same changes. Rescanning a path that was already handled is safe.
- **Handle a marker you cannot use.** If the upstream can still give you
  everything since the marker, read that: a truncated log holds only lines
  written after the truncation, so read it from the start. Otherwise (an
  expired cursor, a marker from an older plugin version) resynchronize to the
  current position or return an error. Do not replay history from before the
  marker.
- **Expect the marker to be cleared.** Silo clears it when the operator
  changes a source's connection or settings, or points a connection at a
  different server, and discards a poll's new marker if one of those edits
  lands while the poll runs. The next poll then arrives with an empty marker,
  like a first poll. Edits to the label, enabled state, interval, or path
  rewrites keep the marker.

Silo advances the marker when the window's work is done:

- the poll returned no changes;
- at least one path was matched and its scans were queued;
- every matched path was debounced as a recent duplicate;
- no path matched any library folder. The poll is shown as "unresolved", and
  Silo moves past it so a watcher that sees folders outside Silo's libraries
  does not stall.

Silo keeps the old marker and records the error on the source when:

- the connection could not be resolved (the plugin is not called);
- `PollChanges` returned an error or timed out;
- matching any path failed with an internal error, even if other paths
  matched. Scans already queued for the matched paths stay queued.

Silo also keeps the old marker when the scans could not be queued or the new
marker could not be saved. These two errors appear only in the activity log,
not on the source, and the source polls again on the next cycle.

## Returning changes

```proto
message PollChangesResponse {
  repeated string source_paths = 1;       // legacy
  string next_marker = 2;
  repeated ScanSourceChange changes = 3;  // preferred
}
```

Return `changes`. When `changes` has any entries, Silo ignores
`source_paths`. `source_paths` remains for older plugins and is handled like
`AUTO` changes.

Paths are absolute and in the upstream's namespace, for example the path as
Sonarr or a NAS sees it. Silo applies the source's path rewrites first, using
the longest matching prefix and converting `\` separators to `/`, so a
Windows-hosted upstream works. Set `emits_native_paths` when your paths
already match Silo's.

Each change has a scope:

| Scope | What Silo does with the rewritten path |
|---|---|
| `FILE` | Scans that path. A video file widens to a scan of its directory, which picks up replaced versions and rewritten sidecars, except directly at a library root, where only the file is scanned. Audiobook, ebook, and manga files stay exact. In a podcast library, `FILE` changes are skipped, including for deleted files; report the directory as `SUBTREE` instead. An existing directory is scanned as a subtree. A path that is a library root is dropped instead of becoming a full library scan. A media file that no longer exists is reconciled so its catalog row is marked missing, as long as the library root is still mounted. |
| `SUBTREE` | Queues a scan of that directory without first checking that it exists, for directory-level changes. The path must be below a library root; a library root itself is rejected. |
| `AUTO` or unspecified | Scans the **parent directory** of the path. `/movies/Film (2020)/Film.mkv` scans `/movies/Film (2020)`. A directory without a trailing slash scans its parent: `/movies/Film (2020)` scans `/movies`, which is a full library scan when `/movies` is the library root. Add a trailing slash (`/movies/Film (2020)/`) to scan the directory itself. |

Prefer `FILE` for files and `SUBTREE` for directories.

A scan only reconciles what it can read. When the scanned directory itself is
gone, the scanner leaves its catalog rows in place, the same way it protects
an unmounted share. To have a deleted, moved, or renamed directory's items
marked missing promptly, also report its parent directory, which still exists
and shows the directory as gone. Otherwise the rows stay until a scan of an
ancestor or the next full library scan.

Silo skips a path without an error when it matches no library folder, matches
two libraries equally, or belongs to a disabled library. A `FILE` change for a
file that still exists but is not a media file the library supports (an
`.nfo`, a subtitle, an image) is also skipped; report the media file or the
directory instead.

## What Silo does after a poll

1. Applies the source's path rewrites.
2. Matches each path to a library folder and picks a scan target by scope.
3. Debounces repeats of an unchanged file: within the Autoscan debounce
   window (60 seconds by default), a report of a file whose size and
   modification time are the same as at its last report is skipped. A file
   that changed, was deleted, or reappeared is always scanned. Reports of an
   existing directory (including `SUBTREE` changes and directories reported
   as `AUTO`) are never debounced; the scan queue merges them with a matching
   scan that is waiting or running. A reported path that no longer exists,
   directory or file, is debounced like a deleted file: a repeat report
   within the window is skipped. The window is keyed on the reported path, after
   rewrites. Changes in one poll that resolve to the same target are queued
   once.
4. Queues the scans. If one poll produces more than 1,000 targets, Silo
   replaces them with one full scan per affected library.
5. Records the poll in the activity log with its status (success,
   unresolved, or error) and counts of changes returned, paths resolved,
   scans created or reused, and scans suppressed.

## Cadence, timeouts, and errors

- Autoscan is off until the operator enables it in the Autoscan settings.
- One host task polls every source at the global poll interval (600 seconds
  by default). A per-source interval only makes that source poll less often:
  a value below the global interval has no effect.
- A failed poll usually counts as a run, so a failing source retries at its
  normal interval, not immediately. If Silo fails to queue the scans or save
  the marker, the source polls again on the next cycle.
- Silo normally gives `PollChanges` up to five minutes and avoids
  overlapping polls of one source, but guarantees neither. Bound your
  upstream calls well below five minutes, and keep polls free of side
  effects so a repeated or overlapping poll is harmless.
- The text of an error you return is stored on the source (up to 2 KiB) and
  shown in the activity log, with credential-looking text masked. Write it for
  the operator, such as `changes_file is not configured`, and leave secrets
  out of it. Do not use the gRPC code `Unavailable` for your own errors: Silo
  shows it as "Plugin unavailable." because that is also what a crashed
  plugin returns. A plain Go error is fine.
- Silo polls a source only while its plugin is enabled. A disabled plugin's
  sources fail with an error and keep their markers.

## Install and try it

1. Build for the platform the Silo server runs on, usually Linux. For an
   ARM server, use `GOARCH=arm64`:

   ```sh
   GOOS=linux GOARCH=amd64 go build -o hello-scan-source ./examples/hello-scan-source
   ```

   A self-describing binary embeds its manifest and computes its checksum at
   startup, as the example does. Run `./hello-scan-source manifest` on a
   matching platform to see what Silo will read.
2. In Silo, open **Admin → Plugins** and upload the binary. Silo reads the
   manifest from the binary itself. Uploading the same `plugin_id` again
   replaces the installation. A zip upload also works if it holds the binary
   as `plugin` and a `manifest.json` with the binary's real checksum and its
   `supported_platforms`. To distribute the plugin, publish it in a catalog
   repository with the download URL and checksum, and operators install it
   from the catalog.
3. Open **Admin → Libraries → Autoscan**, enable Autoscan, and choose **Add
   source**. Your capability appears in the picker with its summary. The
   flow then asks for your config form fields and, unless `connection` is
   `"none"`, a connection.
4. Add path rewrites on the source if your paths differ from Silo's.
5. To test quickly, lower the global poll interval, make a change, and watch
   the source's activity log.

The plugin runs as a child process of the Silo server that polls, so any file,
mount, or network address it reads must be reachable from that server's
environment (inside its container, if it has one). In a deployment with more
than one Silo node, any node in `api` or `integrated` mode can run a poll, and
consecutive polls of one source can run on different nodes. The path or address
must resolve to the same data on every one of them.

The Sonarr/Radarr-specific helpers in the connection step, **Test
connection** and suggested path rewrites, call the Sonarr/Radarr API. They do
not work for other connection kinds.
