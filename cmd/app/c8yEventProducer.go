package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/reubenmiller/go-c8y/pkg/c8y"
)

func produceSampleEventsEndless(ctx context.Context, c8yClient *c8y.Client, moId string, eventType string, sleepTimeSecs int) {
	for {
		_, _, err := c8yClient.Event.Create(ctx, c8y.Event{
			Type:   eventType,
			Text:   "Sample Event for showcasing Event Hub integration",
			Source: &c8y.Source{ID: moId},
			Time:   c8y.NewTimestamp(time.Now()),
		})
		if err != nil {
			slog.Error("Could not create sample Event", "error", err)
		}
		time.Sleep(time.Duration(sleepTimeSecs) * time.Second)
	}
}
