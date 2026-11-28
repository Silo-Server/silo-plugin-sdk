package pluginv1

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func assertFieldNumber(t *testing.T, message protoreflect.ProtoMessage, field string, want protoreflect.FieldNumber) {
	t.Helper()
	got := message.ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name(field))
	if got == nil || got.Number() != want {
		t.Fatalf("%T.%s field number = %v, want %d", message, field, got, want)
	}
}

func TestMetadataItemDescriptor_IncludesReleaseDate(t *testing.T) {
	field := (&MetadataItem{}).ProtoReflect().Descriptor().Fields().ByName("release_date")
	if field == nil {
		t.Fatal("MetadataItem descriptor is missing release_date")
	}
}

func TestMetadataTitleDescriptors_IncludeAliasAndLanguageFields(t *testing.T) {
	for name, message := range map[string]protoreflect.ProtoMessage{
		"ProviderSearchResult": &ProviderSearchResult{},
		"MetadataItem":         &MetadataItem{},
	} {
		fields := message.ProtoReflect().Descriptor().Fields()
		for _, field := range []string{"title_aliases", "title_language", "title_is_fallback"} {
			if fields.ByName(protoreflect.Name(field)) == nil {
				t.Fatalf("%s descriptor is missing %s", name, field)
			}
		}
	}

	if (&ProviderSearchResult{}).ProtoReflect().Descriptor().Fields().ByName("original_language") == nil {
		t.Fatal("ProviderSearchResult descriptor is missing original_language")
	}

	fields := (&TitleAlias{}).ProtoReflect().Descriptor().Fields()
	for _, field := range []string{"title", "language", "kind"} {
		if fields.ByName(protoreflect.Name(field)) == nil {
			t.Fatalf("TitleAlias descriptor is missing %s", field)
		}
	}

	assertFieldNumber(t, &ProviderSearchResult{}, "title_aliases", 9)
	assertFieldNumber(t, &ProviderSearchResult{}, "title_language", 10)
	assertFieldNumber(t, &ProviderSearchResult{}, "title_is_fallback", 11)
	assertFieldNumber(t, &ProviderSearchResult{}, "original_language", 12)
	assertFieldNumber(t, &MetadataItem{}, "title_aliases", 33)
	assertFieldNumber(t, &MetadataItem{}, "title_language", 34)
	assertFieldNumber(t, &MetadataItem{}, "title_is_fallback", 35)
	assertFieldNumber(t, &MetadataItem{}, "title_aliases_complete", 36)
}

func TestMetadataTextLanguageFieldNumbers(t *testing.T) {
	assertFieldNumber(t, &MetadataItem{}, "overview_language", 37)
	assertFieldNumber(t, &SeasonRecord{}, "title_language", 9)
	assertFieldNumber(t, &SeasonRecord{}, "overview_language", 10)
	assertFieldNumber(t, &SeasonRecord{}, "backdrop_path", 11)
	assertFieldNumber(t, &EpisodeRecord{}, "title_language", 12)
	assertFieldNumber(t, &EpisodeRecord{}, "overview_language", 13)
}

func TestMetadataItem_OldPayloadDecodesWithOptionalTitleFields(t *testing.T) {
	oldPayload, err := proto.Marshal(&MetadataItem{
		ProviderId: "603", ItemType: "movie", Title: "The Matrix", OriginalTitle: "The Matrix", Year: 1999,
	})
	if err != nil {
		t.Fatalf("marshal old-style payload: %v", err)
	}
	var decoded MetadataItem
	if err := proto.Unmarshal(oldPayload, &decoded); err != nil {
		t.Fatalf("unmarshal old-style payload: %v", err)
	}
	if decoded.GetTitle() != "The Matrix" || decoded.GetOriginalTitle() != "The Matrix" || decoded.GetYear() != 1999 ||
		len(decoded.GetTitleAliases()) != 0 || decoded.GetTitleLanguage() != "" || decoded.GetTitleIsFallback() || decoded.GetTitleAliasesComplete() {
		t.Fatalf("decoded payload = %#v", &decoded)
	}
}

