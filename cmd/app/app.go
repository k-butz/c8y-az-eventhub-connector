package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/labstack/echo/v4"
	"github.com/reubenmiller/go-c8y/pkg/c8y"
	"github.com/reubenmiller/go-c8y/pkg/microservice"
)

// App represents the http server and c8y microservice application
type App struct {
	echoServer      *echo.Echo
	c8ymicroservice *microservice.Microservice
}

func NewApp() *App {
	app := &App{}
	customHTTPClient := retryablehttp.NewClient()
	opts := microservice.Options{
		HTTPClient: customHTTPClient.StandardClient(),
	}
	c8ymicroservice := microservice.NewDefaultMicroservice(opts)
	customHTTPClient.RetryMax = 2
	customHTTPClient.PrepareRetry = func(req *http.Request) error {
		// Update latest service user credentials
		if username, _, ok := req.BasicAuth(); ok {
			if tenant, username, found := strings.Cut(username, "/"); found {
				for _, serviceUser := range c8ymicroservice.Client.ServiceUsers {
					if serviceUser.Tenant == tenant && serviceUser.Username == username {
						slog.Info("Updating service user credentials for request.", "tenant", tenant, "userID", username)
						req.SetBasicAuth(tenant+"/"+username, serviceUser.Password)
						return nil
					}
				}
			}
		}
		return nil
	}

	customHTTPClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if resp == nil {
			return false, nil
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			return true, nil
		}

		// unauthorized errors can occurs if the service user's credentials are not up to date
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			slog.Info("Service user credentials are invalid, refreshing them.", "statusCode", resp.StatusCode)
			if serviceUsersErr := c8ymicroservice.Client.Microservice.SetServiceUsers(); serviceUsersErr != nil {
				slog.Error("Could not update service users list.", "err", serviceUsersErr)
			} else {
				slog.Info("Updated service users list")
			}
			return true, nil
		}

		if resp.StatusCode == 0 || (resp.StatusCode >= 500 && resp.StatusCode != http.StatusNotImplemented) {
			return true, fmt.Errorf("unexpected HTTP status %s", resp.Status)
		}

		return false, nil
	}

	c8ymicroservice.Config.SetDefault("server.port", "80")
	app.c8ymicroservice = c8ymicroservice
	return app
}

// Run starts the microservice
func (a *App) Run() {
	application := a.c8ymicroservice
	application.Scheduler.Start()

	// Init C8Y client
	slog.Info("Tenant Info", "tenant", application.Client.TenantName)
	serviceUserCtx := application.WithServiceUser(application.Client.TenantName)
	c8yClient := application.Client
	c8yToken, _, err := c8yClient.TenantOptions.GetOption(serviceUserCtx, "az-eventhub-connector", "credentials.c8y-token")
	if err != nil {
		slog.Error("Error while getting api token from platform", "error", err.Error())
	}
	c8yClient.SetToken(c8yToken.Value)

	// Init Event Hub
	// get azure eventhub token and name from tenant options
	azConnectionString, _, err := c8yClient.TenantOptions.GetOption(serviceUserCtx, "az-eventhub-connector", "credentials.az-connection-string")
	if err != nil {
		slog.Warn("Error while getting connection string for event hub", "error", err)
	}
	azEventHubName, _, err := c8yClient.TenantOptions.GetOption(serviceUserCtx, "az-eventhub-connector", "az-eventhub-name")
	if err != nil {
		slog.Warn("Error while getting eventhub name", "errror", err)
	}
	// generate client
	azProducerClient, err := azeventhubs.NewProducerClientFromConnectionString(azConnectionString.Value, azEventHubName.Value, nil)
	if err != nil {
		slog.Error("Error while creating azure producer client", "error", err.Error())
	}
	defer azProducerClient.Close(context.TODO())

	// start simulator client and subscribe to incoming data
	startProducer(serviceUserCtx, c8yClient)
	Subscribe(serviceUserCtx, c8yClient, azProducerClient, "AzEventHubHandler", "c1")
}

func startProducer(ctx context.Context, c8yClient *c8y.Client) {
	// start simulator
	c8yDeviceId, _, err := c8yClient.TenantOptions.GetOption(ctx, "az-eventhub-connector", "c8y-device-id")
	if err != nil {
		slog.Warn("Error while getting c8y device id (used for producing sample data). Won't produce data.", "error", err)
		return
	}
	go produceSampleEventsEndless(ctx, c8yClient, c8yDeviceId.Value, "eventHubDemo", 5)
}
