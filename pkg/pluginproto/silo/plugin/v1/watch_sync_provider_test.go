package pluginv1

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestWatchSyncTypedRemoteStateRoundTrip(t *testing.T) {
	pausedAt := time.Unix(1_700_000_000, 0).UTC()
	input := &WatchSyncListRemoteStateResponse{
		Items: []*WatchSyncRemoteState{{
			ProviderItemKey: "episode:42",
			Media: &WatchSyncMedia{
				MediaItemId: "local-42",
				MediaType:   WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_EPISODE,
			},
			Watched: &WatchSyncRemoteWatchedState{
				PlayCount:     2,
				LastWatchedAt: timestamppb.New(pausedAt.Add(-time.Hour)),
			},
			Progress: &WatchSyncRemoteProgressState{
				ProgressPercent: 42.5,
				PausedAt:        timestamppb.New(pausedAt),
			},
			Watchlist: &WatchSyncRemoteListState{ListedAt: timestamppb.New(pausedAt.Add(-2 * time.Hour)), Removed: true},
		}},
		NextCursor:       "checkpoint-2",
		CompleteSnapshot: true,
	}

	data, err := proto.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var output WatchSyncListRemoteStateResponse
	if err := proto.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}

	item := output.GetItems()[0]
	if item.GetMedia().GetMediaType() != WatchSyncMediaType_WATCH_SYNC_MEDIA_TYPE_EPISODE ||
		item.GetWatched().GetPlayCount() != 2 ||
		item.GetProgress().GetProgressPercent() != 42.5 ||
		!item.GetProgress().GetPausedAt().AsTime().Equal(pausedAt) ||
		!item.GetWatchlist().GetListedAt().AsTime().Equal(pausedAt.Add(-2*time.Hour)) ||
		!item.GetWatchlist().GetRemoved() {
		t.Fatalf("remote state = %#v", item)
	}
}

func TestWatchSyncDeviceAuthorizationRoundTrip(t *testing.T) {
	expiresAt := time.Unix(1_800_000_000, 0).UTC()
	input := &WatchSyncDeviceAuthorizationServiceStartResponse{
		UserCode:                "ABCD-1234",
		VerificationUrl:         "https://provider.example/activate",
		VerificationUrlComplete: "https://provider.example/activate?code=ABCD-1234",
		ProviderState:           []byte(`{"device_code":"secret"}`),
		PollingInterval:         durationpb.New(5 * time.Second),
		ExpiresAt:               timestamppb.New(expiresAt),
	}
	data, err := proto.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var output WatchSyncDeviceAuthorizationServiceStartResponse
	if err := proto.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	if output.GetUserCode() != input.GetUserCode() ||
		output.GetVerificationUrl() != input.GetVerificationUrl() ||
		output.GetVerificationUrlComplete() != input.GetVerificationUrlComplete() ||
		!proto.Equal(&output, input) ||
		string(output.GetProviderState()) != string(input.GetProviderState()) ||
		output.GetPollingInterval().AsDuration() != 5*time.Second ||
		!output.GetExpiresAt().AsTime().Equal(expiresAt) {
		t.Fatalf("device authorization = %#v", &output)
	}
}

func TestWatchSyncDeviceAuthorizationPendingStateRoundTrip(t *testing.T) {
	expiresAt := time.Unix(1_800_000_100, 0).UTC()
	input := &WatchSyncDeviceAuthorizationServicePollResponse{
		Status:          WatchSyncDeviceAuthorizationStatus_WATCH_SYNC_DEVICE_AUTHORIZATION_STATUS_PENDING,
		ProviderState:   []byte("rotated-state"),
		PollingInterval: durationpb.New(10 * time.Second),
		ExpiresAt:       timestamppb.New(expiresAt),
	}
	data, err := proto.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var output WatchSyncDeviceAuthorizationServicePollResponse
	if err := proto.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(input, &output) {
		t.Fatalf("pending device authorization = %#v", &output)
	}
}

