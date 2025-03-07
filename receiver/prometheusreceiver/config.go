// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package prometheusreceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheusreceiver"

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	commonconfig "github.com/prometheus/common/config"
	promconfig "github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/file"
	promHTTP "github.com/prometheus/prometheus/discovery/http"
	"github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"gopkg.in/yaml.v2"
)

const (
	// The key for Prometheus scraping configs.
	prometheusConfigKey = "config"

	// keys to access the http_sd_config from config root
	targetAllocatorConfigKey       = "target_allocator"
	targetAllocatorHTTPSDConfigKey = "http_sd_config"
)

// Config defines configuration for Prometheus receiver.
type Config struct {
	PrometheusConfig *promconfig.Config `mapstructure:"-"`
	BufferPeriod     time.Duration      `mapstructure:"buffer_period"`
	BufferCount      int                `mapstructure:"buffer_count"`
	// UseStartTimeMetric enables retrieving the start time of all counter metrics
	// from the process_start_time_seconds metric. This is only correct if all counters on that endpoint
	// started after the process start time, and the process is the only actor exporting the metric after
	// the process started. It should not be used in "exporters" which export counters that may have
	// started before the process itself. Use only if you know what you are doing, as this may result
	// in incorrect rate calculations.
	UseStartTimeMetric   bool   `mapstructure:"use_start_time_metric"`
	StartTimeMetricRegex string `mapstructure:"start_time_metric_regex"`

	TargetAllocator *targetAllocator `mapstructure:"target_allocator"`

	// ConfigPlaceholder is just an entry to make the configuration pass a check
	// that requires that all keys present in the config actually exist on the
	// structure, ie.: it will error if an unknown key is present.
	ConfigPlaceholder interface{} `mapstructure:"config"`
}

type targetAllocator struct {
	Endpoint    string        `mapstructure:"endpoint"`
	Interval    time.Duration `mapstructure:"interval"`
	CollectorID string        `mapstructure:"collector_id"`
	// ConfigPlaceholder is just an entry to make the configuration pass a check
	// that requires that all keys present in the config actually exist on the
	// structure, ie.: it will error if an unknown key is present.
	ConfigPlaceholder interface{}        `mapstructure:"http_sd_config"`
	HTTPSDConfig      *promHTTP.SDConfig `mapstructure:"-"`
}

var _ component.Config = (*Config)(nil)
var _ confmap.Unmarshaler = (*Config)(nil)

func checkFile(fn string) error {
	// Nothing set, nothing to error on.
	if fn == "" {
		return nil
	}
	_, err := os.Stat(fn)
	return err
}

func checkTLSConfig(tlsConfig commonconfig.TLSConfig) error {
	if err := checkFile(tlsConfig.CertFile); err != nil {
		return fmt.Errorf("error checking client cert file %q: %w", tlsConfig.CertFile, err)
	}
	if err := checkFile(tlsConfig.KeyFile); err != nil {
		return fmt.Errorf("error checking client key file %q: %w", tlsConfig.KeyFile, err)
	}
	if len(tlsConfig.CertFile) > 0 && len(tlsConfig.KeyFile) == 0 {
		return fmt.Errorf("client cert file %q specified without client key file", tlsConfig.CertFile)
	}
	if len(tlsConfig.KeyFile) > 0 && len(tlsConfig.CertFile) == 0 {
		return fmt.Errorf("client key file %q specified without client cert file", tlsConfig.KeyFile)
	}
	return nil
}

// Method to exercise the prometheus file discovery behavior to ensure there are no errors
// - reference https://github.com/prometheus/prometheus/blob/c0c22ed04200a8d24d1d5719f605c85710f0d008/discovery/file/file.go#L372
func checkSDFile(filename string) error {
	content, err := os.ReadFile(filepath.Clean(filename))
	if err != nil {
		return err
	}

	var targetGroups []*targetgroup.Group

	switch ext := filepath.Ext(filename); strings.ToLower(ext) {
	case ".json":
		if err := json.Unmarshal(content, &targetGroups); err != nil {
			return fmt.Errorf("error in unmarshaling json file extension: %w", err)
		}
	case ".yml", ".yaml":
		if err := yaml.UnmarshalStrict(content, &targetGroups); err != nil {
			return fmt.Errorf("error in unmarshaling yaml file extension: %w", err)
		}
	default:
		return fmt.Errorf("invalid file extension: %q", ext)
	}

	for i, tg := range targetGroups {
		if tg == nil {
			return fmt.Errorf("nil target group item found (index %d)", i)
		}
	}
	return nil
}

// Validate checks the receiver configuration is valid.
func (cfg *Config) Validate() error {
	promConfig := cfg.PrometheusConfig
	if promConfig != nil {
		err := cfg.validatePromConfig(promConfig)
		if err != nil {
			return err
		}
	}

	if cfg.TargetAllocator != nil {
		err := cfg.validateTargetAllocatorConfig()
		if err != nil {
			return err
		}
	}
	return nil
}

