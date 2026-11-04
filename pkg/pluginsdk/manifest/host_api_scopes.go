package manifest

import (
	"fmt"
	"regexp"
)

var (
	// hostAPIScopePattern is the shape of a Silo API key scope:
	// colon-separated lowercase segments, such as "admin:monitor:read".
	hostAPIScopePattern = regexp.MustCompile(`^[a-z0-9-]+(?::[a-z0-9-]+)+$`)
)

// validateHostAPIScopes checks the shape of the manifest's host_api_scopes.
// The vocabulary is the host's, so a scope this SDK doesn't know is valid;
// the host grants only the scopes it knows.
func validateHostAPIScopes(scopes []string) error {
	seen := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		if !hostAPIScopePattern.MatchString(scope) {
			return fmt.Errorf("plugin manifest host_api_scopes: %q is not an API key scope", scope)
		}
		if _, dup := seen[scope]; dup {
			return fmt.Errorf("plugin manifest host_api_scopes: %q is listed twice", scope)
		}
		seen[scope] = struct{}{}
	}
	return nil
}
