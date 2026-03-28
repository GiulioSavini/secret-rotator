package notify

import (
	"context"
	"fmt"
)

// SlackNotifier sends rotation events to a Slack webhook.
type SlackNotifier struct {
	webhookURL string
}

type slackPayload struct {
	Text   string       `json:"text"`
	Blocks []slackBlock `json:"blocks"`
}

type slackBlock struct {
	Type string    `json:"type"`
	Text slackText `json:"text"`
}

type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Send posts a Slack-formatted webhook payload.
func (s *SlackNotifier) Send(ctx context.Context, event Event) error {
	payload := slackPayload{
		Text: fmt.Sprintf("Secret Rotation %s: %s", event.Status, event.SecretName),
		Blocks: []slackBlock{
			{
				Type: "section",
				Text: slackText{
					Type: "mrkdwn",
					Text: fmt.Sprintf("*Secret Rotation %s*\n*Secret:* %s\n*Details:* %s",
						event.Status, event.SecretName, event.Details),
				},
			},
		},
	}

	return postJSON(ctx, s.webhookURL, payload)
}

// GenericNotifier sends rotation events as plain JSON to any HTTP endpoint.
type GenericNotifier struct {
	webhookURL string
}

type genericPayload struct {
	SecretName string `json:"secret_name"`
	Status     string `json:"status"`
	Details    string `json:"details"`
	Timestamp  string `json:"timestamp"`
}

// Send posts a generic JSON payload.
func (g *GenericNotifier) Send(ctx context.Context, event Event) error {
	payload := genericPayload{
		SecretName: event.SecretName,
		Status:     event.Status,
		Details:    event.Details,
		Timestamp:  event.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
	}

	return postJSON(ctx, g.webhookURL, payload)
}
