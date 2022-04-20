# Cloud Build Google Chat Notifier

This notifier uses [Google Chat Webhooks](https://developers.google.com/chat/how-tos/webhooks) to
send notifications to your Google Chat space.

This notifier runs as a container via Google Cloud Run and responds to
events that Cloud Build publishes via its
[Pub/Sub topic](https://cloud.google.com/cloud-build/docs/send-build-notifications).

For detailed instructions on setting up this notifier,
see [Configuring Google Chat notifications](https://cloud.google.com/cloud-build/docs/configuring-notifications/configure-googlechat).

## Configuration Variables

This notifier expects the following fields in the `delivery` map to be set:

- `webhook_url`: The `secretRef: <GoogleChat-webhook-URL>` map that references the
Google Chat webhook URL resource path in the `secrets` section.
