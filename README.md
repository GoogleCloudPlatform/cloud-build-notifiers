# Cloud Build Notifiers

This repo provides deployable notifier images and sources, as well as libraries
for creating new notifiers.

[Cloud Build](https://cloud.google.com/cloud-build) notifiers are Docker
containers that connect to the
[Cloud Build Pub/Sub topic](https://cloud.google.com/cloud-build/docs/send-build-notifications)
that adapt Pub/Sub messages about Build update notifications to other
services/protocols, such as SMTP for email.
Cloud Build notifiers are long-lived binaries that receive notifications throughout
Builds' lifecycles (e.g. from the Build starting to execute through the Build finishing).

All notifiers can be built by Cloud Build and deployed on
[Cloud Run](https://cloud.google.com/run). The only prerequisite is to be a
Cloud Build user and to have the
[gcloud CLI tool](https://cloud.google.com/sdk/gcloud/) installed and configured
for your Cloud Build project(s).

There are currently 3 supported notifier types:

-   [`smtp`](./smtp/README.md), which sends emails via an SMTP server.
-   [`http`](./http/README.md), which sends (HTTP `POST`s) a JSON payload to
    another HTTP endpoint.
-   [`slack`](./slack/README.md), which uses a Slack webhook to post a message
    in a Slack channel.

**See the official Google Cloud docs
[here](https://cloud.google.com/cloud-build/docs/configure-notifications) for how to use Cloud Build notifiers.**

## License

This project uses an [Apache 2.0 license](./LICENSE.txt).

## Contributing

See [here](./CONTRIBUTING.md) for contributing guidelines.

## Support

There are several ways to get support for issues in this project:

-   [Cloud Build Slack channel](https://googlecloud-community.slack.com/archives/C4KCRJL4D)
-   [Cloud Build Issue Tracker](https://issuetracker.google.com/issues/new?component=190802&template=1162743)
-   [General Google Cloud support](https://cloud.google.com/cloud-build/docs/getting-support)

Note: Issues filed in this repo are not guaranteed to be addressed.
We recommend filing issues via the [Issue Tracker](https://issuetracker.google.com/issues/new?component=190802&template=1162743).

