# Service graph processor

| Status                   |                   |
| ------------------------ |-------------------|
| Stability                | [alpha]           |
| Supported pipeline types | traces            |
| Distributions            | [contrib], [sumo] |

The service graphs processor is a traces processor that builds a map representing the interrelationships between various services in a system.
The processor will analyse trace data and generate metrics describing the relationship between the services.
These metrics can be used by data visualization apps (e.g. Grafana) to draw a service graph.

Service graphs are useful for a number of use-cases:

* Infer the topology of a distributed system. As distributed systems grow, they become more complex. Service graphs can help you understand the structure of the system.
* Provide a high level overview of the health of your system.
Service graphs show error rates, latencies, among other relevant data.
* Provide an historic view of a system’s topology.
Distributed systems change very frequently,
and service graphs offer a way of seeing how these systems have evolved over time.

This component is based on [Grafana Tempo's service graph processor](https://github.com/grafana/tempo/tree/main/modules/generator/processor/servicegraphs).

## How it works

Service graphs work by inspecting traces and looking for spans with parent-children relationship that represent a request.
The processor uses the OpenTelemetry semantic conventions to detect a myriad of requests.
It currently supports the following requests:

* A direct request between two services where the outgoing and the incoming span must have `span.kind` client and server respectively.
* A request across a messaging system where the outgoing and the incoming span must have `span.kind` producer and consumer respectively.
* A database request; in this case the processor looks for spans containing attributes `span.kind`=client as well as db.name.

Every span that can be paired up to form a request is kept in an in-memory store,
until its corresponding pair span is received or the maximum waiting time has passed.
When either of these conditions are reached, the request is recorded and removed from the local store.

Each emitted metrics series have the client and server label corresponding with the service doing the request and the service receiving the request.

```
traces_service_graph_request_total{client="app", server="db", connection_type="database"} 20
```

TLDR: The processor will try to find spans belonging to requests as seen from the client and the server and will create a metric representing an edge in the graph.

## Metrics

The following metrics are emitted by the processor:

| Metric                                      | Type      | Labels                          | Description                                                  |
|---------------------------------------------|-----------|---------------------------------|--------------------------------------------------------------|
| traces_service_graph_request_total          | Counter   | client, server, connection_type | Total count of requests between two nodes                    |
| traces_service_graph_request_failed_total   | Counter   | client, server, connection_type | Total count of failed requests between two nodes             |
| traces_service_graph_request_server_seconds | Histogram | client, server, connection_type | Time for a request between two nodes as seen from the server |
| traces_service_graph_request_client_seconds | Histogram | client, server, connection_type | Time for a request between two nodes as seen from the client |
| traces_service_graph_unpaired_spans_total   | Counter   | client, server, connection_type | Total count of unpaired spans                                |
| traces_service_graph_dropped_spans_total    | Counter   | client, server, connection_type | Total count of dropped spans                                 |

Duration is measured both from the client and the server sides.

Possible values for `connection_type`: unset, `messaging_system`, or `database`.

Additional labels can be included using the `dimensions` configuration option. Those labels will have a prefix to mark where they originate (client or server span kinds).
The `client_` prefix relates to the dimensions coming from spans with `SPAN_KIND_CLIENT`, and the `server_` prefix relates to the
dimensions coming from spans with `SPAN_KIND_SERVER`.

Since the service graph processor has to process both sides of an edge,
it needs to process all spans of a trace to function properly.
If spans of a trace are spread out over multiple instances, spans are not paired up reliably.
A possible solution to this problem is using the [load balancing exporter](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/loadbalancingexporter)
in a layer on front of collector instances running this processor.

## Visualization

Service graph metrics are natively supported by Grafana since v9.0.4.
To run it, configure a Tempo data source's 'Service Graphs' by linking to the Prometheus backend where metrics are being sent:

```yaml
apiVersion: 1
datasources:
  # Prometheus backend where metrics are sent
  - name: Prometheus
    type: prometheus
    uid: prometheus
    url: <prometheus-url>
    jsonData:
        httpMethod: GET
    version: 1
  - name: Tempo
    type: tempo
    uid: tempo
    url: <tempo-url>
    jsonData:
      httpMethod: GET
      serviceMap:
        datasourceUid: 'prometheus'
    version: 1
```

## Configuration

The following settings are required:

- `metrics_exporter`: the name of the exporter that this processor will write metrics to. This exporter **must** be present in a pipeline.
- `latency_histogram_buckets`: the list of durations defining the latency histogram buckets.
    - Default: `[2ms, 4ms, 6ms, 8ms, 10ms, 50ms, 100ms, 200ms, 400ms, 800ms, 1s, 1400ms, 2s, 5s, 10s, 15s]`
- `dimensions`: the list of dimensions to add together with the default dimensions defined above.

The following settings can be optionally configured:

- `store` defines the config for the in-memory store used to find requests between services by pairing spans.
    - `ttl` - TTL is the time to live for items in the store.
      - Default: `2ms`
    - `max_items` - MaxItems is the maximum number of items to keep in the store.
      - Default: `1000` 
- `cache_loop` - the time to cleans the cache periodically
- `store_expiration_loop`  the time to expire old entries from the store periodically.
- `virtual_node_peer_attributes` the list of attributes need to match for building virtual server node, the higher the front, the higher the priority.
  - Default: `[db.name, net.sock.peer.addr, net.peer.name, rpc.service, net.sock.peer.name, net.peer.name, http.url, http.target]`

## Example configuration

```yaml
receivers:
  otlp:
    protocols:
      grpc:
  otlp/servicegraph: # Dummy receiver for the metrics pipeline
    protocols:
      grpc:
        endpoint: localhost:12345

processors:
  servicegraph:
    metrics_exporter: prometheus/servicegraph # Exporter to send metrics to
    latency_histogram_buckets: [100us, 1ms, 2ms, 6ms, 10ms, 100ms, 250ms] # Buckets for latency histogram
    dimensions: [cluster, namespace] # Additional dimensions (labels) to be added to the metrics extracted from the resource and span attributes
    store: # Configuration for the in-memory store
      ttl: 2s # Value to wait for an edge to be completed
      max_items: 200 # Amount of edges that will be stored in the storeMap      
    cache_loop: 2m # the time to cleans the cache periodically
    store_expiration_loop: 10s # the time to expire old entries from the store periodically.
    virtual_node_peer_attributes:
      - db.name
      - rpc.service
exporters:
  prometheus/servicegraph:
    endpoint: localhost:9090
    namespace: servicegraph
  otlp:
    endpoint: localhost:4317

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [servicegraph]
      exporters: [otlp]
    metrics/servicegraph:
      receivers: [otlp/servicegraph]
      processors: []
      exporters: [prometheus/servicegraph]
```

## Features and Feature-Gates

See the [Collector feature gates](https://github.com/open-telemetry/opentelemetry-collector/blob/main/featuregate/README.md#collector-feature-gates) for an overview of feature gates in the collector.

[alpha]: https://github.com/open-telemetry/opentelemetry-collector#alpha
[contrib]: https://github.com/open-telemetry/opentelemetry-collector-releases/tree/main/distributions/otelcol-contrib
[sumo]: https://github.com/SumoLogic/sumologic-otel-collector
