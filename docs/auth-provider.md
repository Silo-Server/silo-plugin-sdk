# Auth providers (`auth_provider.v1`)

An auth provider lets people sign in to Silo with an account that lives
somewhere else: an OIDC identity provider, an LDAP directory, or a billing
system. The plugin talks to the provider and returns an identity. The host owns
Silo accounts, account linking, sessions, roles, and the login UI.

## Manifest

```json
{
  "type": "auth_provider.v1",
  "id": "main",
  "display_name": "Company SSO",
  "auth_modes": ["oauth2"],
  "metadata": { "connection_test": true }
}
```

- `auth_modes` lists the flows the plugin serves: `password` (the host calls
  `Authenticate` with a username and password) and `oauth2` (the host calls
  `InitAuthorize` and `ExchangeCode`). An empty list means `password`. Use the
  `manifest.AuthModePassword` and `manifest.AuthModeOAuth2` constants. The
  list is an open vocabulary: hosts ignore modes they don't recognize, so a
  plugin that adds a newer mode should still list one an older host
  understands. For this capability type `manifest.Validate` rejects empty
  modes, duplicates, and case variants of the known modes such as `OAuth2`.
- `metadata.connection_test` must be a boolean when present. `true` tells the
  host it may call `AuthProviderChecks.TestConnection`. Hosts read it with
  `manifest.AuthProviderSupportsConnectionTest`. `ServeManifestWithOptions`
  panics at start when it is `true` but no `AuthProviderChecks` server is
  registered.
