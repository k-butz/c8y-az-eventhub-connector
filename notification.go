package main

import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"

	"github.com/reubenmiller/go-c8y/pkg/c8y"
	"github.com/reubenmiller/go-c8y/pkg/c8y/notification2"
)

func main2() {
	log.SetOutput(io.Discard)

	client := c8y.NewClientFromEnvironment(nil, true)

	c8yToken := os.Getenv("C8Y_TOKEN")
	if c8yToken != "" {
		client.SetToken(c8yToken)
	}

	notificationClient, err := client.Notification2.CreateClient(context.Background(), c8y.Notification2ClientOptions{
		// Token:    token.Token,
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
		panic(err)
	}

	err = notificationClient.Connect()

	if err != nil {
		panic(err)
	}

	ch := make(chan notification2.Message)
	notificationClient.Register("*", ch)

	// Enable ctrl-c stop signal
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)

	log.Printf("Listening to messages")
	for {
		select {
		case msg := <-ch:
			log.Printf("On message: %s", msg.Payload)
			if err := notificationClient.SendMessageAck(msg.Identifier); err != nil {
				log.Printf("Failed to send message ack: %s", err)
			}
		case <-signalCh:
			// Enable ctrl-c to stop
			log.Printf("Stopping client")
			notificationClient.Close()
			return
		}
	}
}
