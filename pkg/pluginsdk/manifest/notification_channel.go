package manifest

import (
	"fmt"
	"strings"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/capability"
)

func validateNotificationChannelCapability(descriptor *pluginv1.CapabilityDescriptor) error {
	channel := descriptor.GetNotificationChannel()
	if descriptor.GetType() != capability.NotificationChannel {
		if channel != nil {
			return fmt.Errorf("plugin capability %q: notification_channel descriptor requires type %q", descriptor.GetId(), capability.NotificationChannel)
		}
		return nil
	}
	if !watchSyncSlugPattern.MatchString(descriptor.GetId()) {
		return fmt.Errorf("plugin capability %q: notification channel capability id must be a path-safe lowercase slug", descriptor.GetId())
	}
	if channel == nil {
		return fmt.Errorf("plugin capability %q: notification_channel descriptor is required", descriptor.GetId())
	}
	if strings.TrimSpace(channel.GetDisplayName()) == "" {
		return fmt.Errorf("plugin capability %q: notification_channel display_name is required", descriptor.GetId())
	}
	if err := ValidateDestinationSchema(channel.GetDestinationSchema()); err != nil {
		return fmt.Errorf("plugin capability %q destination schema: %w", descriptor.GetId(), err)
	}
	return nil
}
