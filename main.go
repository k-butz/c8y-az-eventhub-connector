package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	_ "github.com/dimiro1/banner/autoload"
	"github.com/joho/godotenv"
	api "github.com/k-butz/az-eventhub-connector/pkg/api"
	"github.com/k-butz/az-eventhub-connector/pkg/app"
	cfg "github.com/k-butz/az-eventhub-connector/pkg/config"
	mon "github.com/k-butz/az-eventhub-connector/pkg/monitoring"
	util "github.com/k-butz/az-eventhub-connector/pkg/util"
	"github.com/spf13/viper"
)

func main() {
	// load config
	cfg.Load()
	cfg.ConfigureLogger()
	cfgFile := viper.ConfigFileUsed()
	slog.Info(fmt.Sprintf("Used config file: %s", cfgFile))
	cfg.PrintConfigValues()
	if err := cfg.ValidateConfig(); err != nil {
		slog.Error("Configuration has validation errors. This is a fatal error, exiting now", "err", err)
		util.Shutdown(60, 1)
	}

	// load environment variables
	loadEnvFile()
	slog.Info("Environment variables", "envVars", getEnvVarsRedacted())
	requiredEnvVars := []string{"C8Y_BASEURL", "C8Y_TENANT", "C8Y_USER", "C8Y_PASSWORD"}
	for _, k := range requiredEnvVars {
		if v := os.Getenv(k); len(v) == 0 {
			slog.Error("At least one required environment variable is missing. This is a fatal error, exiting now.", "requiredEnvVars", requiredEnvVars)
			util.Shutdown(60, 1)
		}
	}

	// start webserver, monitoring and the business logic
	api.StartWebServer()
	mon.StartResourceLogger()
	app.Run()

	// keep main routine alive
	select {}
}

func loadEnvFile() {
	err := godotenv.Load()
	if err != nil {
		slog.Info(".env file not present (env-File is optional, this is not considered as error)")
	}
}

func getEnvVarsRedacted() []string {
	allEnvs := os.Environ()
	res := make([]string, 0)
	redactKeywords := []string{"secret", "pass", "access", "key", "token"}
	for _, env := range allEnvs {
		newValue := env
		lowerKey := strings.ToLower(env)
		// redact env var if needed
		for _, kw := range redactKeywords {
			if strings.Contains(lowerKey, kw) {
				elements := strings.Split(env, "=")
				newValue = fmt.Sprintf("%s=redacted", elements[0])
			}
		}
		res = append(res, newValue)
	}
	return res
}
