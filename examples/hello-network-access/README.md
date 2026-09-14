# Hello Network Access

A stub `network_access_provider.v1` plugin with no overlay network. It shows
the contract shape and doubles as a host-side test fixture:

- `Connect` marks the instance connected, builds a fake origin per listener
  reported by `RuntimeHost.GetHostInfo`, persists `desired_connected` in
  host-provided instance state, and pushes the status with
  `ReportNetworkAccessStatus`.
- `Disconnect` reverses it.
- `GetStatus` restores `desired_connected` from instance state on first call
  so intent survives a plugin restart.

There is no tsnet dependency. A real provider is described in
[docs/network-access-provider.md](../../docs/network-access-provider.md).

## Build

```sh
go build -o hello-network-access ./examples/hello-network-access
```

## Inspect the manifest

```sh
./hello-network-access manifest
```