func (cfg *Config) validatePromConfig(promConfig *promconfig.Config) error {
	if len(promConfig.ScrapeConfigs) == 0 && cfg.TargetAllocator == nil {
		return errors.New("no Prometheus scrape_configs or target_allocator set")
	}

	// Reject features that Prometheus supports but that the receiver doesn't support:
	// See:
	// * https://github.com/open-telemetry/opentelemetry-collector/issues/3863
	// * https://github.com/open-telemetry/wg-prometheus/issues/3
	unsupportedFeatures := make([]string, 0, 4)
	if len(promConfig.RemoteWriteConfigs) != 0 {
		unsupportedFeatures = append(unsupportedFeatures, "remote_write")
	}
	if len(promConfig.RemoteReadConfigs) != 0 {
		unsupportedFeatures = append(unsupportedFeatures, "remote_read")
	}
	if len(promConfig.RuleFiles) != 0 {
		unsupportedFeatures = append(unsupportedFeatures, "rule_files")
	}
	if len(promConfig.AlertingConfig.AlertRelabelConfigs) != 0 {
		unsupportedFeatures = append(unsupportedFeatures, "alert_config.relabel_configs")
	}
	if len(promConfig.AlertingConfig.AlertmanagerConfigs) != 0 {
		unsupportedFeatures = append(unsupportedFeatures, "alert_config.alertmanagers")
	}
	if len(unsupportedFeatures) != 0 {
		// Sort the values for deterministic error messages.
		sort.Strings(unsupportedFeatures)
		return fmt.Errorf("unsupported features:\n\t%s", strings.Join(unsupportedFeatures, "\n\t"))
	}

	for _, sc := range cfg.PrometheusConfig.ScrapeConfigs {
		for _, rc := range sc.MetricRelabelConfigs {
			if rc.TargetLabel == "__name__" {
				// TODO(#2297): Remove validation after renaming is fixed
				return fmt.Errorf("error validating scrapeconfig for job %v: %w", sc.JobName, errRenamingDisallowed)
			}
		}

		if sc.HTTPClientConfig.Authorization != nil {
			if err := checkFile(sc.HTTPClientConfig.Authorization.CredentialsFile); err != nil {
				return fmt.Errorf("error checking authorization credentials file %q: %w", sc.HTTPClientConfig.Authorization.CredentialsFile, err)
			}
		}

		if err := checkTLSConfig(sc.HTTPClientConfig.TLSConfig); err != nil {
			return err
		}

		for _, c := range sc.ServiceDiscoveryConfigs {
			switch c := c.(type) {
			case *kubernetes.SDConfig:
				if err := checkTLSConfig(c.HTTPClientConfig.TLSConfig); err != nil {
					return err
				}
			case *file.SDConfig:
				for _, file := range c.Files {
					files, err := filepath.Glob(file)
					if err != nil {
						return err
					}
					if len(files) != 0 {
						for _, f := range files {
							err = checkSDFile(f)
							if err != nil {
								return fmt.Errorf("checking SD file %q: %w", file, err)
							}
						}
						continue
					}
					return fmt.Errorf("file %q for file_sd in scrape job %q does not exist", file, sc.JobName)
				}
			}
		}
	}
	return nil
}

func (cfg *Config) validateTargetAllocatorConfig() error {
	// validate targetAllocator
	targetAllocatorConfig := cfg.TargetAllocator
	if targetAllocatorConfig == nil {
		return nil
	}
	// ensure valid endpoint
	if _, err := url.ParseRequestURI(targetAllocatorConfig.Endpoint); err != nil {
		return fmt.Errorf("TargetAllocator endpoint is not valid: %s", targetAllocatorConfig.Endpoint)
	}
	// ensure valid collectorID without variables
	if targetAllocatorConfig.CollectorID == "" || strings.Contains(targetAllocatorConfig.CollectorID, "${") {
		return fmt.Errorf("CollectorID is not a valid ID")
	}

	return nil
}

// Unmarshal a config.Parser into the config struct.
func (cfg *Config) Unmarshal(componentParser *confmap.Conf) error {
	if componentParser == nil {
		return nil
	}
	// We need custom unmarshaling because prometheus "config" subkey defines its own
	// YAML unmarshaling routines so we need to do it explicitly.

	err := componentParser.Unmarshal(cfg, confmap.WithErrorUnused())
	if err != nil {
		return fmt.Errorf("prometheus receiver failed to parse config: %w", err)
	}

	// Unmarshal prometheus's config values. Since prometheus uses `yaml` tags, so use `yaml`.
	promCfg, err := componentParser.Sub(prometheusConfigKey)
	if err != nil || len(promCfg.ToStringMap()) == 0 {
		return err
	}
	out, err := yaml.Marshal(promCfg.ToStringMap())
	if err != nil {
		return fmt.Errorf("prometheus receiver failed to marshal config to yaml: %w", err)
	}

	err = yaml.UnmarshalStrict(out, &cfg.PrometheusConfig)
	if err != nil {
		return fmt.Errorf("prometheus receiver failed to unmarshal yaml to prometheus config: %w", err)
	}

	// Unmarshal targetAllocator configs
	targetAllocatorCfg, err := componentParser.Sub(targetAllocatorConfigKey)
	if err != nil {
		return err
	}
	targetAllocatorHTTPSDCfg, err := targetAllocatorCfg.Sub(targetAllocatorHTTPSDConfigKey)
	if err != nil {
		return err
	}

	targetAllocatorHTTPSDMap := targetAllocatorHTTPSDCfg.ToStringMap()
	if len(targetAllocatorHTTPSDMap) != 0 {
		targetAllocatorHTTPSDMap["url"] = "http://placeholder" // we have to set it as else the marshal will fail
		httpSDConf, err := yaml.Marshal(targetAllocatorHTTPSDMap)
		if err != nil {
			return fmt.Errorf("prometheus receiver failed to marshal config to yaml: %w", err)
		}
		err = yaml.UnmarshalStrict(httpSDConf, &cfg.TargetAllocator.HTTPSDConfig)
		if err != nil {
			return fmt.Errorf("prometheus receiver failed to unmarshal yaml to prometheus config: %w", err)
		}
	}

	return nil
}
