package main

import (
	"flag"
	"fmt"
	"github.com/spf13/pflag"
	"net/url"
	"os"
	"sort"
	"strings"

	networksConfig "github.com/grafana/agent/internal/static/config/networks"

	"github.com/go-kit/log/level"
	"github.com/grafana/agent/internal/boringcrypto"
	"github.com/grafana/agent/internal/build"
	"github.com/grafana/agent/internal/static/config"
	"github.com/grafana/agent/internal/static/server"
	util_log "github.com/grafana/agent/internal/util/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/prometheus/client_golang/prometheus"

	_ "github.com/grafana/loki/clients/pkg/promtail/discovery/consulagent"
	_ "github.com/prometheus/prometheus/discovery/install"

	_ "github.com/grafana/agent/internal/static/integrations/install"
	_ "golang.org/x/crypto/x509roots/fallback"
)

var cmd = &cobra.Command{
	Use:   "telescope",
	Short: "An All-in-One Web3 Observability tooling",
	Long:  `Gain full insights into the performance of your dApps, nodes and onchain events with Telescope.`,
	Run: func(cmd *cobra.Command, args []string) {
		defaultCfg := server.DefaultConfig()
		logger := server.NewLogger(&defaultCfg)

		// Check if config file is specified
		configFile := viper.GetString("config-file")
		if configFile != "" {
			if err := validateConfigFile(configFile); err != nil {
				level.Error(logger).Log("msg", "invalid config file", "err", err)
				os.Exit(1)
			}
			level.Info(logger).Log("msg", "using config file", "path", configFile)
			agent(configFile)
			return
		}

		// Otherwise load config from flags/env
		var config TelescopeConfig
		if err := config.loadConfig(); err != nil {
			level.Error(logger).Log("msg", "failed to load config", "err", err)
			os.Exit(1)
		}

		// Get network config and generate scrape configs
		networkConfig := getNetworkConfig(config.Network)
		level.Info(logger).Log("msg", "starting telescope agent", "network_config", networkConfig)
		scrapeConfigs := networkConfig.GenerateScrapeConfigs(config.ProjectName, config.Network)

		// Generate and write full config
		fullConfig := generateFullConfig(config, scrapeConfigs)
		configFilePath := "telescope_config.yaml"
		if err := writeConfigToFile(fullConfig, configFilePath); err != nil {
			level.Error(logger).Log("msg", "failed to write config file", "path", configFilePath, "err", err)
			os.Exit(1)
		}

		level.Info(logger).Log("msg", "configuration written", "path", configFilePath)
		agent(configFilePath)
	},
}

// getSupportedNetworks returns a sorted list of supported blockchain networks.
func getSupportedNetworks() []string {
	networks := make([]string, 0, len(networkConfigs))
	for network := range networkConfigs {
		networks = append(networks, network)
	}
	sort.Strings(networks)
	return networks
}

var helperFunction = func(cmd *cobra.Command, args []string) {
	defaultCfg := server.DefaultConfig()
	logger := server.NewLogger(&defaultCfg)
	level.Info(logger).Log("msg", "Telescope CLI Help")
	level.Info(logger).Log("msg", "Available flags",
		"metrics", "Enable metrics (bool)",
		"network", fmt.Sprintf("Specify the network (%s)", strings.Join(getSupportedNetworks(), ", ")),
		"project-id", "Specify the project ID",
		"project-name", "Specify the project name",
		"telescope-username", "Specify the telescope username",
		"telescope-password", "Specify the telescope password",
		"remote-write-url", "Specify the remote write URL",
		"config-file", "Specify the config file",
		"enable-logs", "Enable logs (bool)",
		"logs-sink-url", "Specify the Log Sink URL",
		"telescope-loki-username", "Specify the Loki username",
		"telescope-loki-password", "Specify the Loki password",
	)
}

var networkConfigs = map[string]networksConfig.NetworkConfig{
	"ethereum":    networksConfig.NewEthereumConfig(),
	"polkadot":    networksConfig.NewPolkadotConfig(),
	"hyperbridge": networksConfig.NewHyperbridgeConfig(),
	"ssv":         networksConfig.NewSSVConfig(),
}

type Config struct {
	Server       ServerConfig           `yaml:"server"`
	Metrics      MetricsConfig          `yaml:"metrics"`
	Logs         LogsConfig             `yaml:"logs"`
	Integrations map[string]interface{} `yaml:"integrations"`
}

type ServerConfig struct {
	LogLevel string `yaml:"log_level"`
}

type MetricsConfig struct {
	Global        GlobalConfig   `yaml:"global"`
	Wal_Directory string         `yaml:"wal_directory"`
	Configs       []MetricConfig `yaml:"configs"`
}

