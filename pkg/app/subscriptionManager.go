package app

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/reubenmiller/go-c8y/pkg/c8y"
	"github.com/robfig/cron/v3"
	"github.com/spf13/viper"
	"github.com/tidwall/gjson"

	cy "github.com/k-butz/az-eventhub-connector/pkg/c8yapi"
)

func startSubscriptionManager(client *c8y.Client) {
	c := cron.New()
	cronExpression := viper.GetString("subscription.sync_cron")
	query := viper.GetString("subscription.inventory_query")
	subscriptionName := viper.GetString("subscription.name")
	fn := func() {
		// first query all Objects for configured query
		mos, err := cy.QueryManagedObjects(client, cy.NewInventoryQueryOptions(
			cy.WithQuery(query),
			cy.WithKeyExtractorFn(func(item gjson.Result) string {
				return item.Get("id").String()
			}),
		))
		if err != nil {
			slog.Error("Error while querying Managed Objects in Subscription Manager", "err", err)
			return
		}
		slog.Info("Subscription Manager queried objects", "countFoundObjects", len(mos))
		if len(mos) == 0 {
			return
		}
		for _, mo := range mos {
			moId := mo.Get("id").String()
			addObjectSubscription(client, moId, subscriptionName)
		}
	}
	// execute immediately
	go fn()
	// schedule with cron expression
	c.AddFunc(cronExpression, fn)
	c.Start()
	slog.Info("Started Subscription Manager", "cronExpression", cronExpression, "query", query)
}

func addObjectSubscription(client *c8y.Client, moId, subscriptionName string) {
	_, resp, err := client.Notification2.CreateSubscription(context.Background(), "",
		c8y.Notification2Subscription{
			Context: "mo",
			Source: &c8y.Source{
				ID: moId,
			},
			Subscription: subscriptionName,
			SubscriptionFilter: c8y.Notification2SubscriptionFilter{
				Apis: []string{"measurements"},
			},
		},
	)
	if err != nil {
		slog.Error("Error while creating notification subscription", "err", err, "moId", moId)
		return
	}
	if resp == nil {
		slog.Error("Error while creating notification subscription", "err", "response is nil", "moId", moId)
		return
	}
	if resp.StatusCode() != http.StatusCreated {
		slog.Error("Error while creating notification subscription",
			"err", "Received unexpected status code",
			"moId", moId,
			"expectedStatusCode", http.StatusCreated,
			"received", resp.StatusCode(),
		)
		return
	}
	slog.Info("Created new Managed Object subscription", "moId", moId, "subscription", subscriptionName)

	// at this point we know the subscription has been created, let's persist this info on the object
	addManagedObjectProperty(client, moId, "azureNotificationSubscription", subscriptionName)
}

func addManagedObjectProperty(client *c8y.Client, moId, fragmentKey, fragmentValue string) {
	_, resp, err := client.Inventory.Update(context.Background(), moId, map[string]string{fragmentKey: fragmentValue})
	if err != nil {
		slog.Error("Error while persisting subscription marker on Managed Object", "err", err)
		return
	}
	if resp == nil {
		slog.Error("Error while persisting subscription marker on Managed Object", "err", "response is nil")
		return
	}
	if resp.StatusCode() != http.StatusOK {
		slog.Error("Error while persisting subscription marker on Managed Object",
			"err", "Received unexpected status code",
			"moId", moId,
			"expectedStatusCode", http.StatusOK,
			"received", resp.StatusCode(),
		)
		return
	}
	slog.Info("Added subscription marker to Managed Object", "moId",
		moId, "fragmentKey", fragmentKey, "fragmentValue", fragmentValue)
}
