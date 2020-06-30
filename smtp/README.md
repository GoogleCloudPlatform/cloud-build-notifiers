# Cloud Build SMTP Notifier

This notifier uses SMTP to send email notifications.

This notifier runs as a container via Google Cloud Run and responds to
events that Cloud Build publishes via its
[Pub/Sub topic](https://cloud.google.com/cloud-build/docs/send-build-notifications).

For detailed instructions on setting up this notifier,
see [Configuring SMTP notifications](https://cloud.google.com/cloud-build/docs/configuring-notifications/configure-smtp).

## Configuration Variables

This notifier expects the following fields in `delivery` map to be set:

- `server`: The address of the SMTP server.
[If you want to use Gmail](https://developers.google.com/gmail/imap/imap-smtp),
use `smtp.gmail.com`.

- `port`: The port (as a string) that will handle SMTP
requests. If you want to use Gmail, use `587`.

- `sender`: This is the `From`
field - the email that will appear as the sender of the email.

- `recipients`: A
list of `To` addresses.

- `password`: The reference to a configuration in the
`secrets` list.