func TestProviderSearchResult_OldPayloadDecodesWithOptionalTitleFields(t *testing.T) {
	oldPayload, err := proto.Marshal(&ProviderSearchResult{
		ProviderId: "603", ItemType: "movie", Title: "The Matrix", OriginalTitle: "The Matrix", Year: 1999,
	})
	if err != nil {
		t.Fatalf("marshal old-style payload: %v", err)
	}
	var decoded ProviderSearchResult
	if err := proto.Unmarshal(oldPayload, &decoded); err != nil {
		t.Fatalf("unmarshal old-style payload: %v", err)
	}
	if decoded.GetTitle() != "The Matrix" || len(decoded.GetTitleAliases()) != 0 || decoded.GetTitleLanguage() != "" || decoded.GetTitleIsFallback() {
		t.Fatalf("decoded payload = %#v", &decoded)
	}
}

func TestGetMetadataRequestDescriptor_IncludesContextFields(t *testing.T) {
	fields := (&GetMetadataRequest{}).ProtoReflect().Descriptor().Fields()
	for _, name := range []string{"provider_ids", "language", "file_path"} {
		if fields.ByName(protoreflect.Name(name)) == nil {
			t.Fatalf("GetMetadataRequest descriptor is missing %s", name)
		}
	}
}

func TestMetadataProviderRequestDescriptors_IncludeProviderContext(t *testing.T) {
	tests := []struct {
		name      string
		fieldName string
		message   protoreflect.ProtoMessage
	}{
		{
			name:      "GetSeasonsRequest",
			fieldName: "provider_ids",
			message:   &GetSeasonsRequest{},
		},
		{
			name:      "GetEpisodesRequest",
			fieldName: "provider_ids",
			message:   &GetEpisodesRequest{},
		},
		{
			name:      "GetImagesRequest provider_ids",
			fieldName: "provider_ids",
			message:   &GetImagesRequest{},
		},
		{
			name:      "GetImagesRequest language",
			fieldName: "language",
			message:   &GetImagesRequest{},
		},
		{
			name:      "GetImagesRequest season_number",
			fieldName: "season_number",
			message:   &GetImagesRequest{},
		},
		{
			name:      "SearchMetadataRequest language",
			fieldName: "language",
			message:   &SearchMetadataRequest{},
		},
		{
			name:      "GetSeasonsRequest language",
			fieldName: "language",
			message:   &GetSeasonsRequest{},
		},
		{
			name:      "GetEpisodesRequest language",
			fieldName: "language",
			message:   &GetEpisodesRequest{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.message.ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name(tt.fieldName)) == nil {
				t.Fatalf("%s descriptor is missing %s", tt.name, tt.fieldName)
			}
		})
	}
}

func TestGetImagesRequest_SeasonNumberPreservesPresenceAndSpecials(t *testing.T) {
	seasonZero := int32(0)
	assertOptionalInt32Presence(
		t,
		&GetImagesRequest{SeasonNumber: &seasonZero},
		&GetImagesRequest{},
		func(message proto.Message) *int32 {
			return message.(*GetImagesRequest).SeasonNumber
		},
	)
	assertFieldNumber(t, &GetImagesRequest{}, "season_number", 5)
}

func TestImageRecord_SeasonNumberPreservesPresenceAndSpecials(t *testing.T) {
	seasonZero := int32(0)
	assertOptionalInt32Presence(
		t,
		&ImageRecord{SeasonNumber: &seasonZero},
		&ImageRecord{},
		func(message proto.Message) *int32 {
			return message.(*ImageRecord).SeasonNumber
		},
	)
	assertFieldNumber(t, &ImageRecord{}, "season_number", 7)
}

func TestMetadataProviderServiceDescriptor_IncludesPersonDetailRPC(t *testing.T) {
	method := File_silo_plugin_v1_metadata_provider_proto.Services().
		ByName("MetadataProvider").
		Methods().
		ByName("GetPersonDetail")
	if method == nil {
		t.Fatal("MetadataProvider service descriptor is missing GetPersonDetail")
	}
}

