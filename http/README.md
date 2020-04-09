# Cloud Build HTTP Notifier

This notifier uses HTTP to `POST` JSON payload notifications to the given
recipient server.

> This is an Alpha release of Cloud Build Notifiers.
> This codebase might be changed in ways that are not backwards-compatible.
> We do not recommend using this codebase for production applications.
> Furthermore, this release is not subject to any SLA or deprecation policy.

This notifier runs as a container via Google Cloud Run and responds to
events that Cloud Build publishes via its
[Pub/Sub topic](https://cloud.google.com/cloud-build/docs/send-build-notifications).

For detailed instructions on setting up this notifier,
see [Configuring HTTP notifications](https://cloud.google.com/cloud-build/docs/configure-notifications#configuring_http_notifications).

## Configuration Variables

This notifier expects the following fields in the `delivery` map to be set:

- `url`: The HTTP endpoint to which `POST` requests will be sent. No sort of
authentication is expected or used.