func TestWatchSyncDeviceAuthorizationPendingStatePreservesPresence(t *testing.T) {
	explicitEmpty := &WatchSyncDeviceAuthorizationServicePollResponse{ProviderState: []byte{}}
	data, err := proto.Marshal(explicitEmpty)
	if err != nil {
		t.Fatal(err)
	}
	var output WatchSyncDeviceAuthorizationServicePollResponse
	if err := proto.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	if output.ProviderState == nil || len(output.GetProviderState()) != 0 {
		t.Fatalf("explicit empty provider state = %#v", output.ProviderState)
	}

	data, err = proto.Marshal(&WatchSyncDeviceAuthorizationServicePollResponse{})
	if err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := proto.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	if output.ProviderState != nil {
		t.Fatalf("omitted provider state = %#v, want nil", output.ProviderState)
	}
}

func TestWatchSyncListPositionPreservesPresence(t *testing.T) {
	zero := int32(0)
	withZero := &WatchSyncEvent{ListPosition: &zero}
	data, err := proto.Marshal(withZero)
	if err != nil {
		t.Fatal(err)
	}
	var output WatchSyncEvent
	if err := proto.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	if output.ListPosition == nil || output.GetListPosition() != 0 {
		t.Fatalf("explicit zero list position = %#v", output.ListPosition)
	}
	omittedData, err := proto.Marshal(&WatchSyncEvent{})
	if err != nil {
		t.Fatal(err)
	}
	var omittedOutput WatchSyncEvent
	if err := proto.Unmarshal(omittedData, &omittedOutput); err != nil {
		t.Fatal(err)
	}
	if omittedOutput.ListPosition != nil {
		t.Fatal("omitted list position unexpectedly has presence")
	}
}

func TestWatchSyncListTombstoneDoesNotRequireMedia(t *testing.T) {
	input := &WatchSyncRemoteState{
		ProviderItemKey: "remote-1",
		Favorite:        &WatchSyncRemoteListState{Removed: true},
	}
	data, err := proto.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var output WatchSyncRemoteState
	if err := proto.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(input, &output) || output.GetMedia() != nil || !output.GetFavorite().GetRemoved() {
		t.Fatalf("list tombstone = %#v", &output)
	}
}

func TestWatchSyncApplyResultCarriesTypedRateLimit(t *testing.T) {
	retryAfter := 45 * time.Second
	request := &WatchSyncApplyEventsRequest{
		Context: &WatchSyncAuthenticatedContext{
			CapabilityId: "anilist",
		},
		Events: []*WatchSyncEvent{{
			EventId: "event-1",
		}},
	}
	result := &WatchSyncApplyResult{
		EventId: request.GetEvents()[0].GetEventId(),
		Status:  WatchSyncApplyStatus_WATCH_SYNC_APPLY_STATUS_RETRY,
		Fault: &WatchSyncFault{
			Code:        WatchSyncFaultCode_WATCH_SYNC_FAULT_CODE_RATE_LIMITED,
			SafeMessage: "provider rate limit reached",
			RetryAfter:  durationpb.New(retryAfter),
		},
	}

	data, err := proto.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var output WatchSyncApplyResult
	if err := proto.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}

	if request.GetContext().GetCapabilityId() != "anilist" ||
		output.GetEventId() != "event-1" ||
		output.GetStatus() != WatchSyncApplyStatus_WATCH_SYNC_APPLY_STATUS_RETRY ||
		output.GetFault().GetCode() != WatchSyncFaultCode_WATCH_SYNC_FAULT_CODE_RATE_LIMITED ||
		output.GetFault().GetSafeMessage() != "provider rate limit reached" ||
		output.GetFault().GetRetryAfter().AsDuration() != retryAfter {
		t.Fatalf("request=%#v result=%#v", request, &output)
	}
}

func TestWatchSyncEventCarriesAuthoritativeCompletionState(t *testing.T) {
	input := &WatchSyncEvent{
		EventId:           "playback-stop-1",
		WatchHistoryId:    "incomplete-history-row",
		CompletionPercent: 5.5,
		Completed:         false,
	}
	data, err := proto.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var output WatchSyncEvent
	if err := proto.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	if output.GetCompleted() {
		t.Fatal("completed = true, want false")
	}

	input.Completed = true
	data, err = proto.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := proto.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	if !output.GetCompleted() {
		t.Fatal("completed = false, want true")
	}
}
