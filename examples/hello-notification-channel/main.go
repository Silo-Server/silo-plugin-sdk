// Command hello-notification-channel is a notification_channel.v1 plugin
// that posts Silo notifications to an ntfy topic (https://ntfy.sh).
//
// The administrator picks the ntfy server in the plugin's settings; each
// destination only names a topic. Users who aren't administrators fill in
// destinations, so letting them choose the URL would let them make the
// server send requests anywhere.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	sdkruntime "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/runtime"
)

//go:embed manifest.json
var manifestJSON []byte

// version replaces the manifest version when set with -ldflags.
var version string

const defaultServerURL = "https://ntfy.sh"

var topicPattern = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)

type channel struct {
	pluginv1.UnimplementedNotificationChannelServer
	http *http.Client

	mu        sync.RWMutex
	serverURL string
}

// configure reads the administrator's server URL.
func (c *channel) configure(_ context.Context, entries []*pluginv1.ConfigEntry) error {
	serverURL := defaultServerURL
	for _, entry := range entries {
		if entry.GetKey() != "ntfy" {
			continue
		}
		if v := strings.TrimSpace(entry.GetValue().GetFields()["server_url"].GetStringValue()); v != "" {
			serverURL = strings.TrimRight(v, "/")
		}
	}
	c.mu.Lock()
	c.serverURL = serverURL
	c.mu.Unlock()
	return nil
}

func (c *channel) server() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.serverURL
}

func (c *channel) ValidateDestination(_ context.Context, req *pluginv1.ValidateNotificationDestinationRequest) (*pluginv1.ValidateNotificationDestinationResponse, error) {
	topic := req.GetConfig().GetFields()["topic"].GetStringValue()
	if !topicPattern.MatchString(topic) {
		return &pluginv1.ValidateNotificationDestinationResponse{
			Message: "The topic must be 1 to 64 letters, digits, hyphens or underscores.",
		}, nil
	}
	if _, err := url.ParseRequestURI(c.server()); err != nil {
		return &pluginv1.ValidateNotificationDestinationResponse{
			Message: "The administrator's ntfy server URL isn't valid. Ask them to fix it in the plugin's settings.",
		}, nil
	}
	return &pluginv1.ValidateNotificationDestinationResponse{Valid: true}, nil
}

func (c *channel) SendNotification(ctx context.Context, req *pluginv1.SendNotificationRequest) (*pluginv1.SendNotificationResponse, error) {
	fields := req.GetConfig().GetFields()
	topic := fields["topic"].GetStringValue()
	if !topicPattern.MatchString(topic) {
		return outcome(pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_DESTINATION_INVALID, "The topic isn't valid."), nil
	}
	msg := req.GetMessage()
	body := msg.GetBody()
	if body == "" {
		body = msg.GetTitle()
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.server()+"/"+topic, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Title", msg.GetTitle())
	if msg.GetUrl() != "" {
		httpReq.Header.Set("Click", msg.GetUrl())
	}
	if token := fields["access_token"].GetStringValue(); token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		// Network errors are temporary; the host retries.
		return outcome(pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_RETRY, "Couldn't reach the ntfy server."), nil
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))

	switch {
	case resp.StatusCode < 300:
		return outcome(pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_DELIVERED, ""), nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return outcome(pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_DESTINATION_INVALID, "ntfy refused the access token for this topic."), nil
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		out := outcome(pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_RETRY, fmt.Sprintf("ntfy answered %d.", resp.StatusCode))
		if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && seconds > 0 {
			out.RetryAfterSeconds = int32(min(seconds, 3600))
		}
		return out, nil
	default:
		return outcome(pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_FAILED, fmt.Sprintf("ntfy answered %d.", resp.StatusCode)), nil
	}
}

func outcome(o pluginv1.NotificationSendOutcome, message string) *pluginv1.SendNotificationResponse {
	return &pluginv1.SendNotificationResponse{Outcome: o, Message: message}
}

func main() {
	ntfy := &channel{http: &http.Client{Timeout: 10 * time.Second}, serverURL: defaultServerURL}
	sdkruntime.ServeManifestWithOptions(manifestJSON, version, sdkruntime.CapabilityServers{},
		sdkruntime.WithConfigure(ntfy.configure),
		sdkruntime.WithNotificationChannel(ntfy),
	)
}
