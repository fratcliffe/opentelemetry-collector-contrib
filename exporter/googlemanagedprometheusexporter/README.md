# Google Managed Service for Prometheus Exporter

| Status                   |                       |
| ------------------------ |-----------------------|
| Stability                | [beta]                |
| Supported pipeline types | metrics               |
| Distributions            | [contrib], [observiq] |

This exporter can be used to send metrics and traces to [Google Cloud Managed Service for Prometheus](https://cloud.google.com/stackdriver/docs/managed-prometheus).  The difference between this exporter and the `googlecloud` exporter is that metrics sent with this exporter are queried using [promql](https://prometheus.io/docs/prometheus/latest/querying/basics/#querying-prometheus), rather than standard the standard MQL.

This exporter is not the standard method of ingesting metrics into Google Cloud Managed Service for Prometheus, which is built on a drop-in replacement for the Prometheus server: https://github.com/GoogleCloudPlatform/prometheus.  This exporter does not support the full range of Prometheus functionality, including the UI, recording and alerting rules, and can't be used with the GMP Operator, but does support sending metrics.

## Configuration Reference

The following configuration options are supported:

- `project` (optional): GCP project identifier.
- `user_agent` (optional): Override the user agent string sent on requests to Cloud Monitoring (currently only applies to metrics). Specify `{{version}}` to include the application version number. Defaults to `opentelemetry-collector-contrib {{version}}`.
- `metric`(optional): Configuration for sending metrics to Cloud Monitoring.
  - `endpoint` (optional): Endpoint where metric data is going to be sent to. Replaces `endpoint`.
- `use_insecure` (optional): If true, use gRPC as their communication transport. Only has effect if Endpoint is not "".
- `retry_on_failure` (optional): Configuration for how to handle retries when sending data to Google Cloud fails.
  - `enabled` (default = false)
  - `initial_interval` (default = 5s): Time to wait after the first failure before retrying; ignored if `enabled` is `false`
  - `max_interval` (default = 30s): Is the upper bound on backoff; ignored if `enabled` is `false`
  - `max_elapsed_time` (default = 120s): Is the maximum amount of time spent trying to send a batch; ignored if `enabled` is `false`
- `sending_queue` (optional): Configuration for how to buffer traces before sending.
  - `enabled` (default = true)
  - `num_consumers` (default = 10): Number of consumers that dequeue batches; ignored if `enabled` is `false`
  - `queue_size` (default = 1000): Maximum number of batches kept in memory before data; ignored if `enabled` is `false`;
    User should calculate this as `num_seconds * requests_per_second` where:
    - `num_seconds` is the number of seconds to buffer in case of a backend outage
    - `requests_per_second` is the average number of requests per seconds.

Note: These `retry_on_failure` and `sending_queue` are provided (and documented) by the [Exporter Helper](https://github.com/open-telemetry/opentelemetry-collector/tree/main/exporter/exporterhelper#configuration)

## Example Configuration

```yaml
receivers:
    prometheus:
        config:
          scrape_configs:
            # Add your prometheus scrape configuration here.
            # Using kubernetes_sd_configs with namespaced resources (e.g. pod)
            # ensures the namespace is set on your metrics.
            - job_name: 'kubernetes-pods'
                kubernetes_sd_configs:
                - role: pod
                relabel_configs:
                - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
                action: keep
                regex: true
                - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
                action: replace
                target_label: __metrics_path__
                regex: (.+)
                - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
                action: replace
                regex: (.+):(?:\d+);(\d+)
                replacement: $$1:$$2
                target_label: __address__
                - action: labelmap
                regex: __meta_kubernetes_pod_label_(.+)
processors:
    batch:
        # batch metrics before sending to reduce API usage
        send_batch_max_size: 200
        send_batch_size: 200
        timeout: 5s
    memory_limiter:
        # drop metrics if memory usage gets too high
        check_interval: 1s
        limit_percentage: 65
        spike_limit_percentage: 20
    resourcedetection:
        # detect cluster name and location
        detectors: [gcp]
        timeout: 10s
    transform:
      # "location", "cluster", "namespace", "job", "instance", and "project_id" are reserved, and 
      # metrics containing these labels will be rejected.  Prefix them with exported_ to prevent this.
      metric_statements:
      - context: datapoint
        statements:
        - set(attributes["exported_location"], attributes["location"])
        - delete_key(attributes, "location")
        - set(attributes["exported_cluster"], attributes["cluster"])
        - delete_key(attributes, "cluster")
        - set(attributes["exported_namespace"], attributes["namespace"])
        - delete_key(attributes, "namespace")
        - set(attributes["exported_job"], attributes["job"])
        - delete_key(attributes, "job")
        - set(attributes["exported_instance"], attributes["instance"])
        - delete_key(attributes, "instance")
        - set(attributes["exported_project_id"], attributes["project_id"])
        - delete_key(attributes, "project_id")

exporters:
    googlemanagedprometheus:

service:
  pipelines:
    metrics:
      receivers: [prometheus]
      processors: [batch, memory_limiter, transform, resourcedetection]
      exporters: [googlemanagedprometheus]
```

## Resource Attribute Handling

The Google Managed Prometheus exporter maps metrics to the
[prometheus_target](https://cloud.google.com/monitoring/api/resources#tag_prometheus_target)
monitored resource. The logic for mapping to monitored resources is designed to
be used with the prometheus receiver, but can be used with other receivers as
well. To avoid collisions (i.e. "duplicate timeseries enountered" errors), you
need to ensure the prometheus_target resource uniquely identifies the source of
metrics. The exporter uses the following resource attributes to determine
monitored resource:

* location: [`location`, `cloud.availability_zone`, `cloud.region`]
* cluster: [`cluster`, `k8s.cluster.name`]
* namespace: [`namespace`, `k8s.namespace.name`]
* job: [`service.name` + `service.namespace`]
* instance: [`service.instance.id`]

In the configuration above, `cloud.availability_zone`, `cloud.region`, and
`k8s.cluster.name` are detected using the `resourcedetection` processor with
the `gcp` detector. The prometheus receiver sets `service.name` to the
configured `job_name`, and `service.instance.id` is set to the scrape target's
`instance`. The prometheus receiver sets `k8s.namespace.name` when using
`role: pod`.

### Manually Setting location, cluster, or namespace

In GMP, the above attributes are used to identify the `prometheus_target`
monitored resource. As such, it is recommended to avoid writing metric or resource labels
that match these keys. Doing so can cause errors when exporting metrics to
GMP or when trying to query from GMP. So, the recommended way to set them
is with the [resourcedetection processor](../../processor/resourcedetectionprocessor).

If you still need to set `location`, `cluster`, or `namespace` labels
(such as when running in non-GCP environments), you can do so with the
[resource processor](../../processor/resourceprocessor) like so:

```yaml
processors:
  resource:
    attributes:
    - key: "location"
      value: "us-east-1"
      action: upsert
```

### Setting cluster, location or namespace using metric labels

This example copies the `location` metric attribute to a new `exported_location`
attribute, then deletes the original `location`. It is recommended to use the `exported_*`
prefix, which is consistent with GMP's behavior.

You can also use the [groupbyattrs processor](../../processor/groupbyattrsprocessor)
to move metric labels to resource labels. This is useful in situations
where, for example, an exporter monitors multiple namespaces (with
each namespace exported as a metric label). One such example is kube-state-metrics.

Using `groupbyattrs` will promote that label to a resource label and 
associate those metrics with the new resource. For example:

```yaml
processors:
  groupbyattrs:
    keys:
    - namespace
    - cluster
    - location
```

[beta]: https://github.com/open-telemetry/opentelemetry-collector#beta
[contrib]: https://github.com/open-telemetry/opentelemetry-collector-releases/tree/main/distributions/otelcol-contrib
[observiq]: https://github.com/observIQ/observiq-otel-collector