- `icon_url` suits only an absolute URL of an externally hosted image. The
  manifest is embedded at build time and the host requires the installed copy
  to match it, so it cannot contain the installation id a plugin-served asset
  URL needs. Serve your own icon as described under [Login button](#login-button).

### Login button

The host builds the login button from two global-config keys, which override
the capability's manifest values. Declare them in `global_config_schema` so the
operator can set them per installation; each value has the shape
`{"value": "..."}`:

| Key | Meaning |
|---|---|
| `display_name` | Button label, such as `Company SSO`. |
| `icon_url_path` | Path of the icon under the plugin's `/assets/` route, such as `logo.svg`. |

From `icon_url_path` the host builds
`/api/v2/plugin-content/plugins/<installation id>/assets/<path>`. The plugin
must serve that path from a public `http_routes` `GET /assets/...` route,
typically from files embedded with `go:embed`; catalog installs ship only the
binary, so packaged static assets are not available. The host drops an icon
whose route isn't public.

## Settings

Declare settings in `global_config_schema` with `admin_form` fields and mark
secrets with `secret: true`. The host renders the form, stores secret values
encrypted, and redacts them in admin views; the plugin receives plaintext.

The host calls `Runtime.Configure` once per process start and restarts the
plugin when an operator saves new settings. A plugin served by
`runtime.ServeManifestWithOptions` receives the entries through
`runtime.WithConfigure` instead of writing its own `Runtime` server:

```go
runtime.ServeManifestWithOptions(manifestJSON, version,
    runtime.CapabilityServers{AuthProvider: provider},
    runtime.WithAuthProviderChecks(provider),
    runtime.WithConfigure(func(ctx context.Context, entries []*pluginv1.ConfigEntry) error {
        // Store whatever arrived, even nothing. Fail only on input that
        // cannot be decoded at all.
        return provider.apply(entries)
    }))
```

Without `WithConfigure`, `Configure` stays a no-op.

`Configure` must succeed when settings are empty or incomplete. A fresh
install sends no entries at all, and the host still has to start the plugin to
run `TestConnection` against staged settings. Store whatever arrives and return
an error only for input the plugin cannot store, such as a value of the wrong
type. An error from the callback fails the plugin start, so the host can't
reach the plugin until an operator fixes the settings. Report missing or
invalid settings, such as an empty issuer, elsewhere:

- as failed steps from `TestConnection`;
- at sign-in, as an `AUTH_DENIAL_PROVIDER_UNAVAILABLE` denial with a
  `denial_detail` naming the setting, or as a gRPC error.

The host bounds `Configure` with a short control timeout, so parse and store
settings there and leave network calls, such as OIDC discovery, to the
capability RPCs.

## Sign-in

`Authenticate` (password) and `ExchangeCode` (OAuth) both return
`AuthenticateResponse`. `external_subject` is the stable account key: the host
keys identities by plugin installation and `external_subject`, so it must not
change for the same person. Use the provider's immutable ID, not a username or
email.

Fields added in v0.22.0, all optional:

| Field | Meaning |
|---|---|
| `issuer` | Who asserted the subject, such as an OIDC `iss` or an LDAP URL. Informational. |
| `username` | Preferred login name; the base for a new Silo username. |
| `email_verified` | Whether the provider verified `email`. Unset means the provider did not say; `false` is a real answer. Pass the provider's value through as-is. |
| `groups` | Group names after the plugin's normalization. |
| `picture_url` | Absolute `http` or `https` URL of the account picture. |
| `managed_role` | `USER` or `ADMIN` when the plugin's group rules decide the Silo role. `UNSPECIFIED` leaves the role to the host. |
| `refresh_state` | Opaque state for later `CheckAccount` calls, such as a refresh token. The host stores it encrypted and sends it back unchanged. Unset keeps the stored state; an empty Struct clears it. |
| `denial`, `denial_detail` | Why the provider refused the sign-in. See below. |

The `claims` Struct remains for provider-specific data. The host does not read
it.

### Denials

When the provider refuses a sign-in for a known reason, return a response with
`denial` set and **`external_subject` empty**, instead of a gRPC error:

| Denial | Use when |
|---|---|
| `INVALID_CREDENTIALS` | Wrong username or password, or an invalid code. |
| `NOT_PERMITTED` | The account exists but fails the plugin's access rules, such as allowed groups. |
| `ACCOUNT_DISABLED` | The provider disabled or locked the account. |
| `PASSWORD_EXPIRED` | The password must be changed at the provider first. |
| `PROVIDER_UNAVAILABLE` | The provider could not be reached, or the plugin's settings are incomplete. |

Leaving `external_subject` empty keeps a denial fail-closed on hosts built
before v0.22.0: they ignore `denial` but refuse any response without a
subject. `denial_detail` is for the operator log; the host never shows it to
the person signing in. Never put secrets in it.

A gRPC error still means an unexpected failure.

### Authorization requests

`InitAuthorizeRequest` gained `prompt` (an OIDC `prompt` value such as `login`
or `select_account`) and `login_hint`. Empty values mean the plugin's
configured default and no hint. `linking` is true when a signed-in user is
linking the provider to an existing Silo account.

## Checks (`AuthProviderChecks`)

`AuthProviderChecks` is a separate gRPC service so the released
`AuthProviderServer` Go interface keeps its four methods. Register it with
`runtime.WithAuthProviderChecks(server)`. Embed
`pluginv1.UnimplementedAuthProviderChecksServer` to implement only some of its
RPCs, and add a compile-time assertion so a method-set mistake fails the build:

```go
var _ pluginv1.AuthProviderChecksServer = (*Provider)(nil)
```

Without the option, the runtime still registers the service when the
`AuthProvider` server itself satisfies `pluginv1.AuthProviderChecksServer`. That
check happens at run time: a value whose methods have pointer receivers, or a
wrapper around the server, fails it silently, so prefer the explicit option.

The host reaches the service through `(*runtime.Client).AuthProviderChecks()`.
Plugins built before v0.22.0 answer `Unimplemented`. Hosts then show no
connection test, handle `CheckAccount` like `UNSUPPORTED` (the account's
sessions get an absolute age limit and don't slide, and its API keys and
Audiobookshelf sessions are revoked once the person has gone too long without
signing in through the provider), and skip provider logout.

### TestConnection

The host sends the operator's staged settings in
`AuthTestConnectionRequest.config`, in the same shape as `Configure`, before
they are saved. Test that config, not the running one, and persist nothing.
Return one `AuthTestStep` per check in the order they ran, such as
"discovery reachable", "issuer matches", or "service bind". Each step has a
stable `id`, an operator-facing `label`, `ok`, and a `message`. Set the
response's `ok` only when every step passed. Never echo secrets into labels or
messages.

### CheckAccount

The host calls `CheckAccount` to re-check an account that signed in through
the plugin, without the person present.
`CheckAccountRequest` carries `external_subject` and the latest
`refresh_state`. Answer with a `status`:

| Status | Meaning |
|---|---|
| `ACTIVE` | The account exists and passes the access rules. Return current details in `account`. |
| `NOT_FOUND` | The provider no longer knows the account. |
| `DISABLED` | The provider disabled or locked it. |
| `NOT_PERMITTED` | It exists but no longer passes the access rules. The host revokes the account's Silo sessions and deletes its API keys. Use it for a refused refresh token only under the rule below. |
| `UNSUPPORTED` | The plugin cannot check it, for example because the provider never issued a refresh token. The host gives the account's sessions an absolute age limit. Once the person has not signed in through the provider within `auth.refresh_token_expiry` (30 days by default), the host also deletes the account's API keys and revokes its Audiobookshelf sessions. Also use it for a refused refresh token that may have expired. |
| `UNAVAILABLE` | The provider could not be reached. The host retries and does not treat this as a denial. |
| `UNSPECIFIED` | Not an answer; a zero-value response decodes to it. |

Providers refuse an expired refresh token and a revoked one with the same
error, such as an OIDC `invalid_grant`. Answer `NOT_PERMITTED` for a refused
token only when you know the token was inside its lifetime, for example from
the provider's expiry hint or a lifetime the admin configured; then the
refusal can only be a revocation. Otherwise answer `UNSUPPORTED`. A
`NOT_PERMITTED` for a token that merely expired would sign the person out of
every Silo session and delete their API keys.

Hosts treat `UNSPECIFIED`, a status they don't recognize, and a gRPC error
other than `Unimplemented` like `UNAVAILABLE`: they log, keep the account as it
is, and retry. None of them ever counts as `ACTIVE`.

Put rotated state, such as a new refresh token, in `account.refresh_state`;
the same unset/empty rules apply as on sign-in. The host applies every update
to the account named by the request's `external_subject`; it ignores
`account.external_subject`, `account.denial`, and `account.denial_detail` in a
`CheckAccount` response. A `managed_role` value the host doesn't recognize
leaves the role unchanged.

The host guarantees that at most one `CheckAccount` is in flight per
installation and `external_subject` across all nodes. When it receives and
commits an answer, it persists a rotated `account.refresh_state` (for example
under a row lock or compare-and-swap) before the next call for that account. A
plugin can rotate a single-use refresh token without two nodes presenting the
same token, which providers with reuse detection answer by revoking the whole
token family.

That guarantee does not cover a lost answer. If the call fails in transport,
the host node or the plugin dies, or the host's write does not commit, the
plugin may already have spent the stored token at the provider while the host
still holds the previous `refresh_state`. The next call then carries that
previous state. The host records that the earlier outcome is unknown and
applies a `NOT_FOUND`, `DISABLED`, or `NOT_PERMITTED` from that next call as
`UNSUPPORTED`, because it may be the provider refusing an already rotated
token. A plugin that rotates single-use tokens should also mark its own state
uncertain while a refresh's outcome is unknown, so it can tell a refused stale
token from a real denial.

The host does not call `RefreshSession`, which remains in the contract
unchanged.

### EndSessionUrl

On web sign-out the web client asks the host for the provider logout URL
while the Silo session is still valid, then ends the Silo session and
navigates the browser to the URL itself. To answer, the host calls
`EndSessionUrl` with the account's `external_subject`, its latest
`refresh_state`, and the `post_logout_redirect_uri` to return to. The host
bounds the call at about 2 seconds and answers empty past it, and it skips the
call for API keys and impersonation sessions. Return the provider's logout
URL, such as an OIDC `end_session_endpoint` with `id_token_hint`. Keep the ID token in `refresh_state` if you need it for the
hint. Return an empty `url` when the provider has no end-session endpoint or
the operator turned provider logout off.
