# Contributing to the Silo Plugin SDK

The [Silo contribution guide](https://github.com/Silo-Server/.github/blob/main/CONTRIBUTING.md)
covers project-wide coordination, focused changes, evidence, AI disclosure, and
pull request expectations. Those requirements apply here; this guide adds the
SDK-specific workflow.

## Before you start

Open an [issue](https://github.com/Silo-Server/silo-plugin-sdk/issues) before
adding or changing a capability, protobuf contract, manifest field, runtime
behavior, or public Go API. The SDK is a versioned contract consumed by
`silo-server` and every plugin, so identify affected downstream repositories and
the intended compatibility strategy before implementation.

Host-only behavior belongs in
[`silo-server`](https://github.com/Silo-Server/silo-server); provider-specific
behavior belongs in the individual plugin repository.

## Development setup

Use the Go version declared in `go.mod`. Protobuf regeneration also requires
`protoc`; `make proto` installs the generator tools under `bin/` as needed.
Read [docs/compatibility.md](docs/compatibility.md) before changing a public
contract.

## Validate your change

Run focused package tests while iterating. Before opening a pull request, run:

```sh
go test ./...
go vet ./...
go build ./examples/hello-scheduled-task
go build ./examples/hello-runtime-host
gofmt -l .
```

For protobuf or generated-code changes, also run:

```sh
make proto
```

`gofmt -l .` should print nothing. If it reports unrelated pre-existing drift,
none of the Go files touched by your change may appear in the output; do not add
to the output, and report what remains. When no contract change is intended,
`make proto` must leave generated code unchanged. Add compatibility coverage
for presence semantics, validation, and existing consumers when a contract
changes.

## Open the pull request

Use a Conventional Commit title, explain the compatibility and release impact,
list downstream coordination, and paste the actual validation results. Read the
[AI-assisted contribution policy](https://github.com/Silo-Server/silo-server/blob/main/docs/ai-contributions.md)
and include its disclosure block.
