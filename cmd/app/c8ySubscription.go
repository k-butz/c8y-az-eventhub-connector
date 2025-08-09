package app

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs"
	"github.com/reubenmiller/go-c8y/pkg/c8y"
	"github.com/reubenmiller/go-c8y/pkg/c8y/notification2"
)

func Subscribe(ctx context.Context, c8yClient *c8y.Client, azProducerClient *azeventhubs.ProducerClient, subscription string, consumer string) {
	notificationClient, err := c8yClient.Notification2.CreateClient(ctx, c8y.Notification2ClientOptions{
		Consumer: consumer,
		Options: c8y.Notification2TokenOptions{
			ExpiresInMinutes:  1440,
			Subscriber:        consumer,
			DefaultSubscriber: "eventHubConsumer",
			Subscription:      subscription,
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
	slog.Info("Sent message to Event Hub")
}
