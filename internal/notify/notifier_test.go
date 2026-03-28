package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDispatcherFansOutToAllNotifiers(t *testing.T) {
	var mu sync.Mutex
	var called []string

	n1 := &fakeNotifier{sendFn: func(ctx context.Context, e Event) error {
		mu.Lock()
		called = append(called, "n1")
		mu.Unlock()
		return nil
	}}
	n2 := &fakeNotifier{sendFn: func(ctx context.Context, e Event) error {
		mu.Lock()
		called = append(called, "n2")
		mu.Unlock()
		return nil
	}}

	d := NewDispatcher(n1, n2)
	err := d.Send(context.Background(), Event{
		SecretName: "db_pass",
		Status:     "success",
		Details:    "rotated",
		Timestamp:  time.Now(),
	})

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"n1", "n2"}, called)
}

func TestDiscordNotifierFormatsPayload(t *testing.T) {
	var received map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	n := &DiscordNotifier{webhookURL: srv.URL}
	evt := Event{
		SecretName: "db_pass",
		Status:     "success",
		Details:    "rotated OK",
		Timestamp:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	err := n.Send(context.Background(), evt)
	require.NoError(t, err)

	embeds := received["embeds"].([]interface{})
	require.Len(t, embeds, 1)
	embed := embeds[0].(map[string]interface{})
	assert.Contains(t, embed["title"], "success")
	assert.Contains(t, embed["description"], "db_pass")
	// Green color for success
	assert.Equal(t, float64(3066993), embed["color"])

	// Test failure color
	var received2 map[string]interface{}
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received2)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv2.Close()

	n2 := &DiscordNotifier{webhookURL: srv2.URL}
	evt2 := Event{SecretName: "api_key", Status: "failed", Details: "timeout", Timestamp: time.Now()}
	err = n2.Send(context.Background(), evt2)
	require.NoError(t, err)

	embeds2 := received2["embeds"].([]interface{})
	embed2 := embeds2[0].(map[string]interface{})
	assert.Equal(t, float64(15158332), embed2["color"])
}

func TestSlackNotifierFormatsPayload(t *testing.T) {
	var received map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := &SlackNotifier{webhookURL: srv.URL}
	evt := Event{
		SecretName: "db_pass",
		Status:     "success",
		Details:    "rotated OK",
		Timestamp:  time.Now(),
	}
	err := n.Send(context.Background(), evt)
	require.NoError(t, err)

	assert.Contains(t, received["text"], "db_pass")
	blocks := received["blocks"].([]interface{})
	require.NotEmpty(t, blocks)
	block := blocks[0].(map[string]interface{})
	assert.Equal(t, "section", block["type"])
}

func TestGenericNotifierSendsJSON(t *testing.T) {
	var received map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := &GenericNotifier{webhookURL: srv.URL}
	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	evt := Event{SecretName: "api_key", Status: "success", Details: "done", Timestamp: ts}
	err := n.Send(context.Background(), evt)
	require.NoError(t, err)

	assert.Equal(t, "api_key", received["secret_name"])
	assert.Equal(t, "success", received["status"])
	assert.Equal(t, "done", received["details"])
	assert.NotEmpty(t, received["timestamp"])
}

func TestDispatcherEmptyIsNoop(t *testing.T) {
	d := NewDispatcher()
	err := d.Send(context.Background(), Event{SecretName: "x", Status: "success"})
	assert.NoError(t, err)
}

func TestDispatcherCollectsAllErrors(t *testing.T) {
	n1 := &fakeNotifier{sendFn: func(ctx context.Context, e Event) error {
		return errors.New("n1 failed")
	}}
	n2 := &fakeNotifier{sendFn: func(ctx context.Context, e Event) error {
		return errors.New("n2 failed")
	}}

	d := NewDispatcher(n1, n2)
	err := d.Send(context.Background(), Event{SecretName: "x", Status: "failed"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "n1 failed")
	assert.Contains(t, err.Error(), "n2 failed")
}

// fakeNotifier is a test helper implementing Notifier.
type fakeNotifier struct {
	sendFn func(ctx context.Context, e Event) error
}

func (f *fakeNotifier) Send(ctx context.Context, e Event) error {
	return f.sendFn(ctx, e)
}
