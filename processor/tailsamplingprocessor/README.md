# Tail Sampling Processor

| Status                   |                                         |
| ------------------------ |-----------------------------------------|
| Stability                | [beta]                                  |
| Supported pipeline types | traces                                  |
| Distributions            | [contrib], [observiq], [splunk], [sumo] |

The tail sampling processor samples traces based on a set of defined policies. All spans for a given trace MUST be received by the same collector instance for effective sampling decisions.

Please refer to [config.go](./config.go) for the config spec.

The following configuration options are required:
- `policies` (no default): Policies used to make a sampling decision

Multiple policies exist today and it is straight forward to add more. These include:
- `always_sample`: Sample all traces
- `latency`: Sample based on the duration of the trace. The duration is determined by looking at the earliest start time and latest end time, without taking into consideration what happened in between.
- `numeric_attribute`: Sample based on number attributes (resource and record)
- `probabilistic`: Sample a percentage of traces. Read [a comparison with the Probabilistic Sampling Processor](#probabilistic-sampling-processor-compared-to-the-tail-sampling-processor-with-the-probabilistic-policy).
- `status_code`: Sample based upon the status code (`OK`, `ERROR` or `UNSET`)
- `string_attribute`: Sample based on string attributes (resource and record) value matches, both exact and regex value matches are supported
- `trace_state`: Sample based on [TraceState](https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/trace/api.md#tracestate) value matches
- `rate_limiting`: Sample based on rate
- `span_count`: Sample based on the minimum and/or maximum number of spans, inclusive. If the sum of all spans in the trace is outside the range threshold, the trace will not be sampled.
- `boolean_attribute`: Sample based on boolean attribute (resource and record).
- `ottl_condition`: Sample based on given boolean OTTL condition (span and span event).
- `and`: Sample based on multiple policies, creates an AND policy 
- `composite`: Sample based on a combination of above samplers, with ordering and rate allocation per sampler. Rate allocation allocates certain percentages of spans per policy order. 
  For example if we have set max_total_spans_per_second as 100 then we can set rate_allocation as follows
  1. test-composite-policy-1 = 50 % of max_total_spans_per_second = 50 spans_per_second
  2. test-composite-policy-2 = 25 % of max_total_spans_per_second = 25 spans_per_second
  3. To ensure remaining capacity is filled use always_sample as one of the policies

The following configuration options can also be modified:
- `decision_wait` (default = 30s): Wait time since the first span of a trace before making a sampling decision
- `num_traces` (default = 50000): Number of traces kept in memory
- `expected_new_traces_per_sec` (default = 0): Expected number of new traces (helps in allocating data structures)

Each policy will result in a decision, and the processor will evaluate them to make a final decision:

- When there's an "inverted not sample" decision, the trace is not sampled;
- When there's a "sample" decision, the trace is sampled;
- When there's a "inverted sample" decision and no "not sample" decisions, the trace is sampled;
- In all other cases, the trace is NOT sampled

An "inverted" decision is the one made based on the "invert_match" attribute, such as the one from the string tag policy.

Examples:

```yaml
processors:
  tail_sampling:
    decision_wait: 10s
    num_traces: 100
    expected_new_traces_per_sec: 10
    policies:
      [
          {
            name: test-policy-1,
            type: always_sample
          },
          {
            name: test-policy-2,
            type: latency,
            latency: {threshold_ms: 5000}
          },
          {
            name: test-policy-3,
            type: numeric_attribute,
            numeric_attribute: {key: key1, min_value: 50, max_value: 100}
          },
          {
            name: test-policy-4,
            type: probabilistic,
            probabilistic: {sampling_percentage: 10}
          },
          {
            name: test-policy-5,
            type: status_code,
            status_code: {status_codes: [ERROR, UNSET]}
          },
          {
            name: test-policy-6,
            type: string_attribute,
            string_attribute: {key: key2, values: [value1, value2]}
          },
          {
            name: test-policy-7,
            type: string_attribute,
            string_attribute: {key: key2, values: [value1, val*], enabled_regex_matching: true, cache_max_size: 10}
          },
          {
            name: test-policy-8,
            type: rate_limiting,
            rate_limiting: {spans_per_second: 35}
         },
         {
            name: test-policy-9,
            type: string_attribute,
            string_attribute: {key: http.url, values: [\/health, \/metrics], enabled_regex_matching: true, invert_match: true}
         },
         {
            name: test-policy-10,
            type: span_count,
            span_count: {min_spans: 2, max_spans: 20}
         },
         {
             name: test-policy-11,
             type: trace_state,
             trace_state: { key: key3, values: [value1, value2] }
         },
         {
              name: test-policy-12,
              type: boolean_attribute,
              boolean_attribute: {key: key4, value: true}
         },
         {
              name: test-policy-11,
              type: ottl_condition,
              ottl_condition: {
                   error_mode: ignore,
                   span: [
                        "attributes[\"test_attr_key_1\"] == \"test_attr_val_1\"",
                        "attributes[\"test_attr_key_2\"] != \"test_attr_val_1\"",
                   ],
                   spanevent: [
                        "name != \"test_span_event_name\"",
                        "attributes[\"test_event_attr_key_2\"] != \"test_event_attr_val_1\"",
                   ]
              }
         },
         {
            name: and-policy-1,
            type: and,
            and: {
              and_sub_policy: 
              [
                {
                  name: test-and-policy-1,
                  type: numeric_attribute,
                  numeric_attribute: { key: key1, min_value: 50, max_value: 100 }
                },
                {
                    name: test-and-policy-2,
                    type: string_attribute,
                    string_attribute: { key: key2, values: [ value1, value2 ] }
                },
              ]
            }
         },
         {
            name: composite-policy-1,
            type: composite,
            composite:
              {
                max_total_spans_per_second: 1000,
                policy_order: [test-composite-policy-1, test-composite-policy-2, test-composite-policy-3],
                composite_sub_policy:
                  [
                    {
                      name: test-composite-policy-1,
                      type: numeric_attribute,
                      numeric_attribute: {key: key1, min_value: 50, max_value: 100}
                    },
                    {
                      name: test-composite-policy-2,
                      type: string_attribute,
                      string_attribute: {key: key2, values: [value1, value2]}
                    },
                    {
                      name: test-composite-policy-3,
                      type: always_sample
                    }
                  ],
                rate_allocation:
                  [
                    {
                      policy: test-composite-policy-1,
                      percent: 50
                    },
                    {
                      policy: test-composite-policy-2,
                      percent: 25
                    }
                  ]
              }
          },
        ]
```

Refer to [tail_sampling_config.yaml](./testdata/tail_sampling_config.yaml) for detailed examples on using the processor.

### Scaling collectors with the tail sampling processor

This processor requires all spans for a given trace to be sent to the same collector instance for the correct sampling decision to be derived. When scaling the collector, you'll then need to ensure that all spans for the same trace are reaching the same collector. You can achieve this by having two layers of collectors in your infrastructure: one with the [load balancing exporter][loadbalancing_exporter], and one with the tail sampling processor.

While it's technically possible to have one layer of collectors with two pipelines on each instance, we recommend separating the layers in order to have better failure isolation.

### Probabilistic Sampling Processor compared to the Tail Sampling Processor with the Probabilistic policy

The [probabilistic sampling processor][probabilistic_sampling_processor] and the probabilistic tail sampling processor policy work very similar: based upon a configurable sampling percentage they will sample a fixed ratio of received traces. But depending on the overall processing pipeline you should prefer using one over the other.

As a rule of thumb, if you want to add probabilistic sampling and...

...you are not using the tail sampling processor already: use the [probabilistic sampling processor][probabilistic_sampling_processor]. Running the probabilistic sampling processor is more efficient than the tail sampling processor. The probabilistic sampling policy makes decision based upon the trace ID, so waiting until more spans have arrived will not influence its decision. 

...you are already using the tail sampling processor: add the probabilistic sampling policy. You are already incurring the cost of running the tail sampling processor, adding the probabilistic policy will be negligible. Additionally, using the policy within the tail sampling processor will ensure traces that are sampled by other policies will not be dropped.

[beta]: https://github.com/open-telemetry/opentelemetry-collector#beta
[contrib]: https://github.com/open-telemetry/opentelemetry-collector-releases/tree/main/distributions/otelcol-contrib
[probabilistic_sampling_processor]: ../probabilisticsamplerprocessor
[loadbalancing_exporter]: ../../exporter/loadbalancingexporter
[splunk]: https://github.com/signalfx/splunk-otel-collector
[observiq]: https://github.com/observIQ/observiq-otel-collector
[sumo]: https://github.com/SumoLogic/sumologic-otel-collector
