# Cloud Build Prometheus Notifier

This notifier sends Cloud Build metrics to Prometheus using remote write protocol. It collects various metrics about builds, steps, and their durations, making them available for monitoring and alerting.

## Metrics

The notifier collects the following metrics:

- `cloudbuild_build_duration_seconds`: Duration of the entire build
- `cloudbuild_step_duration_seconds`: Duration of individual build steps
- `cloudbuild_build_last_run_status`: Status of the last build run

Each metric includes labels such as:
- `cloud_account_id`: The GCP project ID
- `trigger_name`: Name of the Cloud Build trigger
- `repo_name`: Name of the source repository
- `commit_sha`: Short SHA of the commit
- `status`: Build status
- `machine_type`: Type of machine used for the build
- `ref_type`: Type of reference (branch/tag)
- `ref`: Name of the branch or tag
- `failure_type`: Type of failure (if build failed)
- `failure_detail`: Detailed failure information (if build failed)

## Configuration

Create a configuration file following the example in `prometheus.yaml.example`:

```yaml
apiVersion: cloud-build-notifiers/v1
kind: PrometheusNotifier
metadata:
  name: example-prometheus-notifier
spec:
  notification:
    filter: build.status == Build.Status.SUCCESS
    delivery:
      url: https://prometheus-server:9090/api/v1/write
      # Optional basic auth configuration
      username: prometheus-user
      password:
        secretRef: prometheus-password
  secrets:
  - name: prometheus-password
    value: projects/example-project/secrets/example-prometheus-password/versions/latest
```

### Required Fields

- `delivery.url`: URL of the Prometheus remote write endpoint

### Optional Fields

- `delivery.username`: Username for basic authentication
- `delivery.password.secretRef`: Reference to a GCP secret containing the password for basic authentication

## Building

To build the notifier:

```bash
go build -o prometheus-notifier ./prometheus
```

## Testing

To run the tests:

```bash
go test ./prometheus
```

## Deployment

1. Build and push the container:
```bash
gcloud builds submit --config=deploy.cloudbuild.yaml
```

2. Deploy the notifier:
```bash
gcloud builds triggers create --config=prometheus.yaml
```

## Prometheus Configuration

Ensure your Prometheus server is configured to accept remote write requests:

```yaml
remote_write:
  - url: "https://prometheus-server:9090/api/v1/write"
    remote_timeout: 30s
```

## Security Considerations

- Use HTTPS for the remote write endpoint
- Consider using basic authentication
- Store sensitive credentials in GCP Secret Manager
- Use appropriate IAM permissions
