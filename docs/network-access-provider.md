# Network access providers (`network_access_provider.v1`)

A network access provider gives a Silo deployment an overlay-network identity
(Tailscale via tsnet first; NetBird later) so clients reach Silo without port
forwarding or a public reverse proxy. The plugin owns the overlay client and a
reverse proxy in front of the host's local listeners. The host owns
supervision, per-instance state storage, status aggregation, access-path
routing, and the admin API.

Added in SDK v0.16.0. Server support arrives in stages; check the host's
`/api/v2/network-access/capabilities` before assuming a feature exists.

## Manifest

Declare the capability with the typed descriptor. The host lists providers
from the descriptor without launching the plugin.

```json
{
  "type": "network_access_provider.v1",
  "id": "tailscale",
  "display_name": "Tailscale",
  "network_access_provider": {
    "provider": "tailscale",
    "display_name": "Tailscale"
  }
}
```

`provider` is a stable lowercase, path-safe slug; it appears in admin API
paths such as `/api/v2/admin/network-access/{provider}/status`. Reserved
slugs today: `tailscale`, `netbird`.

## Resident lifecycle

Plugins declaring this capability are resident. The host:

- starts them when the API listener binds (and on every proxy node),
- restarts them on exit or failed health with exponential backoff,
- stops them last during shutdown.

Every host (the API server and each proxy node) runs its own instance of the
same installation. Each instance has its own overlay identity and its own
instance-state scope. Do not assume a single process.

## gRPC service

```proto
service NetworkAccessProvider {
  rpc Connect(NetworkAccessConnectRequest) returns (NetworkAccessStatus);
  rpc Disconnect(NetworkAccessDisconnectRequest) returns (NetworkAccessStatus);
  rpc GetStatus(NetworkAccessGetStatusRequest) returns (NetworkAccessStatus);
}
```

Register it with `runtime.CapabilityServers{NetworkAccessProvider: ...}`.

`NetworkAccessStatus.state` is one of `disconnected`,
`awaiting_authorization`, `connecting`, `connected`, `error`. The vocabulary
is open; the host tolerates values it does not know. `Connect` may return
`awaiting_authorization` or `connecting` and finish enrollment in the
background; push each later transition with
`RuntimeHost.ReportNetworkAccessStatus` so the host does not poll. The host
still calls `GetStatus` on demand.

`desired_connected` is plugin-owned intent. Persist it in instance state (see
below), report it in every status, and reconnect on start when it is true.
The host never writes it.

## What the host tells you: `GetHostInfo`

`RuntimeHost.GetHostInfo` carries everything the proxy needs:

| Field | Meaning |
|---|---|
| `host_role` | `api` or `proxy`. |
| `host_name` | Node name on proxies, server name on the API host. |
| `node_id` | `stream_nodes.id` on proxies, `0` on the API host. |
| `ingress_token` | Per-process-start secret; see below. |
| `listeners` | Local listeners to expose: `name`, loopback `address`, `default_port`. |

`listeners` always contains `api`. On the API host it also carries `jellyfin`
and `abs` when those listeners are enabled. Proxies report only `api`.
`default_port` is the port the plugin should expose that listener on (443 for
`api` under HTTPS, 8096 for `jellyfin`, 13378 for `abs`); zero means use the
provider default. Expose every listener the host reports and return one
`NetworkAccessListener` per exposed listener in the status; `origin` is the
`api` listener's origin.

The typed client is `runtimehost.Client.GetHostInfo`, which returns
`runtimehost.HostInfo` with `Listener(name)` for lookup.

## Proxy contract

For every request the plugin forwards to a host listener:

- Preserve the incoming `Host` header.
- Overwrite `X-Forwarded-Proto` with `https`.
- Set `X-Forwarded-For` to the overlay peer address.
- Set `X-Silo-Ingress-Token` to the value from `GetHostInfo`, replacing any
  client-supplied value; the host answers 403 when the header carries more
  than one value.

The host validates the token, strips the header, and records the request's
access path so stream URLs point tailnet clients at overlay origins. The
plugin's loopback source is in the host's default trusted-proxy list, so the
forwarded headers are honoured.

The ingress token rotates on every host process start and is compared in
constant time. Keep it in memory only; never log or persist it. After a host
or plugin restart, call `GetHostInfo` again before proxying; a stale token is
rejected with `403`.

## Instance state

Per-instance state (tsnet node keys, profile state, `desired_connected`)
lives in the host, encrypted, scoped to installation plus host. The plugin
never sees the scope and never writes files.

```proto
rpc ReadInstanceState(ReadInstanceStateRequest) returns (ReadInstanceStateResponse);
rpc WriteInstanceState(WriteInstanceStateRequest) returns (WriteInstanceStateResponse);
```

Limits: key up to 256 bytes, value up to 256 KiB, up to 256 keys per scope.
The SDK client checks the key and value limits before the round trip.

`runtimehost.InstanceStateStore` exposes the RPCs in the two-method shape of
tailscale's `ipn.StateStore`:

```go
type StateStore interface {
    ReadState(key string) ([]byte, error)   // runtimehost.ErrStateNotExist when absent
    WriteState(key string, value []byte) error
}
```

The SDK does not import tailscale, and `ipn.StateStore` takes `ipn.StateKey`
rather than `string`, so wrap it in the plugin:

```go
type tsStore struct{ inner *runtimehost.InstanceStateStore }

func (s tsStore) ReadState(k ipn.StateKey) ([]byte, error) {
    b, err := s.inner.ReadState(string(k))
    if errors.Is(err, runtimehost.ErrStateNotExist) {
        return nil, ipn.ErrStateNotExist
    }
    return b, err
}

func (s tsStore) WriteState(k ipn.StateKey, v []byte) error {
    return s.inner.WriteState(string(k), v)
}

srv := &tsnet.Server{Store: tsStore{inner: host.InstanceStateStore()}}
```

Each `ReadState`/`WriteState` call uses a 30 s deadline by default;
`WithTimeout` changes it.

## Enrollment

Support both an auth key in plugin config (for hands-off enrollment across
many nodes) and an interactive auth URL as the fallback. While enrollment is
pending, report `state: awaiting_authorization` with `auth_url` set. The
`auth_url` is admin-only: never log it, and clear it once enrollment
completes.

## Logging

Log state transitions and errors. Never log `auth_url`, `ingress_token`,
auth keys, or instance state values.

## Example

[`examples/hello-network-access`](../examples/hello-network-access) is a stub
provider with no overlay network. It shows the manifest, registration,
instance-state persistence of `desired_connected`, and the status push, and
serves as a test fixture for the host.
