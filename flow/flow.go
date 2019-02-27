package flow

import (
	"context"
	"errors"
	"fmt"
	"os"

	"cloud.google.com/go/pubsub"
)

const (
	pubsubGCBTopicID = "cloud-builds"
	pubsubGCRTopicID = "gcr"

	subscriptionPrefix = "flow-sub-"
)

var (
	subscriptionGCB *pubsub.Subscription
	subscriptionGCR *pubsub.Subscription
	client          *pubsub.Client
	cfg             *Config
)

type Flow struct {
	Env           string
	projectID     string
	slackBotToken string
	githubToken   string
}

func New(c *Config) (*Flow, error) {
	cfg = c
	f := &Flow{
		Env:           os.Getenv("FLOW_ENV"),
		projectID:     os.Getenv("FLOW_GCP_PROJECT_ID"),
		slackBotToken: os.Getenv("FLOW_SLACK_BOT_TOKEN"),
		githubToken:   os.Getenv("FLOW_GITHUB_TOKEN"),
	}

	if f.Env == "" || f.projectID == "" || f.slackBotToken == "" || f.githubToken == "" {
		return nil, errors.New("You need to specify a non-empty value for FLOW_ENV, FLOW_GCP_PROJECT_ID, FLOW_SLACK_BOT_TOKEN and FLOW_GITHUB_TOKEN")
	}

	return f, nil
}

func (f *Flow) Start(ctx context.Context, errCh chan error) {
	client, err := newPubSubClient(ctx, f.projectID)
	if err != nil {
		return
	}

	err = createGCBSubscription(ctx, client)
	if err != nil {
		return
	}

	go f.subscribeGCB(ctx, errCh)
}

func newPubSubClient(ctx context.Context, projectID string) (*pubsub.Client, error) {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("Error creating pubsub client: %s", err)
	}
	return client, nil
}

func createGCBSubscription(ctx context.Context, client *pubsub.Client) error {
	topic := client.Topic(pubsubGCBTopicID)
	subscriptionName := fmt.Sprintf("%s%s", subscriptionPrefix+pubsubGCBTopicID)

	subscriptionGCB = client.Subscription(subscriptionName)

	exists, err := subscriptionGCB.Exists(ctx)
	if err != nil {
		return fmt.Errorf("Error checking for subscription: %s", err)
	}
	if !exists {
		if _, err = client.CreateSubscription(ctx, subscriptionName, pubsub.SubscriptionConfig{Topic: topic}); err != nil {
			fmt.Errorf("Failed to create subscription: %s", err)
		}
	}
	return nil
}
