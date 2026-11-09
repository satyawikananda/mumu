package catalog

import (
	"errors"
	"testing"
)

func TestProviderIDStable(t *testing.T) {
	first, err := ProviderID("", "HTTPS://Stream.Example/live.m3u8#fragment")
	if err != nil {
		t.Fatalf("ProviderID: %v", err)
	}
	second, err := ProviderID("", "https://stream.example/live.m3u8")
	if err != nil {
		t.Fatalf("ProviderID: %v", err)
	}
	if first != second {
		t.Fatalf("provider ID changed: %q vs %q", first, second)
	}

	explicit, err := ProviderID("tvg-1", "https://stream.example/live.m3u8")
	if err != nil {
		t.Fatalf("ProviderID explicit: %v", err)
	}
	if explicit != "tvg-1" {
		t.Fatalf("explicit provider ID %q, want tvg-1", explicit)
	}
}

func TestProviderIDRejectsCredentials(t *testing.T) {
	_, err := ProviderID("", "https://user:pass@stream.example/live.m3u8")
	if !errors.Is(err, ErrCredentials) {
		t.Fatalf("error %v, want ErrCredentials", err)
	}
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v, want ErrInvalid", err)
	}
}

func TestCanonicalStreamURL(t *testing.T) {
	for _, tc := range []struct {
		name    string
		raw     string
		want    string
		wantErr error
	}{
		{name: "http", raw: "HTTP://Example.com/live", want: "http://example.com/live"},
		{name: "query kept", raw: "https://example.com/live?token=1", want: "https://example.com/live?token=1"},
		{name: "fragment dropped", raw: "https://example.com/live#x", want: "https://example.com/live"},
		{name: "file url", raw: "file:///tmp/list.m3u", want: "/tmp/list.m3u"},
		{name: "rtmp", raw: "rtmp://example.com/live", wantErr: ErrUnsupportedScheme},
		{name: "empty host", raw: "https:///live", wantErr: ErrInvalid},
		{name: "credentials", raw: "https://a:b@example.com/live", wantErr: ErrCredentials},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CanonicalStreamURL(tc.raw)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error %v, want %v", err, tc.wantErr)
				}

				return
			}
			if err != nil {
				t.Fatalf("CanonicalStreamURL: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSanitizeHeaders(t *testing.T) {
	clean, err := SanitizeHeaders(map[string]string{
		"User-Agent":    "Mozilla/5.0",
		"http-referrer": "https://example.com/",
		"Cookie":        "secret=1",
		"Authorization": "Bearer token",
	})
	if err != nil {
		t.Fatalf("SanitizeHeaders: %v", err)
	}
	if len(clean) != 2 {
		t.Fatalf("headers %+v, want User-Agent and Referer only", clean)
	}
	if clean.Get("user-agent") != "Mozilla/5.0" || clean.Get("Referer") != "https://example.com/" {
		t.Fatalf("headers = %+v", clean)
	}

	if _, err := SanitizeHeaders(map[string]string{"User-Agent": "bad\r\nvalue"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v, want ErrInvalid", err)
	}
}

func TestChannelValidate(t *testing.T) {
	valid := Channel{
		Name:    "News",
		Streams: []Stream{{URL: "https://stream.example/live.m3u8"}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	missingName := valid
	missingName.Name = " "
	if err := missingName.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v, want ErrInvalid", err)
	}

	missingStream := Channel{Name: "News"}
	if err := missingStream.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v, want ErrInvalid", err)
	}
}

func TestFilterMatch(t *testing.T) {
	channel := Channel{Name: "TVRI Nasional", Group: "General", Country: "ID", SourceID: "src"}

	for _, tc := range []struct {
		name   string
		filter Filter
		want   bool
	}{
		{name: "empty", filter: Filter{}, want: true},
		{name: "name", filter: Filter{Query: "tvri"}, want: true},
		{name: "country", filter: Filter{Country: "ID"}, want: true},
		{name: "category", filter: Filter{Category: "general"}, want: true},
		{name: "source", filter: Filter{SourceID: "src"}, want: true},
		{name: "other source", filter: Filter{SourceID: "other"}, want: false},
		{name: "missing", filter: Filter{Query: "sports"}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.filter.Match(channel); got != tc.want {
				t.Fatalf("Match = %v, want %v", got, tc.want)
			}
		})
	}
}
