# Hello Plugin App

This example is a minimal plugin-owned web app exposed through `http_routes.v1`.

It demonstrates:

- declaring HTTP routes in `manifest.json`
- marking one route as a user-sidebar app with `navigation_kind: "user"`
- marking one route as an admin-sidebar app with `navigation_kind: "admin"`
- serving simple HTML through the SDK's `HttpRoutes` gRPC service
- embedding a manifest template and computing the executable checksum

## Build

```sh
go build -o hello-plugin-app ./examples/hello-plugin-app
```

## Inspect the Manifest

```sh
./hello-plugin-app manifest
```

After installation, Silo renders the user route under the main sidebar Apps section and the admin route in admin plugin navigation.