type GlobalConfig struct {
	ScrapeInterval string            `yaml:"scrape_interval"`
	ExternalLabels map[string]string `yaml:"external_labels"`
	RemoteWrite    []RemoteWrite     `yaml:"remote_write"`
}

type MetricConfig struct {
	Name          string         `yaml:"name"`
	HostFilter    bool           `yaml:"host_filter"`
	ScrapeConfigs []ScrapeConfig `yaml:"scrape_configs"`
}

type ScrapeConfig struct {
	JobName       string         `yaml:"job_name"`
	StaticConfigs []StaticConfig `yaml:"static_configs"`
}

type BasicAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type IntegrationsConfig struct {
	Agent        ToIntegrate `yaml:"agent"`
	NodeExporter ToIntegrate `yaml:"node_exporter"`
}

type ToIntegrate struct {
	Enabled bool `yaml:"enabled"`
}

type StaticConfig struct {
	Targets []string `yaml:"targets"`
}

type RemoteWrite struct {
	URL       string    `yaml:"url"`
	BasicAuth BasicAuth `yaml:"basic_auth"`
}

type LogsConfig struct {
	Configs []LogConfig `yaml:"configs"`
}

type LogConfig struct {
	Name      string      `yaml:"name"`
	Clients   []LogClient `yaml:"clients"`
	Positions Positions   `yaml:"positions"`
}

type Positions struct {
	Filename string `yaml:"filename"`
}

type LogClient struct {
	URL            string            `yaml:"url"`
	BasicAuth      BasicAuth         `yaml:"basic_auth"`
	ExternalLabels map[string]string `yaml:"external_labels"`
}

type TelescopeConfig struct {
	Metrics           bool
	Logs              bool
	Network           string
	ProjectId         string
	ProjectName       string
	TelescopeUsername string
	TelescopePassword string
	RemoteWriteUrl    string
	LokiUsername      string
	LokiPassword      string
	LogsSinkURL       string
}

func handleErr(err error, msg string) {
	if err != nil {
		defaultCfg := server.DefaultConfig()
		logger := server.NewLogger(&defaultCfg)
		level.Error(logger).Log("msg", msg, "err", err)
		os.Exit(1)
	}
}

func toLowerAndEscape(input string) string {
	lowercase := strings.ToLower(input)
	escaped := url.QueryEscape(lowercase)
	return escaped
}

func getNetworkConfig(network string) networksConfig.NetworkConfig {
	config, exists := networkConfigs[network]
	if !exists {
		supportedNetworks := getSupportedNetworks()
		handleErr(
			fmt.Errorf("unsupported network %q, must be one of: %s",
				network,
				strings.Join(supportedNetworks, ", ")),
			"Invalid network configuration",
		)
	}
	return config
}

// performs basic validation of the TelescopeConfig.
// It checks URLs, required fields, and validates the network selection.
func (c *TelescopeConfig) validate() error {
	// Validate URLs
	if c.RemoteWriteUrl != "" {
		if err := validateURL(c.RemoteWriteUrl, "remote write"); err != nil {
			return err
		}
	}

	if c.LogsSinkURL != "" {
		if err := validateURL(c.LogsSinkURL, "logs sink"); err != nil {
			return err
		}
	}

	// Validate other required fields
	if c.ProjectId == "" {
		return fmt.Errorf("project ID cannot be empty")
	}

	if c.ProjectName == "" {
		return fmt.Errorf("project name cannot be empty")
	}

	// Validate Network is supported
	if _, exists := networkConfigs[c.Network]; !exists {
		supportedNetworks := getSupportedNetworks()
		return fmt.Errorf("unsupported network %q, must be one of: %s",
			c.Network,
			strings.Join(supportedNetworks, ", "))
	}

	return nil
}

// checks if a URL string is valid and properly formatted.
// It verifies that the URL has both a scheme (http/https) and a host.
func validateURL(urlStr, name string) error {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid %s URL: %v", name, err)
	}
	if parsed.Scheme == "" {
		return fmt.Errorf("%s URL must include scheme (http:// or https://)", name)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%s URL must include host", name)
	}
	return nil
}

