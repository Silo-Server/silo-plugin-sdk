# Scheduled tasks

A `scheduled_task.v1` capability is a job the host runs on a schedule. The
host calls `ScheduledTask.Run` with the task's key each time the schedule
fires. When Silo runs several API servers, one of them takes each run; the
others skip it.

An administrator sets the schedule on the host. Until they do, the host uses
the capability's default:

```json
{
  "type": "scheduled_task.v1",
  "id": "health_check",
  "display_name": "Health check",
  "scheduled_task": {
    "default_trigger": { "type": "interval", "interval_ms": 300000 }
  }
}
```

| `type` | Fields | Runs |
|---|---|---|
| `interval` | `interval_ms`, at least 60000 | Every `interval_ms` milliseconds. |
| `daily` | `time_of_day`, `"HH:MM"` | Once a day at that time, in the time zone of the Silo server process (its `TZ`; UTC in the default container). |
| `weekly` | `time_of_day`, `day_of_week` (0 is Sunday) | Once a week, in the same time zone. |
| `startup` | none | Once when Silo starts. Servers that start together share one run. |

Without a default, a task the administrator hasn't scheduled runs once when
Silo starts.

Many servers install the same plugin with the same default, so a task that
calls an outside service at `03:00` reaches it from all of them at once. Wait
a random few minutes at the start of `Run`, or pick an unusual time. The trigger type vocabulary is open: a host uses its own
default for a type it doesn't know.

A host that predates `scheduled_task` ignores it, and runs the task once each
time it starts, only if it drops unknown fields wherever it reads the manifest.
The SDK's `manifest.Load` always has. A Silo host built before
Silo-Server/silo-server#2502 does not: it reads a bare plugin binary's manifest
strictly, and before starting a plugin it compares the installed manifest with
the one the plugin reports, unknown fields included. It refuses to install a
binary that sets `scheduled_task` and refuses to start such a plugin installed
from an archive.

A task that has to run at a time each user picks, such as a weekly digest
sent in each user's own time zone, can run every hour and decide per user
whether it's time.
