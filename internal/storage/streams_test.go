package storage

import "testing"

func TestListStreamsByIDs(t *testing.T) {
	store := openTestStore(t)
	source := mustSource(t, store, SourceRemotePlaylist, "https://example.com/list.m3u")

	one := batchChannel("one", "One", "https://stream.example/one.m3u8")
	one.Streams = append(one.Streams, Stream{
		ProviderID: "one-hd",
		URL:        "https://stream.example/one-hd.m3u8",
		Quality:    "HD",
		Position:   1,
	})
	if _, err := store.Channels().ReplaceForSource(
		t.Context(),
		source.ID,
		[]ChannelBatch{one, batchChannel("two", "Two", "https://stream.example/two.m3u8")},
		RefreshMeta{},
	); err != nil {
		t.Fatalf("ReplaceForSource: %v", err)
	}

	first, found, err := store.Channels().FindByProvider(t.Context(), source.ID, "one")
	if err != nil || !found {
		t.Fatalf("FindByProvider one = %v, %v", found, err)
	}
	second, found, err := store.Channels().FindByProvider(t.Context(), source.ID, "two")
	if err != nil || !found {
		t.Fatalf("FindByProvider two = %v, %v", found, err)
	}

	grouped, err := store.Channels().ListStreamsByIDs(
		t.Context(),
		[]int64{first.ID, second.ID, first.ID + 4096},
	)
	if err != nil {
		t.Fatalf("ListStreamsByIDs: %v", err)
	}
	if len(grouped) != 2 {
		t.Fatalf("groups %d, want 2", len(grouped))
	}
	if len(grouped[first.ID]) != 2 || grouped[first.ID][1].Quality != "HD" {
		t.Fatalf("first streams = %+v", grouped[first.ID])
	}
	if len(grouped[second.ID]) != 1 || grouped[second.ID][0].URL != "https://stream.example/two.m3u8" {
		t.Fatalf("second streams = %+v", grouped[second.ID])
	}

	empty, err := store.Channels().ListStreamsByIDs(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListStreamsByIDs empty: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty result = %+v, want no groups", empty)
	}
}
