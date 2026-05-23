# Hello Scheduled Task

This example is a minimal self-describing plugin binary.

It demonstrates:

- embedding a manifest template with `//go:embed`
- computing the executable checksum at runtime
- returning the manifest through `Runtime.GetManifest`
- exposing a `scheduled_task.v1` capability

## Build

```sh
go build -o hello-scheduled-task ./examples/hello-scheduled-task
```

## Inspect the Manifest

```sh
./hello-scheduled-task manifest
```

## Install into Silo

Upload the built binary through the admin plugin upload flow, or publish it through a plugin catalog entry that points at the binary URL and checksum.
