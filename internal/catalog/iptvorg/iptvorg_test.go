package iptvorg

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"mumu/internal/catalog"
	"mumu/internal/netguard"
)

func TestPlaylistURL(t *testing.T) {
	provider := NewProvider(netguard.New(netguard.Policy{}))

	for _, tc := range []struct {
		code string
		want string
	}{
		{code: "id", want: BaseURL + "/iptv/countries/id.m3u"},
		{code: " ID ", want: BaseURL + "/iptv/countries/id.m3u"},
		{code: "US", want: BaseURL + "/iptv/countries/us.m3u"},
	} {
		got, err := provider.PlaylistURL(tc.code)
		if err != nil {
			t.Fatalf("PlaylistURL(%q): %v", tc.code, err)
		}
		if got != tc.want {
			t.Fatalf("URL %q, want %q", got, tc.want)
		}
	}

	for _, code := range []string{"", "USA", "1D", "I"} {
		if _, err := provider.PlaylistURL(code); !errors.Is(err, ErrInvalidCountry) {
			t.Fatalf("PlaylistURL(%q) error %v, want ErrInvalidCountry", code, err)
		}
	}
}

func TestParseCountries(t *testing.T) {
	body := `[
		{"name":"Indonesia","code":"id","flag":"ID"},
		{"name":"Australia","code":"AU","flag":"AU"},
		{"name":"","code":"NZ"},
		{"name":"Duplicate","code":"ID"},
		{"name":"Broken","code":"USA"},
		{"name":"Missing","code":""}
	]`

	countries, err := ParseCountries([]byte(body))
	if err != nil {
		t.Fatalf("ParseCountries: %v", err)
	}
	if len(countries) != 3 {
		t.Fatalf("countries = %+v, want 3", countries)
	}
	if countries[0].Name != "Australia" || countries[1].Code != "ID" || countries[2].Name != "NZ" {
		t.Fatalf("countries = %+v, want sorted valid regions", countries)
	}

	if _, err := ParseCountries([]byte(`{"nope":true}`)); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("error %v, want ErrInvalidMetadata", err)
	}
	if _, err := ParseCountries([]byte(`[]`)); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("empty error %v, want ErrInvalidMetadata", err)
	}
}

func TestCountriesFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"Indonesia","code":"ID"}]`))
	}))
	defer server.Close()

	provider := NewProviderWithBase(netguard.New(netguard.Policy{AllowPrivate: true}), server.URL)
	countries, err := provider.Countries(t.Context())
	if err != nil {
		t.Fatalf("Countries: %v", err)
	}
	if len(countries) != 1 || countries[0].Code != "ID" {
		t.Fatalf("countries = %+v", countries)
	}
}

func TestBlocklist(t *testing.T) {
	blocked, err := ParseBlocklist([]byte(`{
		"channels":["blocked-id"],
		"hosts":["bad.example"]
	}`))
	if err != nil {
		t.Fatalf("ParseBlocklist: %v", err)
	}

	channels := []catalog.Channel{
		{ProviderID: "ok", Streams: []catalog.Stream{{URL: "https://good.example/live.m3u8"}}},
		{ProviderID: "blocked-id", Streams: []catalog.Stream{{URL: "https://good.example/live.m3u8"}}},
		{ProviderID: "host", Streams: []catalog.Stream{{URL: "https://cdn.bad.example/live.m3u8"}}},
		{ProviderID: "similar", Streams: []catalog.Stream{{URL: "https://notbad.example/live.m3u8"}}},
		{ProviderID: "suffix", EPGID: "blocked-id@SD", Streams: []catalog.Stream{{URL: "https://good.example/live.m3u8"}}},
	}

	kept, count := blocked.Filter(channels)
	if count != 3 || len(kept) != 2 {
		t.Fatalf("kept = %+v, blocked = %d, want 2 kept and 3 blocked", kept, count)
	}
	if kept[0].ProviderID != "ok" || kept[1].ProviderID != "similar" {
		t.Fatalf("kept = %+v", kept)
	}

	merged := blocked.WithEntries([]BlockEntry{
		{Channel: "upstream", Reason: "dmca"},
		{Channel: "blocked-id", Reason: "nsfw"},
	})
	if !merged.Blocks(catalog.Channel{ProviderID: "upstream"}) {
		t.Fatal("upstream channel not blocked")
	}
	if !merged.Blocks(catalog.Channel{ProviderID: "blocked-id"}) {
		t.Fatal("local channel lost after merge")
	}
}

func TestParseBlockEntries(t *testing.T) {
	entries, err := ParseBlockEntries([]byte(`[
		{"channel":"a","reason":"dmca","ref":"https://example.com/a"},
		{"channel":"a","reason":"nsfw"},
		{"channel":"","reason":"dmca"},
		{"channel":"b"}
	]`))
	if err != nil {
		t.Fatalf("ParseBlockEntries: %v", err)
	}
	if len(entries) != 2 || entries[0].Channel != "a" || entries[1].Channel != "b" {
		t.Fatalf("entries = %+v", entries)
	}

	if _, err := ParseBlockEntries([]byte(`{"no":true}`)); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("error %v, want ErrInvalidMetadata", err)
	}
}

func TestBlockedChannelsFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"channel":"a","reason":"dmca"}]`))
	}))
	defer server.Close()

	provider := NewProviderWithBase(netguard.New(netguard.Policy{AllowPrivate: true}), server.URL)
	entries, err := provider.BlockedChannels(t.Context())
	if err != nil {
		t.Fatalf("BlockedChannels: %v", err)
	}
	if len(entries) != 1 || entries[0].Channel != "a" {
		t.Fatalf("entries = %+v", entries)
	}
}
