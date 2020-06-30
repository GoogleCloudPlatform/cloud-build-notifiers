# Cloud Build Slack Notifier

This notifier uses [Slack Webhooks](https://api.slack.com/messaging/webhooks) to
send notifications to your Slack workspace.

This notifier runs as a container via Google Cloud Run and responds to
events that Cloud Build publishes via its
[Pub/Sub topic](https://cloud.google.com/cloud-build/docs/send-build-notifications).

For detailed instructions on setting up this notifier,
see [Configuring Slack notifications](https://cloud.google.com/cloud-build/docs/configuring-notifications/configure-slack).

## Configuration Variables

This notifier expects the following fields in the `delivery` map to be set:

- `webhook_url`: The `secretRef: <Slack-webhook-URL>` map that references the
Slack webhook URL resource path in the `secrets` section.
