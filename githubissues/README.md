# Cloud Build GitHub Issues Notifier

This notifier uses [GitHub Webhooks](https://docs.github.com/en/developers/webhooks-and-events/webhooks/creating-webhooks) to
create issues against your GitHub repo.

This notifier runs as a container via Google Cloud Run and responds to
events that Cloud Build publishes via its
[Pub/Sub topic](https://cloud.google.com/cloud-build/docs/send-build-notifications).

For detailed instructions on setting up this notifier,
see [Configuring GitHub Issue notifications](https://cloud.google.com/cloud-build/docs/configuring-notifications/configure-githubissues).

## Configuration Variables

This notifier expects the following fields in the `delivery` map to be set:

- `githubRepo`: The name of the repo to create an issue against (e.g. `youruser/yourrepo`)
- `githubToken`: The `secretRef: <github-token>` map that references the GitHub Issue token resource path in the `secrets` section.

This notifier also takes a custom `template` that can either be set inline, or as a uri, as a
JSON object specifying at minimum the customisable `title` and `body` (in Markdown) of the issue. See [GitHub's REST documentation](https://docs.github.com/en/rest/issues/issues#create-an-issue) for more body parameters. See TODO for more on templates.
