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
			Watchlist: &WatchSyncRemoteListState{ListedAt: timestamppb.New(pausedAt.Add(-2 * time.Hour))},
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
		!item.GetWatchlist().GetListedAt().AsTime().Equal(pausedAt.Add(-2*time.Hour)) {
		t.Fatalf("remote state = %#v", item)
	}
}

func TestWatchSyncDeviceAuthorizationRoundTrip(t *testing.T) {
	expiresAt := time.Unix(1_800_000_000, 0).UTC()
	input := &WatchSyncStartDeviceAuthorizationResponse{
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
	var output WatchSyncStartDeviceAuthorizationResponse
	if err := proto.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	if output.GetUserCode() != input.GetUserCode() ||
		output.GetPollingInterval().AsDuration() != 5*time.Second ||
		!output.GetExpiresAt().AsTime().Equal(expiresAt) {
		t.Fatalf("device authorization = %#v", &output)
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