func TestGetPersonDetailRequestDescriptor_IncludesProviderContext(t *testing.T) {
	fields := (&GetPersonDetailRequest{}).ProtoReflect().Descriptor().Fields()
	for _, name := range []string{"provider_ids", "language"} {
		if fields.ByName(protoreflect.Name(name)) == nil {
			t.Fatalf("GetPersonDetailRequest descriptor is missing %s", name)
		}
	}
}

func TestPersonDetailRecordDescriptor_IncludesRefreshFields(t *testing.T) {
	fields := (&PersonDetailRecord{}).ProtoReflect().Descriptor().Fields()
	for _, name := range []string{
		"name",
		"sort_name",
		"bio",
		"birth_date",
		"death_date",
		"birthplace",
		"homepage",
		"photo_path",
		"photo_thumbhash",
		"provider_ids",
	} {
		if fields.ByName(protoreflect.Name(name)) == nil {
			t.Fatalf("PersonDetailRecord descriptor is missing %s", name)
		}
	}
}

// The episode order fields' wire contract: number, kind, and for the episode
// numbers explicit presence (proto3 optional).
func TestEpisodeOrderingFields(t *testing.T) {
	for _, tc := range []struct {
		message  protoreflect.ProtoMessage
		field    string
		number   protoreflect.FieldNumber
		kind     protoreflect.Kind
		presence bool
	}{
		{&GetSeasonsRequest{}, "ordering", 4, protoreflect.StringKind, false},
		{&GetEpisodesRequest{}, "ordering", 5, protoreflect.StringKind, false},
		{&EpisodeRecord{}, "absolute_number", 14, protoreflect.Int32Kind, true},
		{&EpisodeRecord{}, "default_season_number", 15, protoreflect.Int32Kind, true},
		{&EpisodeRecord{}, "default_episode_number", 16, protoreflect.Int32Kind, true},
	} {
		assertFieldNumber(t, tc.message, tc.field, tc.number)
		field := tc.message.ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name(tc.field))
		if field.Kind() != tc.kind || field.HasPresence() != tc.presence || field.IsList() {
			t.Errorf("%T.%s = %v (presence %v, list %v), want %v (presence %v)",
				tc.message, tc.field, field.Kind(), field.HasPresence(), field.IsList(), tc.kind, tc.presence)
		}
	}
}

// An episode's default-order coordinates can be season 0 (specials), which
// must stay distinguishable from a plugin that did not send them.
func TestEpisodeRecord_DefaultOrderNumbersPreservePresence(t *testing.T) {
	zero := int32(0)
	for name, get := range map[string]func(proto.Message) *int32{
		"absolute_number":        func(m proto.Message) *int32 { return m.(*EpisodeRecord).AbsoluteNumber },
		"default_season_number":  func(m proto.Message) *int32 { return m.(*EpisodeRecord).DefaultSeasonNumber },
		"default_episode_number": func(m proto.Message) *int32 { return m.(*EpisodeRecord).DefaultEpisodeNumber },
	} {
		t.Run(name, func(t *testing.T) {
			present := &EpisodeRecord{AbsoluteNumber: &zero, DefaultSeasonNumber: &zero, DefaultEpisodeNumber: &zero}
			assertOptionalInt32Presence(t, present, &EpisodeRecord{}, get)

			seven := int32(7)
			data, err := proto.Marshal(&EpisodeRecord{AbsoluteNumber: &seven, DefaultSeasonNumber: &seven, DefaultEpisodeNumber: &seven})
			if err != nil {
				t.Fatal(err)
			}
			decoded := &EpisodeRecord{}
			if err := proto.Unmarshal(data, decoded); err != nil {
				t.Fatal(err)
			}
			if value := get(decoded); value == nil || *value != 7 {
				t.Fatalf("%s after a round trip = %v, want 7", name, value)
			}
		})
	}
}
