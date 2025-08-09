package app

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/labstack/echo/v4"
	"github.com/reubenmiller/go-c8y/pkg/c8y"
	"github.com/reubenmiller/go-c8y/pkg/c8y/notification2"
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

	// Init C8Y
	slog.Info("Tenant Info", "tenant", application.Client.TenantName)
	serviceUserCtx := application.WithServiceUser(application.Client.TenantName)
	c8yClient := application.Client
	// get api token from cumulocity
	option, _, err := c8yClient.TenantOptions.GetOption(serviceUserCtx, "az-eventhub-connector", "c8y-token")
	if err != nil {
		slog.Error("Error while getting api token from platform", "error", err.Error())
	}
	c8yClient.SetToken(option.Value)

	// Init Event Hub
	azProducerClient, err := azeventhubs.NewProducerClientFromConnectionString("NAMESPACE CONNECTION STRING", "EVENT HUB NAME", nil)
	if err != nil {
		slog.Error("Error while creating azure producer client", "error", err.Error())
	}
	defer azProducerClient.Close(context.TODO())

	// subscribe to notification stream
	notificationClient, err := c8yClient.Notification2.CreateClient(serviceUserCtx, c8y.Notification2ClientOptions{
		Consumer: "eventHubConsumer",
		Options: c8y.Notification2TokenOptions{
			ExpiresInMinutes:  1440,
			Subscriber:        "eventHubConsumer",
			DefaultSubscriber: "eventHubConsumer",
			Subscription:      "AzEventHubHandler",
			Shared:            false,
		},
	})
	if err != nil {
		slog.Error("Error while creating notification subscription", "error", err.Error())
	}

	// connect and send all received messages to channel
	err = notificationClient.Connect()
	if err != nil {
		slog.Error("Error while connecting to notification subscription", "error", err.Error())
	}
	ch := make(chan notification2.Message)
	notificationClient.Register("*", ch)

	// Enable ctrl-c stop signal
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)

	for {
		select {
		case msg := <-ch:
			handleInboundMessage(msg, azProducerClient)
			if err := notificationClient.SendMessageAck(msg.Identifier); err != nil {
				slog.Warn("Failed to send message ack: %s", "error", err)
			}
		case <-signalCh:
			// Enable ctrl-c to stop
			log.Printf("Stopping client")
			notificationClient.Close()
			return
		}
	}
}

func handleInboundMessage(msg notification2.Message, azProducerClient *azeventhubs.ProducerClient) {
	slog.Info("Received message", "msg", msg.Payload)

	// now send to event hub
	if azProducerClient == nil {
		slog.Info("Message was not sent to Eventhub", "reason", "producer client is null")
		return
	}
	batch, err := azProducerClient.NewEventDataBatch(context.TODO(), &azeventhubs.EventDataBatchOptions{})
	test := azeventhubs.EventData{Body: []byte(msg.Payload)}
	batch.AddEventData(&test, nil)
	if err != nil {
		slog.Error("Error while adding data to eventhub message", "error", err.Error())
	}
	err = azProducerClient.SendEventDataBatch(context.TODO(), batch, nil)
	if err != nil {
		slog.Error("Error while sending batch to eventhub", "error", err.Error())
	}
}
