# Hello Scan Source

A minimal `scan_source.v1` plugin for Silo Autoscan. Another tool appends
changed paths to a change-log file, one per line, and each poll returns the
lines added since the previous poll.

It demonstrates:

- a self-describing binary: embedded manifest, checksum computed at runtime
- a setup descriptor in `metadata.scan_source` with one per-source setting
  (`changes_file`) that the Add-source flow renders as a text field
- reading that setting from `source_config` on every `PollChanges`
- marker handling: an empty marker starts from now, the marker is the byte
  offset just past the last complete line reported (so each poll reads only
  what was appended), and a line still being written waits for the next poll.
  A log that goes missing after the source has read it fails the poll until
  it returns, so the host keeps the marker and nothing is replayed
- rotation: to rotate the log, truncate it or start the new file empty. The
  poll then reads the new log from the start when the marker is past its end
  or no longer falls just after a newline. If the new log grows past the old
  marker and a newline happens to sit there, the lines before it are missed.
  The example also cannot tell a log replaced by a different, longer file
  from one that grew. A production plugin should put a file identity (such as
  the inode) or a check of the bytes before the offset into the marker
- structured `changes`: a line ending in `/` is a `SUBTREE` change, anything
  else is a `FILE` change

See [docs/scan-source.md](../../docs/scan-source.md) for the full contract.

## Build

Build for the platform your Silo server runs on, usually Linux. For an ARM
server, use `GOARCH=arm64`:

```sh
GOOS=linux GOARCH=amd64 go build -o hello-scan-source ./examples/hello-scan-source
```

## Inspect the manifest

On a matching platform:

```sh
./hello-scan-source manifest
```

## Test

```sh
go test ./examples/hello-scan-source
```

## Try it in Silo

1. Upload the binary in **Admin → Plugins**.
2. In **Admin → Libraries → Autoscan**, enable Autoscan, choose **Add
   source**, pick **Change log file**, and enter the log path as the Silo
   server sees it, for example `/data/silo-changes.log`.
3. Wait for one poll so the source records its starting position. Lower the
   global poll interval in the Autoscan settings to make this quicker.
4. Append a path inside one of your libraries:

   ```sh
   echo "/data/movies/Example (2024)/Example (2024).mkv" >> /data/silo-changes.log
   ```

   After the next poll, the source's activity log shows the change and the
   scan it queued.

The file must be readable from inside the Silo server's environment, such as
its container. With more than one Silo node, any node in `api` or
`integrated` mode can run a poll, so the path must reach the same file on
every one of them, for example on a shared mount.
