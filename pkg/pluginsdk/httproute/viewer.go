// Package httproute reads what the host tells an http_routes.v1 handler about
// the person making a request.
package httproute

import (
	"strconv"
	"strings"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

// Headers the host sets when it proxies a browser's or app's request to a
// plugin route. The host builds them from its own session and drops any
// X-Silo- header the client sent, so a handler can trust them on those
// requests.
const (
	HeaderUserID         = "X-Silo-User-Id"
	HeaderUserRole       = "X-Silo-User-Role"
	HeaderUserName       = "X-Silo-User-Name"
	HeaderProfileID      = "X-Silo-Profile-Id"
	HeaderProfileName    = "X-Silo-Profile-Name"
	HeaderProfilePrimary = "X-Silo-Profile-Primary"
	HeaderTheme          = "X-Silo-Theme"
)

// Viewer is the signed-in person a request comes from.
type Viewer struct {
	// UserID is the account. Profile IDs repeat across accounts, so key
	// per-profile data by UserID and ProfileID together.
	UserID int
	// Admin is true for an account with the server admin role. It is not
	// ProfilePrimary.
	Admin bool
	// UserName is the account's username, when the host knows it.
	UserName string
	// ProfileID is the household profile the host verified for the request,
	// or empty when none was selected.
	ProfileID string
	// ProfileName is the profile's display name, when the host knows it.
	ProfileName string
	// ProfilePrimary is true for the account's primary (household parent)
	// profile.
	ProfilePrimary bool
	// Theme is the viewer's UI theme, such as "dark", when set.
	Theme string
}

// ViewerOf returns the signed-in viewer of a request. It reports false when
// the request has no signed-in account, as on a public route.
func ViewerOf(req *pluginv1.HandleHTTPRequest) (Viewer, bool) {
	userID, err := strconv.Atoi(Header(req, HeaderUserID))
	if err != nil || userID <= 0 {
		return Viewer{}, false
	}
	return Viewer{
		UserID:         userID,
		Admin:          Header(req, HeaderUserRole) == "admin",
		UserName:       Header(req, HeaderUserName),
		ProfileID:      Header(req, HeaderProfileID),
		ProfileName:    Header(req, HeaderProfileName),
		ProfilePrimary: Header(req, HeaderProfilePrimary) == "true",
		Theme:          Header(req, HeaderTheme),
	}, true
}

// Header returns the request header name, matched case-insensitively, with
// surrounding space trimmed. It returns "" when the header is absent.
func Header(req *pluginv1.HandleHTTPRequest, name string) string {
	headers := req.GetHeaders()
	if value, ok := headers[name]; ok {
		return strings.TrimSpace(value)
	}
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
