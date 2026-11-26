package catalog

import (
	"errors"
	"net/url"
	"path/filepath"
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
	fileWant, err := filepath.Abs(filepath.FromSlash("/tmp/list.m3u"))
	if err != nil {
		t.Fatalf("file path fixture: %v", err)
	}

	for _, tc := range []struct {
		name    string
		raw     string
		want    string
		wantErr error
	}{
		{name: "http", raw: "HTTP://Example.com/live", want: "http://example.com/live"},
		{name: "query kept", raw: "https://example.com/live?token=1", want: "https://example.com/live?token=1"},
		{name: "fragment dropped", raw: "https://example.com/live#x", want: "https://example.com/live"},
		{name: "file url", raw: "file:///tmp/list.m3u", want: fileWant},
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

func TestFileURLPath(t *testing.T) {
	for _, tc := range []struct {
		name string
		goos string
		host string
		path string
		want string
	}{
		{name: "windows drive", goos: "windows", path: "/C:/tmp/list.m3u8", want: `C:\tmp\list.m3u8`},
		{name: "windows drive host", goos: "windows", host: "C:", path: "/tmp/list.m3u8", want: `C:\tmp\list.m3u8`},
		{name: "windows unc", goos: "windows", host: "server", path: "/share/list.m3u8", want: `\\server\share\list.m3u8`},
		{name: "unix", goos: "linux", path: "/tmp/list.m3u8", want: "/tmp/list.m3u8"},
	} {
		got := fileURLPath(&url.URL{Host: tc.host, Path: tc.path}, tc.goos)
		if got != tc.want {
			t.Errorf("%s: path=%q, want %q", tc.name, got, tc.want)
		}
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

func TestHasLiveStream(t *testing.T) {
	for _, tc := range []struct {
		name    string
		streams []Stream
		want    bool
	}{
		{name: "no streams", streams: []Stream{}, want: false},
		{
			name:    "not live",
			streams: []Stream{{URL: "https://stream.example/a.m3u8"}},
			want:    false,
		},
		{
			name:    "live",
			streams: []Stream{{URL: "https://stream.example/a.m3u8", Live: true}},
			want:    true,
		},
		{
			name: "one live among several",
			streams: []Stream{
				{URL: "https://stream.example/a.m3u8"},
				{URL: "https://stream.example/b.m3u8", Live: true},
			},
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channel := Channel{Name: "News", Streams: tc.streams}
			if got := HasLiveStream(channel); got != tc.want {
				t.Fatalf("HasLiveStream = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFilterLiveOnly(t *testing.T) {
	live := Stream{URL: "https://stream.example/live.m3u8", Live: true}
	notLive := Stream{URL: "https://stream.example/vod.m3u8"}

	liveChannel := Channel{
		Name:     "TVRI Nasional",
		Group:    "General",
		Country:  "ID",
		SourceID: "src",
		Streams:  []Stream{live},
	}
	notLiveChannel := Channel{
		Name:     "TVRI Nasional",
		Group:    "General",
		Country:  "ID",
		SourceID: "src",
		Streams:  []Stream{notLive},
	}

	for _, tc := range []struct {
		name    string
		channel Channel
		filter  Filter
		want    bool
	}{
		{
			name:    "live stream kept",
			channel: liveChannel,
			filter:  Filter{LiveOnly: true},
			want:    true,
		},
		{
			name:    "only non-live stream dropped",
			channel: notLiveChannel,
			filter:  Filter{LiveOnly: true},
			want:    false,
		},
		{
			name:    "channel without streams dropped",
			channel: Channel{Name: "TVRI Nasional", Group: "General", Country: "ID", SourceID: "src"},
			filter:  Filter{LiveOnly: true},
			want:    false,
		},
		{
			name: "one live among several kept",
			channel: Channel{
				Name:     "TVRI Nasional",
				Group:    "General",
				Country:  "ID",
				SourceID: "src",
				Streams:  []Stream{notLive, live},
			},
			filter: Filter{LiveOnly: true},
			want:   true,
		},
		{
			name:    "live with matching country category source and query",
			channel: liveChannel,
			filter: Filter{
				LiveOnly: true,
				Query:    "tvri",
				Country:  "id",
				Category: "general",
				SourceID: "src",
			},
			want: true,
		},
		{
			name:    "live with mismatched country",
			channel: liveChannel,
			filter:  Filter{LiveOnly: true, Country: "US"},
			want:    false,
		},
		{
			name:    "live with mismatched category",
			channel: liveChannel,
			filter:  Filter{LiveOnly: true, Category: "News"},
			want:    false,
		},
		{
			name:    "live with mismatched source",
			channel: liveChannel,
			filter:  Filter{LiveOnly: true, SourceID: "other"},
			want:    false,
		},
		{
			name:    "live with mismatched query",
			channel: liveChannel,
			filter:  Filter{LiveOnly: true, Query: "sports"},
			want:    false,
		},
		{
			name:    "non-live with matching filters still dropped",
			channel: notLiveChannel,
			filter: Filter{
				LiveOnly: true,
				Query:    "tvri",
				Country:  "ID",
				Category: "General",
				SourceID: "src",
			},
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.filter.Match(tc.channel); got != tc.want {
				t.Fatalf("Match = %v, want %v", got, tc.want)
			}
		})
	}
}
