package manifest

import (
	"fmt"
	"regexp"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Silo-Server/silo-plugin-sdk/pkg/pluginsdk/capability"
)

var (
	timeOfDayPattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
)

// Scheduled task trigger types this SDK knows. The vocabulary is open: a
// host falls back to its default schedule for a type it doesn't know.
const (
	TriggerTypeInterval = "interval"
	TriggerTypeDaily    = "daily"
	TriggerTypeWeekly   = "weekly"
	TriggerTypeStartup  = "startup"

	// MinTriggerIntervalMs is the shortest default interval a manifest may
	// declare.
	MinTriggerIntervalMs = 60_000
)

func validateScheduledTaskCapability(descriptor *pluginv1.CapabilityDescriptor) error {
	task := descriptor.GetScheduledTask()
	if task == nil {
		return nil
	}
	if descriptor.GetType() != capability.ScheduledTask {
		return fmt.Errorf("plugin capability %q: scheduled_task descriptor requires type %q", descriptor.GetId(), capability.ScheduledTask)
	}
	trigger := task.GetDefaultTrigger()
	if trigger == nil {
		return nil
	}
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("plugin capability %q default_trigger: "+format, append([]any{descriptor.GetId()}, args...)...)
	}
	switch trigger.GetType() {
	case "":
		return invalid("type is required")
	case TriggerTypeInterval:
		if trigger.GetIntervalMs() < MinTriggerIntervalMs {
			return invalid("interval_ms must be at least %d", MinTriggerIntervalMs)
		}
	case TriggerTypeWeekly:
		if day := trigger.GetDayOfWeek(); day < 0 || day > 6 {
			return invalid("day_of_week must be 0 (Sunday) to 6 (Saturday)")
		}
		fallthrough
	case TriggerTypeDaily:
		if !timeOfDayPattern.MatchString(trigger.GetTimeOfDay()) {
			return invalid("time_of_day must be HH:MM")
		}
	}
	return nil
}
