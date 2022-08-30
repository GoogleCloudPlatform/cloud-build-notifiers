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

## For release 1.15 and above:
Please do not upgrade to 1.15 as it contains bindings/templating functionality which may break existing slack setups below 1.15. Official documentation will be released detailing usage for bindings/templating, but for now the feature is in alpha so existing users are recommended to use releases older than 1.15.

You can specify the slack version like so:
```
gcloud run deploy service-name \
   --image=us-east1-docker.pkg.dev/gcb-release/cloud-build-notifiers/slack:slack-1.14.0 \
   --no-allow-unauthenticated \
   --update-env-vars=CONFIG_PATH=config-path,PROJECT_ID=project-id
```
