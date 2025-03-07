# Metrics Generation Processor

| Status                   |                   |
| ------------------------ | ----------------- |
| Stability                | [development]     |
| Supported pipeline types | metrics           |
| Distributions            | [contrib], [sumo] |

**Status: under development; Not recommended for production usage.**

## Description

The metrics generation processor (`experimental_metricsgenerationprocessor`) can be used to create new metrics using existing metrics following a given rule. Currently it supports following two approaches for creating a new metric.

1. It can create a new metric from two existing metrics by applying one of the folliwing arithmetic operations: add, subtract, multiply, divide and percent. One use case is to calculate the `pod.memory.utilization` metric like the following equation-
`pod.memory.utilization` = (`pod.memory.usage.bytes` / `node.memory.limit`)
1. It can create a new metric by scaling the value of an existing metric with a given constant number. One use case is to convert `pod.memory.usage` metric values from Megabytes to Bytes (multiply the existing metric's value by 1,048,576)

## Configuration

Configuration is specified through a list of generation rules. Generation rules find the metrics which 
match the given metric names and apply the specified operation to those metrics.

```yaml
processors:
    # processor name: experimental_metricsgeneration
    experimental_metricsgeneration:

        # specify the metric generation rules
        rules:
              # Name of the new metric. This is a required field.
            - name: <new_metric_name>

              # Unit for the new metric being generated.
              unit: <new_metric_unit>

              # type describes how the new metric will be generated. It can be one of `calculate` or `scale`.  calculate generates a metric applying the given operation on two operand metrics. scale operates only on operand1 metric to generate the new metric.
              type: {calculate, scale}

              # This is a required field.
              metric1: <first_operand_metric>

              # This field is required only if the type is "calculate".
              metric2: <second_operand_metric>

              # Operation specifies which arithmetic operation to apply. It must be one of the five supported operations.
              operation: {add, subtract, multiply, divide, percent}
```

## Example Configurations

### Create a new metric using two existing metrics
```yaml
# create pod.cpu.utilized following (pod.cpu.usage / node.cpu.limit)
rules:
    - name: pod.cpu.utilized
      type: calculate
      metric1: pod.cpu.usage
      metric2: node.cpu.limit
      operation: divide
```

### Create a new metric scaling the value of an existing metric
```yaml
# create pod.memory.usage.bytes from pod.memory.usage.megabytes
rules:
    - name: pod.memory.usage.bytes
      unit: Bytes
      type: scale
      metric1: pod.memory.usage.megabytes
      operation: multiply
      scale_by: 1048576
```

[development]: https://github.com/open-telemetry/opentelemetry-collector#development
[contrib]: https://github.com/open-telemetry/opentelemetry-collector-releases/tree/main/distributions/otelcol-contrib
[sumo]: https://github.com/SumoLogic/sumologic-otel-collector
