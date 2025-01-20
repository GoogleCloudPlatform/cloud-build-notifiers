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
## Slack BlockKit Template Functions
- The `replace` function allows replacement of substrings in any {{template variables}} in the .json Slack template. (For example, the variable `.Build.FailureInfo.Detail` contains double quotes, which breaks the BlockKitTemplate parsing.)
   - Usage: `{{replace .Build.FailureInfo.Detail "\"" "'"}}`


```
GCP_PROJECT_ID=sinergia-f853f
BUCKET_NAME=sinergia-f853f-notifiers-config

gsutil ls -p ${GCP_PROJECT_ID} 

gsutil cp slack/slack_template.json gs://$BUCKET_NAME/
gsutil cat gs://$BUCKET_NAME/slack_template.json

gsutil cp slack/slack_testing.yaml gs://$BUCKET_NAME/slack.yaml

gcloud run deploy --project $GCP_PROJECT_ID sinergia-build-notification-slack \
   --image=us-east1-docker.pkg.dev/gcb-release/cloud-build-notifiers/slack \
   --no-allow-unauthenticated \
   --update-env-vars=CONFIG_PATH=gs://$BUCKET_NAME/slack.yaml,PROJECT_ID=$GCP_PROJECT_ID


```
```
Repo: {{.Build.Source.gitSource.Url}} \n Commit: {{.Build.Source.gitSource.Revision}} \n
```
---------

GCP_PROJECT_ID=sinergia-prod-f543f
BUCKET_NAME=sinergia-f543f-notifiers-config

gsutil ls -p ${GCP_PROJECT_ID} 

gsutil cp slack/slack_template.json gs://$BUCKET_NAME/
gsutil cat gs://$BUCKET_NAME/slack_template.json

gsutil cp slack/slack_testing.yaml gs://$BUCKET_NAME/slack.yaml

gcloud run deploy --project $GCP_PROJECT_ID sinergia-build-notification-slack \
   --image=us-east1-docker.pkg.dev/gcb-release/cloud-build-notifiers/slack \
   --no-allow-unauthenticated \
   --update-env-vars=CONFIG_PATH=gs://$BUCKET_NAME/slack.yaml,PROJECT_ID=$GCP_PROJECT_ID



https://cloud.google.com/go/docs/reference/cloud.google.com/go/cloudbuild/1.12.0/apiv1/v2/cloudbuildpb#cloud_google_com_go_cloudbuild_apiv1_v2_cloudbuildpb_Build