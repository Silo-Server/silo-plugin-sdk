package manifest

import (
	"fmt"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/capability"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/events"
)

// validateSubscriptions checks an event_consumer.v1 capability's event names.
// It checks their shape, not the catalog: a host ignores a name it doesn't
// deliver, so a plugin built against a newer SDK still loads on an older host.
func validateSubscriptions(descriptor *pluginv1.CapabilityDescriptor) error {
	subscriptions := descriptor.GetSubscriptions()
	if len(subscriptions) == 0 {
		return nil
	}
	if descriptor.GetType() != capability.EventConsumer {
		return fmt.Errorf("plugin capability %q: subscriptions require type %q", descriptor.GetId(), capability.EventConsumer)
	}
	seen := make(map[string]struct{}, len(subscriptions))
	for _, name := range subscriptions {
		if !events.ValidName(name) {
			return fmt.Errorf("plugin capability %q: subscription %q is not an event name", descriptor.GetId(), name)
		}
		if _, dup := seen[name]; dup {
			return fmt.Errorf("plugin capability %q: subscription %q is listed twice", descriptor.GetId(), name)
		}
		seen[name] = struct{}{}
	}
	return nil
}