// checks if the provided config file path is valid,
// exists, and points to a regular file rather than a directory.
func validateConfigFile(configFile string) error {
	if configFile == "" {
		return fmt.Errorf("config file path cannot be empty")
	}

	info, err := os.Stat(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("config file does not exist: %s", configFile)
		}
		return fmt.Errorf("error accessing config file: %w", err)
	}

	if info.IsDir() {
		return fmt.Errorf("config file path points to a directory: %s", configFile)
	}

	return nil
}
// validates the logs-specific configuration.
// It checks that all required fields for log collection are provided when logs are enabled.
func (c *TelescopeConfig) validateLogsConfig() error {
	if !c.Logs {
		return nil // Logs not enabled, no validation needed
	}

	if c.LogsSinkURL == "" {
		return fmt.Errorf("logs-sink-url is required when logs are enabled")
	}

	if c.LokiUsername == "" {
		return fmt.Errorf("telescope-loki-username is required when logs are enabled")
	}

	if c.LokiPassword == "" {
		return fmt.Errorf("telescope-loki-password is required when logs are enabled")
	}

	return nil
}

// validates the metrics-specific configuration.
// It checks that all required fields for metrics collection are provided when metrics are enabled.
func (c *TelescopeConfig) validateMetricsConfig() error {
	if !c.Metrics {
		return nil // Metrics not enabled, no validation needed
	}

	if c.TelescopeUsername == "" {
		return fmt.Errorf("telescope-username is required when metrics are enabled")
	}

	if c.TelescopePassword == "" {
		return fmt.Errorf("telescope-password is required when metrics are enabled")
	}

	if c.RemoteWriteUrl == "" {
		return fmt.Errorf("remote-write-url is required when metrics are enabled")
	}

	return nil
}

// generates the complete agent configuration from the provided
// TelescopeConfig and network scrape configurations.
func generateFullConfig(config TelescopeConfig, networkScrapeConfigs []networksConfig.ScrapeConfig) Config {
	scrapeConfigs := make([]ScrapeConfig, len(networkScrapeConfigs))
	for i, nsc := range networkScrapeConfigs {
		scrapeConfigs[i] = ScrapeConfig{
			JobName: nsc.JobName,
			StaticConfigs: []StaticConfig{
				{
					Targets: nsc.StaticConfigs[0].Targets,
				},
			},
		}
	}

	cfg := Config{
		Server: ServerConfig{
			LogLevel: "info",
		},
		Metrics: MetricsConfig{
			Wal_Directory: "/tmp/telescope",
			Global: GlobalConfig{
				ScrapeInterval: "15s",
				ExternalLabels: map[string]string{
					"project_id":   config.ProjectId,
					"project_name": config.ProjectName,
				},
				RemoteWrite: []RemoteWrite{
					{
						URL: config.RemoteWriteUrl,
						BasicAuth: BasicAuth{
							Username: config.TelescopeUsername,
							Password: config.TelescopePassword,
						},
					},
				},
			},
			Configs: []MetricConfig{
				{
					Name:          toLowerAndEscape(config.ProjectName + "_" + config.Network + "_metrics"),
					HostFilter:    false,
					ScrapeConfigs: scrapeConfigs,
				},
			},
		},
		Integrations: map[string]interface{}{
			"agent":         ToIntegrate{Enabled: false},
			"node_exporter": ToIntegrate{Enabled: true},
		},
	}

	if config.Logs {
		cfg.Logs = LogsConfig{
			Configs: []LogConfig{
				{
					Name: "telescope_logs",
					Clients: []LogClient{
						{
							URL: config.LogsSinkURL,
							BasicAuth: BasicAuth{
								Username: config.LokiUsername,
								Password: config.LokiPassword,
							},
							ExternalLabels: map[string]string{
								"project_id":   config.ProjectId,
								"project_name": config.ProjectName,
							},
						},
					},
					Positions: Positions{
						Filename: "/tmp/telescope_logs",
					},
				},
			},
		}
	}

	return cfg
}

// writes the provided Config to a YAML file at the specified path.
func writeConfigToFile(config Config, filePath string) error {
	if filePath == "" {
		return fmt.Errorf("empty file path provided")
	}

	var buf strings.Builder
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)

	if err := encoder.Encode(&config); err != nil {
		return fmt.Errorf("error marshaling to YAML: %v", err)
	}

	if err := os.WriteFile(filePath, []byte(buf.String()), 0644); err != nil {
		return fmt.Errorf("error writing to file: %v", err)
	}

	return nil
}

