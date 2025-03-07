# File Storage

| Status        |                                         |
| ------------- | --------------------------------------- |
| Stability     | [beta]                                  |
| Distributions | [contrib], [observiq], [splunk], [sumo] |

The File Storage extension can persist state to the local file system. 

The extension requires read and write access to a directory. A default directory can be used, but it must already exist in order for the extension to operate.

`directory` is the relative or absolute path to the dedicated data storage directory. 
The default directory is `%ProgramData%\Otelcol\FileStorage` on Windows and `/var/lib/otelcol/file_storage` otherwise.

`timeout` is the maximum time to wait for a file lock. This value does not need to be modified in most circumstances.
The default timeout is `1s`.

## Compaction
`compaction` defines how and when files should be compacted. There are two modes of compaction available (both of which can be set concurrently):
- `compaction.on_start` (default: false), which happens when collector starts
- `compaction.on_rebound` (default: false), which happens online when certain criteria are met; it's discussed in more detail below

`compaction.directory` specifies the directory used for compaction (as a midstep).

`compaction.max_transaction_size` (default: 65536): defines maximum size of the compaction transaction.
A value of zero will ignore transaction sizes.

### Rebound (online) compaction

For rebound compaction, there are two additional parameters available:
- `compaction.rebound_needed_threshold_mib` (default: 100) - when allocated data exceeds this amount, the "compaction needed" flag will be enabled
- `compaction.rebound_trigger_threshold_mib` (default: 10) - if the "compaction needed" flag is set and allocated data drops below this amount, compaction will begin and the "compaction needed" flag will be cleared
- `compaction.check_interval` (default: 5s) - specifies how frequently the conditions for compaction are being checked

The idea behind rebound compaction is that in certain workloads (e.g. [persistent queue](https://github.com/open-telemetry/opentelemetry-collector/tree/main/exporter/exporterhelper#persistent-queue)) the storage might grow significantly (e.g. when the exporter is unable to send the data due to network problem) after which it is being emptied as the underlying issue is gone (e.g. network connectivity is back). This leaves a significant space that needs to be reclaimed (also, this space is reported in memory usage as mmap() is used underneath). The optimal conditions for this to happen online is after the storage is largely drained, which is being controlled by `rebound_trigger_threshold_mib`. To make sure this is not too sensitive, there's also `rebound_needed_threshold_mib` which specifies the total claimed space size that must be met for online compaction to even be considered. Consider following diagram for an example of meeting the rebound (online) compaction conditions.

```
  ▲
  │
  │             XX.............
m │            XXXX............
e ├───────────XXXXXXX..........────────────  rebound_needed_threshold_mib
m │         XXXXXXXXX..........
o │        XXXXXXXXXXX.........
r │       XXXXXXXXXXXXXXXXX....
y ├─────XXXXXXXXXXXXXXXXXXXXX..────────────  rebound_trigger_threshold_mib
  │   XXXXXXXXXXXXXXXXXXXXXXXXXX.........
  │ XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
  └──────────────── time ─────────────────►
     │           |            |
     issue       draining     compaction happens
     starts      begins       and reclaims space

 X - actually used space
 . - claimed but no longer used space
```


## Example

```
extensions:
  file_storage:
  file_storage/all_settings:
    directory: /var/lib/otelcol/mydir
    timeout: 1s
    compaction:
      on_start: true
      directory: /tmp/
      max_transaction_size: 65_536

service:
  extensions: [file_storage, file_storage/all_settings]
  pipelines:
    traces:
      receivers: [nop]
      processors: [nop]
      exporters: [nop]

# Data pipeline is required to load the config.
receivers:
  nop:
processors:
  nop:
exporters:
  nop:
```

[beta]: https://github.com/open-telemetry/opentelemetry-collector#beta
[contrib]: https://github.com/open-telemetry/opentelemetry-collector-releases/tree/main/distributions/otelcol-contrib
[observiq]: https://github.com/observIQ/observiq-otel-collector
[splunk]: https://github.com/signalfx/splunk-otel-collector
[sumo]: https://github.com/SumoLogic/sumologic-otel-collector
