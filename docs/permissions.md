# Plugin permissions

A plugin's manifest says what data it receives and what it may call on the
host. The administrator approves that when installing the plugin, and again
when an upgrade asks for more. Until an upgrade's new permissions are
approved, the plugin keeps running with what was approved before.

Permissions don't sandbox a plugin: it is native code running on the server,
and installing one trusts its publisher. They make what a plugin receives
visible and keep an upgrade from quietly widening it.

## Events

The host derives event permissions from `subscriptions`. Every catalog event
has an access class ([events.md](events.md#access)): subscribing to an
`admin` event, such as `session.started`, asks for admin activity, and
subscribing to a `user_data` event, such as `notification.created`, asks for
profiles' personal data. `public` events and plugin events need no approval.

## Host API scopes

A plugin that calls the host's HTTP API lists the API key scopes it needs:

```json
{
  "plugin_id": "example.activity",
  "host_api_scopes": ["admin:monitor:read"]
}
```

The scope vocabulary is the host's API key scope catalog. Ask for the
narrowest scopes that cover the routes you call. A host grants only scopes it
knows, so check `HostAPICredential.Scopes` before relying on one.

Once an administrator approves the scopes, the host issues an API key that
carries exactly those scopes and acts as the administrator who approved it.
It appears on the host's API keys page, where it can be revoked. Read it with
`runtimehost.Client.HostAPICredential`:

```go
cred, err := sdkruntime.Host().HostAPICredential(ctx)
switch {
case errors.Is(err, runtimehost.ErrHostAPINotGranted):
	// Not approved yet: tell the administrator, don't retry in a loop.
case errors.Is(err, runtimehost.ErrHostAPIUnsupported):
	// The host is too old to issue keys.
case err != nil:
	return err
}
req, _ := http.NewRequestWithContext(ctx, http.MethodGet, cred.BaseURL+"/api/v2/admin/sessions", nil)
req.Header.Set("Authorization", "Bearer "+cred.Token)
```

Treat the token as a credential: keep it in memory, never log or persist it,
and call `HostAPICredential` again after the host answers `401`, because the
administrator can revoke or reissue it. The key stops working if the
administrator who approved it loses the admin role; a new approval issues a
new key.

Don't ask administrators to paste API keys into your plugin's settings to get
the same access.
