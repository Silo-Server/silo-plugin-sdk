package pluginv1

import (
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestMarkerRequests_EpisodeIdentityFieldNumbers(t *testing.T) {
	assertFieldNumber(t, &FetchMarkersRequest{}, "episode_external_ids", 7)
	assertFieldNumber(t, &FetchMarkersRequest{}, "episode_title", 8)
	assertFieldNumber(t, &SubmitMarkerRequest{}, "episode_external_ids", 10)
	assertFieldNumber(t, &SubmitMarkerRequest{}, "episode_title", 11)
	assertFieldNumber(t, &FetchMarkersRequest{}, "episode_numbering", 9)
	assertFieldNumber(t, &SubmitMarkerRequest{}, "episode_numbering", 12)
}

func TestMarkerMessages_FileFactsAndAbsentFieldNumbers(t *testing.T) {
	assertFieldNumber(t, &FetchMarkersRequest{}, "file_hash", 10)
	assertFieldNumber(t, &FetchMarkersRequest{}, "file_size", 11)
	assertFieldNumber(t, &FetchMarkersRequest{}, "accepts_absent", 12)
	assertFieldNumber(t, &MarkerSegment{}, "absent", 8)
	assertFieldNumber(t, &SubmitMarkerRequest{}, "file_hash", 13)
	assertFieldNumber(t, &SubmitMarkerRequest{}, "file_size", 14)
	assertFieldNumber(t, &SubmitMarkerRequest{}, "source", 15)
	assertFieldNumber(t, &SubmitMarkerRequest{}, "algorithm", 16)
	assertFieldNumber(t, &SubmitMarkerRequest{}, "confidence", 17)
	assertFieldNumber(t, &SubmitMarkerRequest{}, "absent", 18)
}

// A host that predates the episode fields sends requests without them; the
// plugin must see the series identity unchanged and empty episode fields.
func TestFetchMarkersRequest_OldPayloadDecodesWithoutEpisodeIdentity(t *testing.T) {
	oldPayload, err := proto.Marshal(&FetchMarkersRequest{
		ItemType:      "episode",
		ExternalIds:   &MarkerExternalIDs{TmdbId: "1399", TvdbId: "121361"},
		SeasonNumber:  1,
		EpisodeNumber: 2,
	})
	if err != nil {
		t.Fatalf("marshal old-style payload: %v", err)
	}
	var decoded FetchMarkersRequest
	if err := proto.Unmarshal(oldPayload, &decoded); err != nil {
		t.Fatalf("unmarshal old-style payload: %v", err)
	}
	if decoded.GetExternalIds().GetTvdbId() != "121361" || decoded.GetSeasonNumber() != 1 || decoded.GetEpisodeNumber() != 2 {
		t.Fatalf("decoded series identity = %+v", &decoded)
	}
	if decoded.GetEpisodeExternalIds().GetTvdbId() != "" || decoded.GetEpisodeTitle() != "" || decoded.GetEpisodeNumbering() != "" {
		t.Fatalf("episode identity = %q/%q/%q, want empty", decoded.GetEpisodeExternalIds().GetTvdbId(), decoded.GetEpisodeTitle(), decoded.GetEpisodeNumbering())
	}
}

// A host that predates the file facts sends requests without them, and an
// absent segment reaches any host with no bounds, which an older host reads
// as covering the whole file.
func TestMarkerMessages_OldPayloadsAndAbsentSegmentPresence(t *testing.T) {
	fetchBytes, err := proto.Marshal(&FetchMarkersRequest{ItemType: "movie", ExternalIds: &MarkerExternalIDs{TmdbId: "603"}})
	if err != nil {
		t.Fatalf("marshal fetch: %v", err)
	}
	var fetch FetchMarkersRequest
	if err := proto.Unmarshal(fetchBytes, &fetch); err != nil {
		t.Fatalf("unmarshal fetch: %v", err)
	}
	if fetch.GetFileHash() != "" || fetch.GetFileSize() != 0 || fetch.GetAcceptsAbsent() {
		t.Fatalf("fetch file facts = %q/%d/%t, want empty", fetch.GetFileHash(), fetch.GetFileSize(), fetch.GetAcceptsAbsent())
	}

	submitBytes, err := proto.Marshal(&SubmitMarkerRequest{ItemType: "movie", Segment: "intro", StartSeconds: proto.Float64(0), EndSeconds: proto.Float64(30)})
	if err != nil {
		t.Fatalf("marshal submit: %v", err)
	}
	var submit SubmitMarkerRequest
	if err := proto.Unmarshal(submitBytes, &submit); err != nil {
		t.Fatalf("unmarshal submit: %v", err)
	}
	if submit.GetFileHash() != "" || submit.GetFileSize() != 0 || submit.GetSource() != "" ||
		submit.GetAlgorithm() != "" || submit.GetConfidence() != 0 || submit.GetAbsent() {
		t.Fatalf("submit new fields not empty: %+v", &submit)
	}

	segBytes, err := proto.Marshal(&MarkerSegment{Segment: "intro", Absent: true, Confidence: 0.8})
	if err != nil {
		t.Fatalf("marshal segment: %v", err)
	}
	var seg MarkerSegment
	if err := proto.Unmarshal(segBytes, &seg); err != nil {
		t.Fatalf("unmarshal segment: %v", err)
	}
	if !seg.GetAbsent() || seg.StartSeconds != nil || seg.EndSeconds != nil {
		t.Fatalf("absent segment = absent %t start %v end %v, want absent with no bounds", seg.GetAbsent(), seg.StartSeconds, seg.EndSeconds)
	}
}

func TestSubmitMarkerRequest_EpisodeIdentityRoundTrips(t *testing.T) {
	data, err := proto.Marshal(&SubmitMarkerRequest{
		ItemType:           "episode",
		ExternalIds:        &MarkerExternalIDs{TvdbId: "121361"},
		SeasonNumber:       1,
		EpisodeNumber:      1,
		Segment:            "intro",
		EpisodeExternalIds: &MarkerExternalIDs{TvdbId: "3254641", TmdbId: "63056"},
		EpisodeTitle:       "Winter Is Coming",
		EpisodeNumbering:   "tvdb",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded SubmitMarkerRequest
	if err := proto.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.GetEpisodeExternalIds().GetTvdbId() != "3254641" || decoded.GetEpisodeExternalIds().GetTmdbId() != "63056" ||
		decoded.GetEpisodeTitle() != "Winter Is Coming" || decoded.GetEpisodeNumbering() != "tvdb" {
		t.Fatalf("decoded episode identity = %+v", &decoded)
	}
}
