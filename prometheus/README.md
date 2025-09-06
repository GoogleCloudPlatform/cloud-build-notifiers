# Cloud Build Prometheus Notifier

This notifier sends Cloud Build metrics to Prometheus using remote write protocol. It collects various metrics about builds, steps, and their durations, making them available for monitoring and alerting.

## Dashboard

![grafana_dashboard_pipelines](/prometheus/docs/example_dashboard.jpeg)

https://grafana.com/grafana/dashboards/24009-cloud-build/

## Metrics

The notifier collects the following metrics:

- `cloudbuild_build_duration_seconds`: Duration of the entire build (from start to finish)
- `cloudbuild_build_queue_duration_seconds`: Duration the build spent waiting in the queue (from create to start)
- `cloudbuild_step_duration_seconds`: Duration of individual build steps
- `cloudbuild_build_last_run_status`: Status of the last build run (1.0 for success, 0.0 for failure)
- `cloudbuild_build_timestamp`: Timestamp of when the build finished (or started if no finish time)

Each metric includes labels such as:
- `cloud_account_id`: The GCP project ID
- `trigger_name`: Name of the Cloud Build trigger
- `repo_name`: Name of the source repository
- `status`: Build status (SUCCESS, FAILURE, etc.)
- `machine_type`: Type of machine used for the build (fetched from worker pool config for private pools)
- `ref_type`: Type of reference (branch/tag/unknown)
- `ref`: Name of the branch or tag
- `failure_type`: Type of failure (if build failed)

Additional labels for step metrics:
- `step_name`: Name of the individual build step
- `step_status`: Status of the individual build step
- `step_id`: ID of the individual build step

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
