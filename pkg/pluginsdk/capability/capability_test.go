package capability

import "testing"

func TestScanSourceIsKnownType(t *testing.T) {
	if ScanSource != "scan_source.v1" {
		t.Fatalf("ScanSource const = %q, want %q", ScanSource, "scan_source.v1")
	}
	found := false
	for _, k := range KnownTypes {
		if k == ScanSource {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ScanSource (%q) missing from KnownTypes %v", ScanSource, KnownTypes)
	}
}

func TestMarkerProviderIsKnownType(t *testing.T) {
	if MarkerProvider != "marker_provider.v1" {
		t.Fatalf("MarkerProvider const = %q, want %q", MarkerProvider, "marker_provider.v1")
	}
	found := false
	for _, k := range KnownTypes {
		if k == MarkerProvider {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("MarkerProvider (%q) missing from KnownTypes %v", MarkerProvider, KnownTypes)
	}
}

func TestImageResolverIsKnownType(t *testing.T) {
	if ImageResolver != "image_resolver.v1" {
		t.Fatalf("ImageResolver const = %q, want %q", ImageResolver, "image_resolver.v1")
	}
	found := false
	for _, k := range KnownTypes {
		if k == ImageResolver {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ImageResolver (%q) missing from KnownTypes %v", ImageResolver, KnownTypes)
	}
}

func TestNetworkAccessProviderIsKnownType(t *testing.T) {
	if NetworkAccessProvider != "network_access_provider.v1" {
		t.Fatalf("NetworkAccessProvider const = %q, want %q", NetworkAccessProvider, "network_access_provider.v1")
	}
	found := false
	for _, k := range KnownTypes {
		if k == NetworkAccessProvider {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("NetworkAccessProvider (%q) missing from KnownTypes %v", NetworkAccessProvider, KnownTypes)
	}
}

func TestPlaybackSourceProviderIsKnownType(t *testing.T) {
	if PlaybackSourceProvider != "playback_source_provider.v1" {
		t.Fatalf("PlaybackSourceProvider const = %q, want %q", PlaybackSourceProvider, "playback_source_provider.v1")
	}
	for _, k := range KnownTypes {
		if k == PlaybackSourceProvider {
			return
		}
	}
	t.Fatalf("PlaybackSourceProvider (%q) missing from KnownTypes %v", PlaybackSourceProvider, KnownTypes)
}
