# Hello Notification Channel

A `notification_channel.v1` plugin that sends Silo notifications to an
[ntfy](https://ntfy.sh) topic. It shows:

- declaring a destination kind and its settings form in `manifest.json`
- checking a destination's settings in `ValidateDestination`
- mapping the service's answers onto `SendNotification` outcomes, so the
  host retries temporary failures and disables a destination whose token
  stops working
- keeping the server URL in the administrator's settings, since users who
  aren't administrators fill in destinations

## Build

```sh
go build -o hello-notification-channel ./examples/hello-notification-channel
./hello-notification-channel manifest
```

## Use

Install the binary through **Admin > Plugins**. Optionally set the ntfy
server in the plugin's settings (the default is `https://ntfy.sh`). Each
profile then adds an ntfy destination in its notification settings and
subscribes to the topic in the ntfy app.

See [docs/notification-channel.md](../../docs/notification-channel.md).
