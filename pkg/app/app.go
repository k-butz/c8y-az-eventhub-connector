package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs"
	util "github.com/k-butz/az-eventhub-connector/pkg/util"
	"github.com/reubenmiller/go-c8y/pkg/c8y"
	"github.com/reubenmiller/go-c8y/pkg/c8y/notification2"
	"github.com/spf13/viper"
	"github.com/tidwall/gjson"
)

func Run() {
	c8y.SilenceLogger()

	// init c8y client and print user id and roles
	c8yClient := c8y.NewClient(nil, os.Getenv("C8Y_BASEURL"), os.Getenv("C8Y_TENANT"), os.Getenv("C8Y_USER"), os.Getenv("C8Y_PASSWORD"), true)
	slog.Info("Created Cumulocity Client")
	userId, userRoles, err := getUserDetails(c8yClient)
	if err != nil {
		slog.Error("Error while retrieving info for current c8y user. This is a fatal error, exiting now.",
			"err", err, "hint", "do you have all required environemnt variables (C8Y_HOST, C8Y_TENANT, C8Y_USER, C8Y_PASSWORD) present?")
		util.Shutdown(60, 1)
	}
	slog.Info("User details", "userId", userId, "roles", userRoles)

	// read and validate tenant options (app will shutdown if not present)
	category := "azEventHubConnector"
	slog.Info("Requesting tenant options", "category", category)
	opts, err := readTenantOptions(c8yClient, category)
	if err != nil {
		slog.Error("Error while requesting tenant options. This is a fatal error, exiting now.")
		util.Shutdown(60, 1)
	}
	slog.Info(fmt.Sprintf("Found %d elements in tenant options", len(opts)))
	slog.Info("Validating tenant options ...")

	// eventhub name
	ehNameKey := "eventHubName"
	ehNameValue, ok := opts[ehNameKey]
	if !ok || len(ehNameValue) == 0 {
		slog.Error("Eventhub name was not found in tenant options. This is a fatal error, exiting now",
			"tip", fmt.Sprintf("Make sure to have configured a tenant option with category '%s' and key '%s'", category, ehNameKey))
		util.Shutdown(60, 1)
	}
	slog.Info("Found required tenant option for event hub name", "category", category, "key", ehNameKey, "value", ehNameValue)

	// eventhub connection string
	ehConnStrValue := ""
	if len(os.Getenv("EVENTHUB_CONN_STR")) > 0 {
		ehConnStrValue = os.Getenv("EVENTHUB_CONN_STR")
	} else {
		ehConnStrKey := "connectionString"
		ehConnStrValue, ok = opts[ehConnStrKey]
		if !ok || len(ehConnStrValue) == 0 {
			slog.Error("Eventhub connection string was not found in tenant options. This is a fatal error, exiting now",
				"tip", fmt.Sprintf("Make sure to have configured a tenant option with category '%s' and key '%s'", category, ehConnStrKey))
			util.Shutdown(60, 1)
		}
		slog.Info("Found required tenant option for event hub name", "category", category, "key", ehConnStrKey, "value", "redacted")
		requiredPrefix := "Endpoint=sb://"
		if !strings.HasPrefix(ehConnStrValue, requiredPrefix) {
			slog.Error(fmt.Sprintf("Eventhub connection string has been found but does not start with required prefix '%s'. This is a fatal error, exiting now", requiredPrefix),
				"tip", fmt.Sprintf("Make sure your connection string starts with '%s'", requiredPrefix))
			util.Shutdown(60, 1)
		}
	}
	slog.Info("Found all required tenant options", "category", category)

	// init azure client
	slog.Info("Initializing Azure Client ...")
	azClient, err := createAzureClient(ehNameValue, ehConnStrValue)
	if err != nil {
		slog.Error("Error while creating Azure Client. This error is fatal, exiting now", "err", err)
		util.Shutdown(60, 1)
	}
	slog.Info("Created Azure Client")
	slog.Info("Testing connectivity to access configured Event Hub...")
	_, err = azClient.GetEventHubProperties(context.Background(), nil)
	if err != nil {
		slog.Error("Connectivity check for Azure failed. This error is fatal, exiting now", "err", err)
		util.Shutdown(60, 1)
	}
	slog.Info("Connectivity check for Azure succeeded. Setting up Live-Data Pipeline now.")

	// we're all set, lets initialize and start live-data pipeline now
	go startPipeline(c8yClient, azClient)
	go startSubscriptionManager(c8yClient)
}

func startPipeline(c8yClient *c8y.Client, azureClient *azeventhubs.ProducerClient) {
	measurementSubscriptionName := viper.GetString("subscription.name")
	subscriberName := viper.GetString("subscription.subscriber_name")
	p := Pipeline{
		Id:          generateBase64ShortID(),
		C8YClient:   c8yClient,
		AzureClient: azureClient,
		SubscriptionCfg: SubscriptionConfig{
			Subscription: measurementSubscriptionName,
			Subscriber:   subscriberName,
			Consumer:     measurementSubscriptionName + "Consumer",
		},
		InboundChan: make(chan notification2.Message),
		InboundCfg: InboundProcessingConfig{
			WorkerCount: viper.GetInt("workers.count_inbound_workers"),
		},
		OutboundChan: make(chan gjson.Result),
		OutboundCfg: OutboundProcessingConfig{
			WorkerCount:        viper.GetInt("workers.count_outbound_workers"),
			BatchMaxIntervalMs: viper.GetInt("batching.time_window_ms"),
			BatchSizeMax:       viper.GetInt("batching.max_count_elements"),
		},
	}
	p.Run()
	slog.Info("Pipeline created and started")
}

func getUserDetails(c8yClient *c8y.Client) (string, []string, error) {
	user, resp, err := c8yClient.User.GetCurrentUser(context.Background())
	if err != nil {
		return "", nil, err
	}
	if resp == nil {
		return "", nil, errors.New("server response is nil")
	}
	if resp.StatusCode() != http.StatusOK {
		return "", nil,
			fmt.Errorf("unexpected server response status code. Expected: %d. Received: %d", http.StatusOK, resp.StatusCode())
	}
	roleIds := make([]string, 0)
	for _, role := range user.EffectiveRoles {
		roleIds = append(roleIds, role.ID)
	}
	return user.ID, roleIds, nil
}

func readTenantOptions(client *c8y.Client, optionCategory string) (map[string]string, error) {
	opts, resp, err := client.TenantOptions.GetOptionsForCategory(context.Background(), optionCategory)
	if err != nil {
		return map[string]string{}, err
	}
	if resp == nil {
		return map[string]string{}, errors.New("response is nil")
	}
	if resp.StatusCode() != http.StatusOK {
		return map[string]string{},
			fmt.Errorf("unexpected server response status code. Expected: %d. Received: %d", http.StatusOK, resp.StatusCode())
	}
	return opts, nil
}

func createAzureClient(eventhubName, connectionString string) (*azeventhubs.ProducerClient, error) {
	res, err := azeventhubs.NewProducerClientFromConnectionString(
		connectionString,
		eventhubName,
		nil,
	)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func generateBase64ShortID() string {
	randomBytes := make([]byte, 6)
	rand.Read(randomBytes)
	encoded := base64.URLEncoding.EncodeToString(randomBytes)
	return encoded[:8]
}
