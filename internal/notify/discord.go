package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	discordColorGreen = 3066993  // success
	discordColorRed   = 15158332 // failure
)

// DiscordNotifier sends rotation events to a Discord webhook.
type DiscordNotifier struct {
	webhookURL string
}

type discordPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

type discordEmbed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       int    `json:"color"`
	Timestamp   string `json:"timestamp"`
}

// Send posts a Discord-formatted webhook payload.
func (d *DiscordNotifier) Send(ctx context.Context, event Event) error {
	color := discordColorGreen
	if event.Status == "failed" {
		color = discordColorRed
	}

	payload := discordPayload{
		Embeds: []discordEmbed{
			{
				Title:       fmt.Sprintf("Secret Rotation: %s", event.Status),
				Description: fmt.Sprintf("%s: %s", event.SecretName, event.Details),
				Color:       color,
				Timestamp:   event.Timestamp.Format(time.RFC3339),
			},
		},
	}

	return postJSON(ctx, d.webhookURL, payload)
}

func postJSON(ctx context.Context, url string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("sending webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}
