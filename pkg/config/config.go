package config

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/spf13/viper"
)

func Load() {
	loadConfig()
	setDefaultConfigs()
}

func ValidateConfig() error {
	keys := viper.GetViper().AllKeys()
	missingKeys := []string{}
	for _, v := range []string{
		"logs.level",
		"subscription.name",
		"subscription.subscriber",
		"subscription.inventory_query",
		"subscription.sync_cron",
		"workers.count_inbound_workers",
		"workers.count_outbound_workers",
		"batching.time_window_ms",
		"batching.max_count_elements",
		"api.port",
		"monitoring.console_cron",
	} {
		if !slices.Contains(keys, v) {
			missingKeys = append(missingKeys, v)
		}
	}
	if len(missingKeys) > 0 {
		return fmt.Errorf("one or more required Configuration Keys not found: %s", strings.Join(missingKeys, ", "))
	}
	for k, v := range viper.AllSettings() {
		if v == nil || len(fmt.Sprintf("%v", v)) == 0 {
			return fmt.Errorf("configuration key '%s' has an empty value", k)
		}
	}
	return nil
}

func PrintConfigValues() {
	slog.Info("Application configuration", "cfg", strings.Join(GetRedactedConfigValues(), ", "))
}

func setDefaultConfigs() {
	viper.SetDefault("logs.level", "INFO")

	viper.SetDefault("subscription.name", "AzEventHubIntegration")
	viper.SetDefault("subscription.subscriber", "ehSubscriber")
	viper.SetDefault("subscription.inventory_query", "has(fwdToAzureEventHub) and not has(azureNotificationSubscription)")
	viper.SetDefault("subscription.sync_cron", "*/10 * * * *")

	viper.SetDefault("workers.count_inbound_workers", 3)
	viper.SetDefault("workers.count_outbound_workers", 3)

	viper.SetDefault("batching.time_window_ms", 1000)
	viper.SetDefault("batching.max_count_elements", 25)

	viper.SetDefault("api.port", 80)

	viper.SetDefault("monitoring.console_cron", "@every 30s")
}

func loadConfig() {
	viper.SetConfigName("config.toml")
	viper.SetConfigType("toml")

	viper.AddConfigPath("/etc/az-eventhub-connector/")
	viper.AddConfigPath("$HOME/.az-eventhub-connector")
	viper.AddConfigPath(".")

	err := viper.ReadInConfig()

	if err != nil {
		slog.Error("Error loading config file", "err", err)
	}
}

func GetRedactedConfigValues() []string {
	allKeys := viper.AllKeys()
	// redact confidential setttings
	res := make([]string, 0)
	// value of all configurations where key contains one of below will be redacted
	redactKeywords := []string{"secret", "pass", "access", "key", "token"}
	for _, key := range allKeys {
		newValue := fmt.Sprintf("%v", viper.Get(key))

		lowerKey := strings.ToLower(key)
		for _, kw := range redactKeywords {
			if strings.Contains(lowerKey, kw) {
				newValue = "redacted"
			}
		}
		res = append(res, fmt.Sprintf("%s=%v", key, newValue))
	}
	return res
}
