package events

import (
	"encoding/json"
	"time"
)

// LibraryChanged is the payload of catalog.library.changed.
type LibraryChanged struct {
	LibraryID int `json:"library_id"`
	// Reason is what changed the library, such as "scan".
	Reason                 string `json:"reason"`
	New                    int    `json:"new"`
	Updated                int    `json:"updated"`
	Missing                int    `json:"missing"`
	MatchedFiles           int    `json:"matched_files"`
	RetriedItems           int    `json:"retried_items"`
	StillUnmatchedWarnings int    `json:"still_unmatched_warnings"`
}

// ItemChanged is the payload of catalog.item.changed.
type ItemChanged struct {
	LibraryID int    `json:"library_id"`
	ContentID string `json:"content_id"`
	// Change names what changed, such as "metadata_updated". The vocabulary
	// is open; treat an unknown value as a general change.
	Change string `json:"change"`
}

// SessionEvent is the payload of session.started, session.paused,
// session.resumed and session.stopped.
type SessionEvent struct {
	SessionID string `json:"session_id"`
	// Seq orders one session's events: a higher number is newer. Events can
	// arrive out of order.
	Seq int64     `json:"seq"`
	At  time.Time `json:"at"`
	// State is the session's state after the event: playing, paused or
	// stopped.
	State string `json:"state"`
	// EndReason is why a stopped session ended.
	EndReason string `json:"end_reason,omitempty"`
	// Session is the live session on a started, paused or resumed event, as
	// the /api/v2 AdminPlaybackSession schema describes it. Absent when no
	// server has a live record of it. DecodeSession reads common fields.
	Session json.RawMessage `json:"session,omitempty"`
	// History is the history entry a stopped event recorded, as the /api/v2
	// AdminPlaybackHistoryEntry schema describes it. DecodeHistory reads
	// common fields.
	History json.RawMessage `json:"history,omitempty"`
}

// DecodeSession decodes the live session. It returns nil when the event has
// none.
func (e SessionEvent) DecodeSession() (*PlaybackSession, error) {
	return decodeOptional[PlaybackSession](e.Session)
}

// DecodeHistory decodes the history entry. It returns nil when the event has
// none.
func (e SessionEvent) DecodeHistory() (*PlaybackHistoryEntry, error) {
	return decodeOptional[PlaybackHistoryEntry](e.History)
}

func decodeOptional[T any](raw json.RawMessage) (*T, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	out := new(T)
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, err
	}
	return out, nil
}

// PlaybackSession is a subset of the /api/v2 AdminPlaybackSession schema. As
// in /api/v2, IDs are strings.
type PlaybackSession struct {
	SessionID           string    `json:"session_id"`
	UserID              string    `json:"user_id"`
	Username            string    `json:"username"`
	ProfileID           string    `json:"profile_id"`
	ProfileName         string    `json:"profile_name,omitempty"`
	ContentID           string    `json:"content_id,omitempty"`
	MediaTitle          string    `json:"media_title"`
	MediaType           string    `json:"media_type"`
	SeriesName          string    `json:"series_name,omitempty"`
	EpisodeName         string    `json:"episode_name,omitempty"`
	SeasonNumber        *int      `json:"season_number,omitempty"`
	EpisodeNumber       *int      `json:"episode_number,omitempty"`
	PosterURL           string    `json:"poster_url,omitempty"`
	PlayMethod          string    `json:"play_method"`
	EffectivePlayMethod string    `json:"effective_play_method,omitempty"`
	StartedAt           time.Time `json:"started_at"`
	PositionSeconds     float64   `json:"position_seconds"`
	IsPaused            bool      `json:"is_paused"`
	ClientIP            string    `json:"client_ip,omitempty"`
	// StreamLocation is "local" or "remote".
	StreamLocation    string `json:"stream_location"`
	ClientName        string `json:"client_name,omitempty"`
	ClientVersion     string `json:"client_version,omitempty"`
	DeviceID          string `json:"device_id,omitempty"`
	DevicePlatform    string `json:"device_platform,omitempty"`
	StreamBitrateKbps *int   `json:"stream_bitrate_kbps"`
}

