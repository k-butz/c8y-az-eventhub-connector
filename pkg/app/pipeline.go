package app

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs"
	"github.com/destel/rill"
	util "github.com/k-butz/az-eventhub-connector/pkg/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/reubenmiller/go-c8y/pkg/c8y"
	"github.com/reubenmiller/go-c8y/pkg/c8y/notification2"
	"github.com/tidwall/gjson"
)

type Pipeline struct {
	Id              string
	C8YClient       *c8y.Client
	AzureClient     *azeventhubs.ProducerClient
	SubscriptionCfg SubscriptionConfig
	InboundChan     chan notification2.Message
	InboundCfg      InboundProcessingConfig
	OutboundChan    chan gjson.Result
	OutboundCfg     OutboundProcessingConfig
}

type SubscriptionConfig struct {
	Subscription string
	Subscriber   string
	Consumer     string
}

type InboundProcessingConfig struct {
	WorkerCount int
}

type OutboundProcessingConfig struct {
	WorkerCount        int
	BatchMaxIntervalMs int
	BatchSizeMax       int
}

var (
	measurementsForwarded = promauto.NewCounter(prometheus.CounterOpts{
		Name: "fwd_measurements_total",
		Help: "The total number of forwarded measurements",
	})
)

func (p *Pipeline) Run() {
	c8y.SilenceLogger()
	// start in- and outbound routines in background
	go inboundProcessing(p)
	slog.Info("Set up notification processing (inbound)", "pipeline", p.Id)
	go outboundProcessing(p)
	slog.Info("Set up azure processing (outbound)", "pipeline", p.Id)

	// subscribe to live data
	token, err := GenerateMeasurementToken(p.C8YClient, p.SubscriptionCfg.Subscription, p.SubscriptionCfg.Subscriber)
	if err != nil {
		slog.Error("Error while generating token for Notification Subscription", "err", err, "pipeline", p.Id)
	}
	notificationClient, err := p.C8YClient.Notification2.CreateClient(context.Background(), c8y.Notification2ClientOptions{
		Token:    token,
		Consumer: p.SubscriptionCfg.Consumer,
	})
	if err != nil {
		slog.Error("Error while creating notification subscription. This is a fatal error, exiting now.", "pipeline", p.Id, "error", err.Error())
		util.Shutdown(60, 1)
	}
	notification2.SetLogger(nil)
	slog.Info("Start listening to Live Data", "pipeline", p.Id)
	err = notificationClient.Connect()
	if err != nil {
		util.Shutdown(0, 2)
	}
	ch := make(chan notification2.Message)
	notificationClient.Register("*", ch)

	// Enable ctrl-c stop signal
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)

	for {
		select {
		case msg := <-ch:
			// for now, acknowledge the message on receival
			// Alternatively, we could acknowledge it once submitted to Azure
			if err := notificationClient.SendMessageAck(msg.Identifier); err != nil {
				slog.Warn("Failed to send message ack", "msg", msg, "error", err)
			}
			// send data to inbound channel
			p.InboundChan <- msg
		case <-signalCh:
			// Enable ctrl-c to stop
			log.Printf("Stopping client")
			notificationClient.Close()
			return
		}
	}
}

// listens to all messages on inbound channel
// processes each message with a worker pool
func inboundProcessing(p *Pipeline) {
	msgs := rill.FromChan(p.InboundChan, nil)
	rill.ForEach(msgs, p.InboundCfg.WorkerCount, func(msg notification2.Message) error {
		slog.Debug("received message",
			"consumer", p.SubscriptionCfg.Consumer,
			"pipeline", p.Id,
			"action", msg.Action,
			"description", msg.Description,
			"identifier", msg.Identifier,
		)

		// convert raw json string to gjson
		// gjson is a fast, convenient lib in Go to work with JSONs https://github.com/tidwall/gjson
		gData := gjson.Parse(string(msg.Payload))

		// send message to outbound/azure channel
		if isInPipelineScope(&gData) {
			p.OutboundChan <- gData
		}

		return nil
	})
}

func outboundProcessing(p *Pipeline) {
	elements := rill.FromChan(p.OutboundChan, nil)
	idBatches := rill.Batch(elements, p.OutboundCfg.BatchSizeMax, time.Duration(p.OutboundCfg.BatchMaxIntervalMs)*time.Millisecond)
	contentType := "application/json"
	_ = rill.ForEach(idBatches, p.OutboundCfg.WorkerCount, func(c8yBatch []gjson.Result) error {
		slog.Debug("Processing outbound batch", "batchSize", len(c8yBatch))

		// used to measure the execution time
		start := time.Now()

		batches := []*azeventhubs.EventDataBatch{}

		// convert to Azure Event Batch
		azBatch, err := p.AzureClient.NewEventDataBatch(context.Background(), nil)
		if err != nil {
			slog.Error("Error while creating Azure Event Data Batch. Skipping this batch of data.", "err", err)
			return nil
		}
		batches = append(batches, azBatch)
		currentBatch := azBatch
		for _, c8yElement := range c8yBatch {
			messageId := c8yElement.Get("id").String()
			azElement := azeventhubs.EventData{
				// required
				Body: []byte(c8yElement.Str),
				// optional
				Properties:  map[string]any{"origin": "cumulocity"},
				ContentType: &contentType,
				MessageID:   &messageId,
			}
			if err := currentBatch.AddEventData(&azElement, nil); err != nil {
				// Azure Event Hub has a limit of 1 MB per batch
				// Code is running in below block when an element doesn't fit into current batch
				// => register new batch and add element there
				if err == azeventhubs.ErrEventDataTooLarge {
					slog.Debug("Element did not fit into current Azure Batch anymore. Creating new batch.")
					b, err := p.AzureClient.NewEventDataBatch(context.Background(), nil)
					if err != nil {
						slog.Error("Error while creating new azure batch", "err", err)
						continue
					}
					batches = append(batches, b)
					currentBatch = b
					if err := currentBatch.AddEventData(&azElement, nil); err != nil {
						slog.Error("Error while adding c8y element to new azure event hub batch",
							"err", err, "c8yElementRaw", c8yElement.Raw)
						continue
					}
					// all other errors
				} else {
					slog.Error("Error while adding c8y element to current azure event hub batch",
						"err", err, "c8yElementRaw", c8yElement.Raw)
					continue
				}
			}
		}

		sendBatches(p.AzureClient, batches)

		slog.Info("Sent all batches towards Azure Event Hub", "ctBatches", len(batches), "durationMs", time.Since(start).Milliseconds())

		return nil
	})
}

func sendBatches(client *azeventhubs.ProducerClient, azBatches []*azeventhubs.EventDataBatch) {
	for _, azBatch := range azBatches {
		if err := client.SendEventDataBatch(context.Background(), azBatch, nil); err != nil {
			slog.Error("Error while sending batch towards Azure", "err", err)
			continue
		}
		slog.Info("Sent data batch towards configured Azure Event Hub", "ctElements", azBatch.NumEvents())
		measurementsForwarded.Add(float64(azBatch.NumEvents()))
	}
}

// placeholder function in case you want to filter in/out messages that should be sent to Azure
func isInPipelineScope(element *gjson.Result) bool {
	// Tip, if you want to extract certain keys to do checks, you can do:
	// srcId := element.Get("source.id").String()
	return true
}