// loads and validates the agent configuration from command line flags.
// It checks for required flags based on enabled features and validates the configuration.
func (c *TelescopeConfig) loadConfig() error {
	if err := checkRequiredFlags(); err != nil {
		return fmt.Errorf("flag validation error: %w", err)
	}

	// Load values from viper
	c.Metrics = viper.GetBool("metrics")
	c.Logs = viper.GetBool("enable-logs")
	c.Network = viper.GetString("network")
	c.ProjectId = viper.GetString("project-id")
	c.ProjectName = viper.GetString("project-name")
	c.TelescopeUsername = viper.GetString("telescope-username")
	c.TelescopePassword = viper.GetString("telescope-password")
	c.RemoteWriteUrl = viper.GetString("remote-write-url")
	c.LokiUsername = viper.GetString("telescope-loki-username")
	c.LokiPassword = viper.GetString("telescope-loki-password")
	c.LogsSinkURL = viper.GetString("logs-sink-url")

	// Run all validations
	if err := c.validate(); err != nil {
		return fmt.Errorf("config validation error: %w", err)
	}

	if err := c.validateMetricsConfig(); err != nil {
		return fmt.Errorf("metrics configuration error: %w", err)
	}

	if err := c.validateLogsConfig(); err != nil {
		return fmt.Errorf("logs configuration error: %w", err)
	}

	return nil
}

// verifies that all required command line flags are provided.
// Required flags depend on which features (metrics/logs) are enabled.
func checkRequiredFlags() error {
	// Always required
	baseFlags := []string{
		"network",
		"project-id",
		"project-name",
	}

	// Required for metrics
	metricsFlags := []string{
		"telescope-username",
		"telescope-password",
		"remote-write-url",
	}

	// Required for logs
	logsFlags := []string{
		"logs-sink-url",
		"telescope-loki-username",
		"telescope-loki-password",
	}

	missingFlags := []string{}

	// Check base flags
	for _, flag := range baseFlags {
		if viper.GetString(flag) == "" {
			missingFlags = append(missingFlags, flag)
		}
	}

	// Check metrics flags if metrics enabled
	if viper.GetBool("metrics") {
		for _, flag := range metricsFlags {
			if viper.GetString(flag) == "" {
				missingFlags = append(missingFlags, flag)
			}
		}
	}

	// Check logs flags if logs enabled
	if viper.GetBool("enable-logs") {
		for _, flag := range logsFlags {
			if viper.GetString(flag) == "" {
				missingFlags = append(missingFlags, flag)
			}
		}
	}

	if len(missingFlags) > 0 && viper.GetString("config-file") == "" {
		return fmt.Errorf("missing required flags: %s", strings.Join(missingFlags, ", "))
	}

	return nil
}

func init() {
	prometheus.MustRegister(build.NewCollector("agent"))
	cobra.OnInitialize(initConfig)
	cmd.SetHelpFunc(helperFunction)

	// Add and bind flags
	cmd.Flags().String("config-file", "", "Specify the config file") // Changed from StringVar
	cmd.Flags().Bool("metrics", true, "Enable metrics")
	cmd.Flags().Bool("enable-logs", false, "Enable logs")
	cmd.Flags().String("network", "", "Specify the network")
	cmd.Flags().String("project-id", "", "Specify the project ID")
	cmd.Flags().String("project-name", "", "Specify the project name")
	cmd.Flags().String("telescope-username", "", "Specify the telescope username")
	cmd.Flags().String("telescope-password", "", "Specify the telescope password")
	cmd.Flags().String("remote-write-url", "", "Specify the remote write URL")
	cmd.Flags().String("logs-sink-url", "", "Specify the Log Sink URL")
	cmd.Flags().String("telescope-loki-username", "", "Specify the Loki username")
	cmd.Flags().String("telescope-loki-password", "", "Specify the Loki password")

	// Bind all flags to viper
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		viper.BindPFlag(f.Name, f)
	})
}

func initConfig() {
	viper.AutomaticEnv() // read in environment variables that match
}

// initializes and runs the monitoring agent with the provided configuration file.
func agent(configPath string) {
	defaultCfg := server.DefaultConfig()
	logger := server.NewLogger(&defaultCfg)

	reloader := func(log *server.Logger) (*config.Config, error) {
		fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		return config.Load(fs, []string{"-config.file", configPath}, log)
	}

	cfg, err := reloader(logger)
	if err != nil {
		level.Error(logger).Log("msg", "failed to load config", "err", err)
		os.Exit(1)
	}

	logger = server.NewLogger(cfg.Server)
	util_log.Logger = logger

	level.Info(logger).Log("msg", "starting agent", "boringcrypto", boringcrypto.Enabled)
	ep, err := NewEntrypoint(logger, cfg, reloader)
	if err != nil {
		level.Error(logger).Log("msg", "failed to create agent entrypoint", "err", err)
		os.Exit(1)
	}

	if err = ep.Start(); err != nil {
		level.Error(logger).Log("msg", "error running agent", "err", err)
		ep.Stop() // Ensure cleanup happens
		os.Exit(1)
	}

	ep.Stop()
	level.Info(logger).Log("msg", "agent exiting")
}

func main() {
	cobra.OnInitialize(initConfig)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
