package httproute_test

import (
	"testing"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/httproute"
)

func TestViewerOf(t *testing.T) {
	for _, tc := range []struct {
		name    string
		headers map[string]string
		want    httproute.Viewer
		wantOK  bool
	}{
		{name: "public route", headers: map[string]string{"Accept": "text/html"}},
		{name: "malformed user id", headers: map[string]string{"X-Silo-User-Id": "seven"}},
		{name: "zero user id", headers: map[string]string{"X-Silo-User-Id": "0"}},
		{
			name: "admin on a primary profile",
			headers: map[string]string{
				"X-Silo-User-Id": "7", "X-Silo-User-Role": "admin", "X-Silo-User-Name": "josh",
				"X-Silo-Profile-Id": "p1", "X-Silo-Profile-Name": "Josh", "X-Silo-Profile-Primary": "true",
				"X-Silo-Theme": "dark",
			},
			want:   httproute.Viewer{UserID: 7, Admin: true, UserName: "josh", ProfileID: "p1", ProfileName: "Josh", ProfilePrimary: true, Theme: "dark"},
			wantOK: true,
		},
		{
			// A proxy or test client may change the case of header names.
			name:    "user without a profile, lowercase names",
			headers: map[string]string{"x-silo-user-id": " 12 ", "x-silo-user-role": "user"},
			want:    httproute.Viewer{UserID: 12},
			wantOK:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := httproute.ViewerOf(&pluginv1.HandleHTTPRequest{Headers: tc.headers})
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("ViewerOf() = %+v, %v; want %+v, %v", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
