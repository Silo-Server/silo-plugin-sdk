package main

import (
	"context"
	"mime"
	"net/http"
	"net/http/httptest"
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/manifest"
	"google.golang.org/protobuf/types/known/structpb"
)

// TestManifestIsValid keeps the example's destination form within the rules
// hosts enforce at install.
func TestManifestIsValid(t *testing.T) {
	if _, err := manifest.LoadWithChecksum(manifestJSON, "0.1.0"); err != nil {
		t.Fatal(err)
	}
}

// send posts one message through a fake ntfy server and returns the outcome
// and the request the server saw.
func send(t *testing.T, title string, handler http.HandlerFunc) (*pluginv1.SendNotificationResponse, *http.Request) {
	t.Helper()
	var seen *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen == nil {
			seen = r
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	config, err := structpb.NewStruct(map[string]any{"topic": "alerts", "access_token": "tk_1"})
	if err != nil {
		t.Fatal(err)
	}
	c := &channel{http: newHTTPClient(), serverURL: srv.URL}
	resp, err := c.SendNotification(context.Background(), &pluginv1.SendNotificationRequest{
		Config:  config,
		Message: &pluginv1.NotificationMessage{Title: title, Body: "body"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return resp, seen
}

func TestSendNotificationOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		want    pluginv1.NotificationSendOutcome
	}{
		{"ntfy refuses the token", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"code":40301,"http":403,"error":"forbidden"}`))
		}, pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_DESTINATION_INVALID},
		{"a proxy answers 401", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "<html>Sign in</html>", http.StatusUnauthorized)
		}, pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_RETRY},
		{"a redirect to a login page", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/login" {
				return // 200 to a GET: must not count as delivered
			}
			http.Redirect(w, r, "/login", http.StatusFound)
		}, pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_RETRY},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := send(t, "title", tc.handler)
			if resp.GetOutcome() != tc.want {
				t.Fatalf("outcome = %v (%q), want %v", resp.GetOutcome(), resp.GetMessage(), tc.want)
			}
		})
	}
}

func TestSendNotificationTitleHeader(t *testing.T) {
	resp, seen := send(t, "Nouvel épisode\nde Severance", func(http.ResponseWriter, *http.Request) {})
	if resp.GetOutcome() != pluginv1.NotificationSendOutcome_NOTIFICATION_SEND_OUTCOME_DELIVERED {
		t.Fatalf("outcome = %v (%q), want delivered", resp.GetOutcome(), resp.GetMessage())
	}
	title, err := new(mime.WordDecoder).DecodeHeader(seen.Header.Get("Title"))
	if err != nil || title != "Nouvel épisode de Severance" {
		t.Fatalf("Title header decodes to %q (%v)", title, err)
	}
}
