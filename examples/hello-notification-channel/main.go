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
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime"
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
	if !validToken(req.GetConfig().GetFields()["access_token"].GetStringValue()) {
		return &pluginv1.ValidateNotificationDestinationResponse{
			Message: "The access token can only contain printable ASCII characters.",
		}, nil
	}
	if !validServerURL(c.server()) {
		return &pluginv1.ValidateNotificationDestinationResponse{
			Message: "The administrator's ntfy server URL isn't valid. Ask them to fix it in the plugin's settings.",
		}, nil
	}
	return &pluginv1.ValidateNotificationDestinationResponse{Valid: true}, nil
}

// validServerURL reports whether raw is an absolute http or https URL, the
// only kind the HTTP client can post to.
func validServerURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// validToken reports whether token can go in an HTTP header unchanged.
func validToken(token string) bool {
	return !strings.ContainsFunc(token, func(r rune) bool { return r < 0x20 || r > 0x7e })
}

// headerValue makes text safe for an HTTP header: Go refuses control
// characters, so they become spaces, and ntfy reads non-ASCII text only
// RFC 2047 encoded.
func headerValue(text string) string {
	text = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, text)
	return mime.BEncoding.Encode("utf-8", strings.TrimSpace(text))
}

func (c *channel) SendNotification(ctx context.Context, req *pluginv1.SendNotificationRequest) (*pluginv1.SendNotificationResponse, error) {
	fields := req.GetConfig().GetFields()
	topic := fields["topic"].GetStringValue()
	token := fields["access_token"].GetStringValue()
	if !topicPattern.MatchString(topic) || !validToken(token) {
		return outcome(pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_DESTINATION_INVALID, "The topic or access token isn't valid."), nil
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
	httpReq.Header.Set("Title", headerValue(msg.GetTitle()))
	if msg.GetUrl() != "" {
		httpReq.Header.Set("Click", headerValue(msg.GetUrl()))
	}
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		// Network errors are temporary; the host retries.
		return outcome(pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_RETRY, "Couldn't reach the ntfy server."), nil
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	switch {
	case resp.StatusCode < 300:
		return outcome(pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_DELIVERED, ""), nil
	case resp.StatusCode < 400:
		// The client doesn't follow redirects: a redirect, such as to a
		// proxy's login page, would turn the POST into a GET and its 200
		// would look like a delivery.
		return outcome(pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_RETRY, fmt.Sprintf("The ntfy server redirected the message (%d).", resp.StatusCode)), nil
	case refusedByNtfy(resp.StatusCode, respBody):
		return outcome(pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_DESTINATION_INVALID, "ntfy refused the access token for this topic."), nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		// Not ntfy's own answer, so maybe a proxy in front of it: retry
		// rather than disable a destination that may be fine.
		return outcome(pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_RETRY, fmt.Sprintf("The ntfy server answered %d.", resp.StatusCode)), nil
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		out := outcome(pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_RETRY, fmt.Sprintf("ntfy answered %d.", resp.StatusCode))
		// Pass ntfy's wait through unshortened; the host applies its own cap.
		if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && seconds > 0 {
			out.RetryAfterSeconds = int32(min(seconds, math.MaxInt32))
		}
		return out, nil
	default:
		return outcome(pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_FAILED, fmt.Sprintf("ntfy answered %d.", resp.StatusCode)), nil
	}
}

// refusedByNtfy reports whether a 401 or 403 is ntfy's own refusal of the
// topic's credentials, which comes as JSON such as
// {"code":40101,"http":401,"error":"unauthorized"}. A proxy or CDN in front
// of ntfy answers differently, and only ntfy's answer proves the
// destination's settings wrong.
func refusedByNtfy(status int, body []byte) bool {
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		return false
	}
	var ntfyErr struct {
		Code int `json:"code"`
		HTTP int `json:"http"`
	}
	return json.Unmarshal(body, &ntfyErr) == nil && ntfyErr.HTTP == status && ntfyErr.Code/100 == status
}

func outcome(o pluginv1.NotificationSendOutcome, message string) *pluginv1.SendNotificationResponse {
	return &pluginv1.SendNotificationResponse{Outcome: o, Message: message}
}

// newHTTPClient returns a client that doesn't follow redirects, so
// SendNotification sees them.
func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func main() {
	ntfy := &channel{http: newHTTPClient(), serverURL: defaultServerURL}
	sdkruntime.ServeManifestWithOptions(manifestJSON, version, sdkruntime.CapabilityServers{},
		sdkruntime.WithConfigure(ntfy.configure),
		sdkruntime.WithNotificationChannel(ntfy),
	)
}
