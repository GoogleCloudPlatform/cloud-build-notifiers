# Notifiers Golang library

This `notifiers` Go package exposes a lightweight, zero-magic, _optional_
framework for writing new notifiers and extensions.

To write your own notifier using this package, all you need to do is write
something that implements the `notifiers.Notifier` interface and then pass that
to `notifiers.Main` in your Go executable's `main` method (or wherever). That
`Main` function will set up your notifiers with your config from GCS and your
secrets stored on Secret Manager. Feel free to copy the `cloudbuild.yaml` and
`Dockerfile` in the notifiers that use this package, like `http`, to build and
deploy your own notifier.

In order to filter on specific notifications, you can use the
`notifiers.EventFilter` interface, again, optionally. This library provides two
`EventFilter` implementations:

- `notifiers.CELPredicate`: This filter uses a
compiled-at-startup [CEL](https://opensource.google/projects/cel) program string
to filter on incoming notifications. It uses a single input variable named
`event` and features the same fields as `notifiers.CloudBuildEvent`. For
example, you can write a filter like `event.status == "SUCCESS" || "special" in
event.tags` to only notify on events that are successful or have the `"special"`
build tag.

- `notifiers.TriggerPredicate`: This filter uses the given trigger
name to filter on build events.