// PlaybackHistoryEntry is a subset of the /api/v2 AdminPlaybackHistoryEntry
// schema. As in /api/v2, IDs are strings.
type PlaybackHistoryEntry struct {
	SessionID       string    `json:"session_id"`
	UserID          string    `json:"user_id"`
	Username        string    `json:"username"`
	ProfileID       string    `json:"profile_id"`
	ProfileName     string    `json:"profile_name"`
	MediaItemID     string    `json:"media_item_id"`
	MediaTitle      string    `json:"media_title"`
	MediaType       string    `json:"media_type"`
	PlayMethod      string    `json:"play_method"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	WatchedSeconds  float64   `json:"watched_seconds"`
	DurationSeconds *float64  `json:"duration_seconds"`
	Completed       bool      `json:"completed"`
	SeriesTitle     string    `json:"series_title"`
	SeasonNumber    *int      `json:"season_number"`
	EpisodeNumber   *int      `json:"episode_number"`
	ClientIP        *string   `json:"client_ip"`
	DeviceID        *string   `json:"device_id"`
	DevicePlatform  *string   `json:"device_platform"`
	ClientName      *string   `json:"client_name"`
	EndReason       *string   `json:"end_reason"`
}

// ScanEvent is the payload of scan.started, scan.completed, scan.failed and
// scan.cancelled.
type ScanEvent struct {
	ID        string `json:"id"`
	LibraryID int    `json:"library_id"`
	// Mode and Trigger say how the scan runs and what started it. Their
	// vocabularies are open.
	Mode string `json:"mode"`
	// Path is set when the scan covers one folder of the library.
	Path         string      `json:"path,omitempty"`
	Trigger      string      `json:"trigger"`
	Status       string      `json:"status"`
	StartedAt    *time.Time  `json:"started_at,omitempty"`
	CompletedAt  *time.Time  `json:"completed_at,omitempty"`
	ErrorMessage string      `json:"error_message,omitempty"`
	Result       *ScanResult `json:"result,omitempty"`
}

// ScanResult counts what a scan found.
type ScanResult struct {
	New       int `json:"new"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Missing   int `json:"missing"`
	// MissingSkippedProtected counts files the scan didn't mark missing
	// because their storage was offline or unreadable.
	MissingSkippedProtected int `json:"missing_skipped_protected"`
	FilesDeleted            int `json:"files_deleted"`
	ItemsDeleted            int `json:"items_deleted"`
	MatchedFiles            int `json:"matched_files"`
	StillUnmatchedWarnings  int `json:"still_unmatched_warnings"`
	Errors                  int `json:"errors"`
}

// JobEvent is the payload of job.created, job.completed, job.failed and
// job.cancelled.
type JobEvent struct {
	ID string `json:"id"`
	// JobType names the job, such as "library_refresh". The vocabulary is
	// open.
	JobType         string     `json:"job_type"`
	Status          string     `json:"status"`
	CreatedByUserID int        `json:"created_by_user_id"`
	Message         string     `json:"message"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	ProgressCurrent int        `json:"progress_current"`
	ProgressTotal   int        `json:"progress_total"`
	RequestedAt     time.Time  `json:"requested_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// RequestEvent is the payload of request.submitted, request.approved,
// request.declined and request.fulfilled.
type RequestEvent struct {
	ID string `json:"id"`
	// MediaType is "movie" or "series". The vocabulary is open.
	MediaType string `json:"media_type"`
	Title     string `json:"title"`
	Year      *int   `json:"year,omitempty"`
	TMDBID    int    `json:"tmdb_id,omitempty"`
	TVDBID    *int   `json:"tvdb_id,omitempty"`
	IMDbID    string `json:"imdb_id,omitempty"`
	// PosterPath is a TMDB image path, such as "/abc.jpg".
	PosterPath string `json:"poster_path,omitempty"`
	// Status is the request's state after the event. The vocabulary is open.
	Status string `json:"status"`
	// RequestedByUserID and RequestedByProfileID identify the requester;
	// both are empty for a request no profile made, such as an import.
	RequestedByUserID    int    `json:"requested_by_user_id,omitempty"`
	RequestedByProfileID string `json:"requested_by_profile_id,omitempty"`
	RequesterName        string `json:"requester_name,omitempty"`
	// Reason is why a request was declined, when one was given.
	Reason string `json:"reason,omitempty"`
	// LibraryContentID is the catalog item a fulfilled request matched.
	LibraryContentID string    `json:"library_content_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// Notification is the payload of notification.created: one delivery
// to one profile's inbox.
type Notification struct {
	ID string `json:"id"`
	// Type is the delivery type, such as "episode.available",
	// "request.approved", "request.declined" or "request.fulfilled". The
	// vocabulary is open.
	Type string `json:"type"`
	// UserID is the account that owns the profile. Profile IDs repeat across
	// accounts, so identify a recipient by UserID and ProfileID together.
	UserID        int     `json:"user_id"`
	ProfileID     string  `json:"profile_id"`
	LibraryID     *int    `json:"library_id,omitempty"`
	SeriesID      *string `json:"series_id,omitempty"`
	EpisodeID     *string `json:"episode_id,omitempty"`
	SeriesTitle   string  `json:"series_title,omitempty"`
	EpisodeTitle  string  `json:"episode_title,omitempty"`
	SeasonNumber  *int    `json:"season_number,omitempty"`
	EpisodeNumber *int    `json:"episode_number,omitempty"`
	// PosterPath is a catalog image path. Resolve it to a URL with the
	// host's ResolveCatalogImageURLs.
	PosterPath string `json:"poster_path,omitempty"`
	// PosterURL is a poster URL when the host has one ready. It can expire.
	PosterURL string `json:"poster_url,omitempty"`
	// ReasonFlags is a JSON object whose shape depends on Type: why an
	// episode was announced (favorite, watchlist, continue_watching,
	// next_up) or which request changed.
	ReasonFlags json.RawMessage `json:"reason_flags,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}
