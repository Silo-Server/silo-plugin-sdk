# Scheduled tasks

A `scheduled_task.v1` capability is a job the host runs on a schedule. The
host calls `ScheduledTask.Run` with the task's key each time the schedule
fires, on one API server.

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
| `daily` | `time_of_day`, `"HH:MM"` | Once a day at that time, in the server's local time. |
| `weekly` | `time_of_day`, `day_of_week` (0 is Sunday) | Once a week. |
| `startup` | none | Once each time the host starts. |

Without a default, and on hosts that predate `default_trigger`, a task the
administrator hasn't scheduled runs once each time the host starts. The
trigger type vocabulary is open: a host uses its own default for a type it
doesn't know.

A task that has to run at a time each user picks, such as a weekly digest
sent in each user's own time zone, can run every hour and decide per user
whether it's time.
