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

There are currently 4 supported notifier types:

-   [`bigquery`](./bigquery/README.md), which writes Build updates and related
    data to a BigQuery table.
-   [`http`](./http/README.md), which sends (HTTP `POST`s) a JSON payload to
    another HTTP endpoint.
-   [`slack`](./slack/README.md), which uses a Slack webhook to post a message
    in a Slack channel.
-   [`smtp`](./smtp/README.md), which sends emails via an SMTP server.

**See the official documentation on Google Cloud for how to configure each notifier:**

- [Configuring BigQuery notifications](https://cloud.google.com/cloud-build/docs/configuring-notifications/configure-bigquery)
- [Configuring HTTP notifications](https://cloud.google.com/cloud-build/docs/configuring-notifications/configure-http)
- [Configuring Slack notifications](https://cloud.google.com/cloud-build/docs/configuring-notifications/configure-slack)
- [Configuring SMTP notifications](https://cloud.google.com/cloud-build/docs/configuring-notifications/configure-smtp)


## Setup Script

A [setup script](./setup.sh) exists that should automate _most_ of the notifier setup.

Run `./setup.sh --help` for usage instructions.

## Common Flags

The following are flags that belong to every notifier via inclusion of the `lib/notifiers` library.

### `--smoketest`

This flag starts up the notifier image but only logs the notifier name (via type) and then exits.

### `--setup_check`

This flag starts up the notifier, which does the following:

1. Read the notifier configuration YAML from STDIN.
1. Decode it into a configuration object.
1. Attempt to call `notifier.SetUp` on the given notifier using the configuration and a faked-out `SecretGetter`.
1. Exit successfully unless one of the previous steps failed.

This can be done using the following commands:

```bash
# First build the notifier locally.
$ sudo docker build . \
    -f=./${NOTIFIER_TYPE}/Dockerfile --tag=${NOTIFIER_TYPE}-test
# Then run the `setup_check` with your YAML.
# --interactive to allow reading from STDIN.
# --rm to clean/remove the image once it exits.
$ sudo docker run \
    --interactive \
    --rm \ 
    --name=${NOTIFIER_TYPE}-test \
    ${NOTIFIER_TYPE}-test:latest --setup_check --alsologtostderr -v=5 \
    < path/to/my/config.yaml 
```

## License

This project uses an [Apache 2.0 license](./LICENSE).

## Contributing

See [here](./CONTRIBUTING.md) for contributing guidelines.

## Support

There are several ways to get support for issues in this project:

-   [Cloud Build Slack channel](https://googlecloud-community.slack.com/archives/C4KCRJL4D)
-   [Cloud Build Issue Tracker](https://issuetracker.google.com/issues/new?component=190802&template=1162743)
-   [General Google Cloud support](https://cloud.google.com/cloud-build/docs/getting-support)

Note: Issues filed in this repo are not guaranteed to be addressed.
We recommend filing issues via the [Issue Tracker](https://issuetracker.google.com/issues/new?component=190802&template=1162743).

