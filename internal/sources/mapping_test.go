package sources

import (
	"errors"
	"testing"

	"mumu/internal/catalog"
)

func TestKindRoundTrip(t *testing.T) {
	for _, kind := range []catalog.SourceKind{
		catalog.SourceBuiltin,
		catalog.SourceRemotePlaylist,
		catalog.SourceLocalPlaylist,
		catalog.SourceCustomStream,
	} {
		stored, err := ToStorageKind(kind)
		if err != nil {
			t.Fatalf("ToStorageKind(%s): %v", kind, err)
		}
		back, err := FromStorageKind(stored)
		if err != nil {
			t.Fatalf("FromStorageKind(%s): %v", stored, err)
		}
		if back != kind {
			t.Fatalf("round trip %s -> %s", kind, back)
		}
	}

	if _, err := FromStorageKind("unknown"); !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("error %v, want catalog.ErrInvalid", err)
	}
}

func TestSourceMappingIsDeterministic(t *testing.T) {
	source := catalog.Source{
		Kind:        catalog.SourceRemotePlaylist,
		Location:    "https://example.com/list.m3u?token=1",
		DisplayName: "Example",
		Enabled:     true,
	}

	first, err := ToStorageSource(source)
	if err != nil {
		t.Fatalf("ToStorageSource: %v", err)
	}
	source.Location = "https://example.com/list.m3u?token=2"
	second, err := ToStorageSource(source)
	if err != nil {
		t.Fatalf("ToStorageSource: %v", err)
	}
	if first.ID == "" || first.ID != second.ID {
		t.Fatalf("source IDs %q and %q, want stable", first.ID, second.ID)
	}

	back, err := FromStorageSource(first)
	if err != nil {
		t.Fatalf("FromStorageSource: %v", err)
	}
	if back.Kind != catalog.SourceRemotePlaylist || back.DisplayName != "Example" || !back.Enabled {
		t.Fatalf("source = %+v", back)
	}
}

func TestToStorageBatch(t *testing.T) {
	channels := []catalog.Channel{{
		ProviderID: "tvg-1",
		Name:       "News",
		Group:      "News",
		Country:    "ID",
		Language:   "Indonesian",
		LogoURL:    "https://logo.example/news.png",
		EPGID:      "tvg-1",
		Streams: []catalog.Stream{{
			URL:     "https://stream.example/news.m3u8",
			Quality: "1080p",
			HLS:     true,
			Live:    true,
			Headers: catalog.Headers{"User-Agent": "Mozilla/5.0", "Cookie": "secret"},
		}},
	}}

	batch, err := ToStorageBatch(channels)
	if err != nil {
		t.Fatalf("ToStorageBatch: %v", err)
	}
	if len(batch) != 1 {
		t.Fatalf("batch %+v", batch)
	}

	item := batch[0]
	if item.Channel.StreamURL != "https://stream.example/news.m3u8" || item.Channel.EPGID != "tvg-1" {
		t.Fatalf("channel = %+v", item.Channel)
	}
	if len(item.Streams) != 1 || !item.Streams[0].HLS || item.Streams[0].Quality != "1080p" {
		t.Fatalf("streams = %+v", item.Streams)
	}
	if item.Streams[0].Headers["User-Agent"] != "Mozilla/5.0" {
		t.Fatalf("headers = %+v", item.Streams[0].Headers)
	}
	if _, exists := item.Streams[0].Headers["Cookie"]; exists {
		t.Fatalf("cookie survived sanitizing: %+v", item.Streams[0].Headers)
	}

	back := FromStorageChannel(item.Channel, item.Streams)
	if back.Name != "News" || back.Group != "News" || len(back.Streams) != 1 {
		t.Fatalf("mapped = %+v", back)
	}
}

func TestToStorageBatchRejectsEmptyStreams(t *testing.T) {
	_, err := ToStorageBatch([]catalog.Channel{{Name: "News"}})
	if !errors.Is(err, catalog.ErrInvalid) {
		t.Fatalf("error %v, want catalog.ErrInvalid", err)
	}
}

func TestToStorageBatchKeepsStableStreamIdentity(t *testing.T) {
	stream := catalog.Stream{URL: "https://stream.example/news.m3u8", Live: true}
	first, err := ToStorageBatch([]catalog.Channel{{Name: "News", Streams: []catalog.Stream{stream}}})
	if err != nil {
		t.Fatalf("ToStorageBatch: %v", err)
	}
	second, err := ToStorageBatch([]catalog.Channel{{Name: "News", Streams: []catalog.Stream{stream}}})
	if err != nil {
		t.Fatalf("ToStorageBatch: %v", err)
	}
	if first[0].Streams[0].ProviderID == "" || first[0].Streams[0].ProviderID != second[0].Streams[0].ProviderID {
		t.Fatalf("stream IDs %q and %q", first[0].Streams[0].ProviderID, second[0].Streams[0].ProviderID)
	}
}
