# Cloud Build Microsoft Teams Notifier

This notifier uses HTTP to `POST` JSON payload notifications in the 
[legacy actionable message format](https://docs.microsoft.com/en-us/outlook/actionable-messages/message-card-reference) 
to a given
[incoming webhook connector](https://teams.microsoft.com/l/app/203a1e2c-26cc-47ca-83ae-be98f960b6b2?source=store-copy-link).

This notifier runs as a container via Google Cloud Run and responds to
events that Cloud Build publishes via its
[Pub/Sub topic](https://cloud.google.com/cloud-build/docs/send-build-notifications).

As this notifier is effectively an extension of the HTTP notifier, for detailed instructions on setting up this notifier,
see [Configuring HTTP notifications](https://cloud.google.com/cloud-build/docs/configuring-notifications/configure-http).

## Configuration Variables

This notifier expects the following fields in the `delivery` map to be set:

- `url`: The HTTP endpoint of the incoming webhook connector to which `POST` requests will be sent. No sort of
authentication is expected or used.
