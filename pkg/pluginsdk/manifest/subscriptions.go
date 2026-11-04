package manifest

import (
	"fmt"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/capability"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/events"
)

// SubscriptionProblems lists what is wrong with a manifest's event
// subscriptions: a subscription on a capability other than
// event_consumer.v1, a name that isn't an event name, a name listed twice, or
// a name that is neither a catalog event nor a plugin event, which no host
// delivers.
//
// Validate doesn't apply these checks, because manifests that predate the
// event catalog load without them and a host must keep loading installed
// plugins. Call it from a plugin's own tests to catch a subscription that
// will never fire.
func SubscriptionProblems(manifest *pluginv1.PluginManifest) []string {
	var problems []string
	for _, descriptor := range manifest.GetCapabilities() {
		subscriptions := descriptor.GetSubscriptions()
		if len(subscriptions) == 0 {
			continue
		}
		if descriptor.GetType() != capability.EventConsumer {
			problems = append(problems, fmt.Sprintf("capability %q: subscriptions need type %q", descriptor.GetId(), capability.EventConsumer))
			continue
		}
		seen := make(map[string]struct{}, len(subscriptions))
		for _, name := range subscriptions {
			switch _, known := events.AccessOf(name); {
			case !events.ValidName(name):
				problems = append(problems, fmt.Sprintf("capability %q: %q is not an event name", descriptor.GetId(), name))
			case !known:
				problems = append(problems, fmt.Sprintf("capability %q: %q is not in the event catalog, so no host delivers it", descriptor.GetId(), name))
			}
			if _, dup := seen[name]; dup {
				problems = append(problems, fmt.Sprintf("capability %q: %q is listed twice", descriptor.GetId(), name))
			}
			seen[name] = struct{}{}
		}
	}
	return problems
}
