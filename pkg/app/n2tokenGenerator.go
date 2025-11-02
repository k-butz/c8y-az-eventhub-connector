package app

import (
	"context"
	"errors"

	"github.com/reubenmiller/go-c8y/pkg/c8y"
)

type Notification2TokenOptions struct {
	Subscriber   string `json:"subscriber,omitempty"`
	Subscription string `json:"subscription,omitempty"`
}

func GenerateMeasurementToken(c *c8y.Client, subscription string, subscriber string) (string, error) {
	data := new(c8y.Notification2Token)
	resp, err := c.SendRequest(context.Background(), c8y.RequestOptions{
		Method: "POST",
		Path:   "notification2/token",
		Body: Notification2TokenOptions{
			Subscription: subscription,
			Subscriber:   subscriber,
		},
		ResponseData: data,
	})
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", errors.New("server response is nil")
	}
	return data.Token, nil
}
