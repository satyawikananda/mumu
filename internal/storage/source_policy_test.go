package storage

import "testing"

func TestSourcePolicyDefaultsToStrict(t *testing.T) {
	store := openTestStore(t)

	source, err := NewSource(SourceRemotePlaylist, "https://example.com/list.m3u", "List")
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	stored, err := store.Sources().Upsert(t.Context(), source)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if stored.AllowPrivate {
		t.Fatal("new source stored a private-host consent")
	}
	if len(stored.AllowedSchemes) != 2 || stored.AllowedSchemes[0] != "http" || stored.AllowedSchemes[1] != "https" {
		t.Fatalf("schemes = %v, want http https", stored.AllowedSchemes)
	}
}

func TestSourcePolicyRoundTrip(t *testing.T) {
	store := openTestStore(t)

	source, err := NewSource(SourceRemotePlaylist, "https://example.com/list.m3u", "List")
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	source.AllowPrivate = true
	source.AllowedSchemes = []string{"HTTPS", "http", "https", " ", "FILE"}

	if _, err := store.Sources().Upsert(t.Context(), source); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	stored, found, err := store.Sources().Get(t.Context(), source.ID)
	if err != nil || !found {
		t.Fatalf("Get = %v, %v", found, err)
	}
	if !stored.AllowPrivate {
		t.Fatal("consent was not persisted")
	}
	// The repository trims, lowercases and deduplicates but does not sort or
	// judge a scheme: the catalog layer owns allow/deny.
	if len(stored.AllowedSchemes) != 3 {
		t.Fatalf("schemes = %v, want three distinct entries", stored.AllowedSchemes)
	}
	if stored.AllowedSchemes[0] != "https" || stored.AllowedSchemes[1] != "http" || stored.AllowedSchemes[2] != "file" {
		t.Fatalf("schemes = %v", stored.AllowedSchemes)
	}

	list, err := store.Sources().List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || !list[0].AllowPrivate || len(list[0].AllowedSchemes) != 3 {
		t.Fatalf("list = %+v", list)
	}
}

func TestSourcePolicyUpsertReplacesStoredConsent(t *testing.T) {
	store := openTestStore(t)

	source, err := NewSource(SourceRemotePlaylist, "https://example.com/list.m3u", "List")
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	source.AllowPrivate = true
	if _, err := store.Sources().Upsert(t.Context(), source); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}

	reimported, err := NewSource(SourceRemotePlaylist, "https://example.com/list.m3u", "List")
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	stored, err := store.Sources().Upsert(t.Context(), reimported)
	if err != nil {
		t.Fatalf("second Upsert: %v", err)
	}
	if stored.AllowPrivate {
		t.Fatal("re-import kept a consent the user did not grant again")
	}
}
