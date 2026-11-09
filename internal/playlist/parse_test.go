package playlist

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseExtendedAttributes(t *testing.T) {
	body := `#EXTM3U
#PLAYLIST:Example list
#EXTINF:-1 tvg-id="tvri.id" tvg-logo="https://logo.example/tvri.png" tvg-country="id" tvg-language="Indonesian" group-title="General",TVRI Nasional
https://stream.example/tvri.m3u8
`
	result, err := Parse([]byte(body), Options{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if result.Kind != KindChannelList || result.Name != "Example list" {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Channels) != 1 {
		t.Fatalf("channels %d, want 1", len(result.Channels))
	}

	channel := result.Channels[0]
	if channel.Name != "TVRI Nasional" || channel.ProviderID != "tvri.id" {
		t.Fatalf("channel = %+v", channel)
	}
	if channel.Group != "General" || channel.Country != "ID" || channel.Language != "Indonesian" {
		t.Fatalf("metadata = %+v", channel)
	}
	if channel.LogoURL != "https://logo.example/tvri.png" || channel.EPGID != "tvri.id" {
		t.Fatalf("logo or EPG = %+v", channel)
	}
	if len(channel.Streams) != 1 || !channel.Streams[0].HLS || !channel.Streams[0].Live {
		t.Fatalf("stream = %+v", channel.Streams)
	}
}

func TestParseQuotedAttributesAndCommas(t *testing.T) {
	body := `#EXTM3U
#EXTINF:-1 tvg-id="news" group-title="News, Weather",News, Weather HD
https://stream.example/news.m3u8
`
	result, err := Parse([]byte(body), Options{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	channel := result.Channels[0]
	if channel.Name != "News, Weather HD" || channel.Group != "News, Weather" {
		t.Fatalf("channel = %+v", channel)
	}
}

func TestParseUnquotedAttributes(t *testing.T) {
	body := `#EXTM3U
#EXTINF:-1 tvg-id=tv1 group-title=News,TV One
https://stream.example/tv1.m3u8
`
	result, err := Parse([]byte(body), Options{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	channel := result.Channels[0]
	if channel.ProviderID != "tv1" || channel.Group != "News" || channel.Name != "TV One" {
		t.Fatalf("channel = %+v", channel)
	}
}

func TestParseUnicodeBOMAndCRLF(t *testing.T) {
	body := "\xef\xbb\xbf#EXTM3U\r\n#EXTINF:-1,TV São Paulo\r\nhttps://stream.example/sp.m3u8\r\n"
	result, err := Parse([]byte(body), Options{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if result.Channels[0].Name != "TV São Paulo" {
		t.Fatalf("name %q", result.Channels[0].Name)
	}
}

func TestParseWindows1252Fallback(t *testing.T) {
	body := []byte("#EXTM3U\n#EXTINF:-1,TV S\xe3o Paulo\nhttps://stream.example/sp.m3u8\n")
	result, err := Parse(body, Options{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if result.Channels[0].Name != "TV São Paulo" {
		t.Fatalf("name %q", result.Channels[0].Name)
	}
	if !hasWarning(result, CodeEncodingFallback) {
		t.Fatalf("warnings = %+v, want encoding fallback", result.Warnings)
	}
}

func TestParseDuplicateNamesStaySeparate(t *testing.T) {
	body := `#EXTM3U
#EXTINF:-1 tvg-id="one",News
https://stream.example/one.m3u8
#EXTINF:-1 tvg-id="two",News
https://stream.example/two.m3u8
`
	result, err := Parse([]byte(body), Options{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Channels) != 2 {
		t.Fatalf("channels %d, want 2", len(result.Channels))
	}
}

func TestParseVariantsMerge(t *testing.T) {
	body := `#EXTM3U
#EXTINF:-1 tvg-id="news",News HD (1080p)
https://stream.example/hd.m3u8
#EXTINF:-1 tvg-id="news",News SD (480p)
https://stream.example/sd.m3u8
`
	result, err := Parse([]byte(body), Options{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Channels) != 1 || len(result.Channels[0].Streams) != 2 {
		t.Fatalf("channels = %+v", result.Channels)
	}
	if quality := result.Channels[0].Streams[0].Quality; quality != "1080p" {
		t.Fatalf("quality %q, want 1080p", quality)
	}
}

func TestParseSkipsBrokenEntries(t *testing.T) {
	body := `#EXTM3U
#EXTINF:-1,No URL
#EXTINF:-1 tvg-id="nameless"
https://stream.example/nameless.m3u8
#EXTINF:-1 tvg-id="rtmp",RTMP
rtmp://stream.example/live
#EXTINF:-1 tvg-id="private",Private
http://192.168.1.10/live.m3u8
`
	result, err := Parse([]byte(body), Options{})
	if err != nil {
		if !errors.Is(err, ErrNoChannels) {
			t.Fatalf("Parse: %v", err)
		}
	}
	if result.Skipped != 4 {
		t.Fatalf("skipped %d, want 4", result.Skipped)
	}
	for _, code := range []Code{CodeMissingURL, CodeMissingName, CodeUnsupportedScheme, CodePrivateAddress} {
		if !hasWarning(result, code) {
			t.Fatalf("warnings %+v, want %s", result.Warnings, code)
		}
		if result.SkippedReasons[code] == 0 {
			t.Fatalf("skip reasons %+v, want %s", result.SkippedReasons, code)
		}
	}
	if len(result.SkippedEntries) != 4 {
		t.Fatalf("skipped entries %+v, want 4", result.SkippedEntries)
	}
	if entry := result.SkippedEntries[3]; entry.Host != "192.168.1.10" || entry.Scheme != "http" {
		t.Fatalf("private entry %+v, want host and scheme only", entry)
	}
}

func TestParsePrivateStreamsWhenAllowed(t *testing.T) {
	body := `#EXTM3U
#EXTINF:-1 tvg-id="private",Private
http://192.168.1.10/live.m3u8
`
	result, err := Parse([]byte(body), Options{AllowPrivateHosts: true})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Channels) != 1 {
		t.Fatalf("channels %d, want 1", len(result.Channels))
	}
}

func TestParseHeaders(t *testing.T) {
	body := `#EXTM3U
#EXTINF:-1 tvg-id="news" http-referrer="https://example.com/" http-user-agent="Mozilla/5.0" Cookie="secret",News
#EXTVLCOPT:http-referrer=https://example.com/watch
https://stream.example/news.m3u8
`
	result, err := Parse([]byte(body), Options{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	headers := result.Channels[0].Streams[0].Headers
	if headers.Get("Referer") != "https://example.com/watch" {
		t.Fatalf("headers = %+v", headers)
	}
	if headers.Get("User-Agent") != "Mozilla/5.0" {
		t.Fatalf("headers = %+v", headers)
	}
	if _, exists := headers["Cookie"]; exists {
		t.Fatalf("cookie was kept: %+v", headers)
	}
}

func TestParseLocalPaths(t *testing.T) {
	dir := t.TempDir()
	body := `#EXTM3U
#EXTINF:-1 tvg-id="local",Local
channels/local.m3u8
`
	result, err := Parse([]byte(body), Options{AllowLocalPaths: true, BaseDir: dir})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := filepath.Join(dir, "channels", "local.m3u8")
	if result.Channels[0].Streams[0].URL != want {
		t.Fatalf("URL %q, want %q", result.Channels[0].Streams[0].URL, want)
	}

	_, err = Parse([]byte(body), Options{})
	if !errors.Is(err, ErrNoChannels) {
		t.Fatalf("remote parse error %v, want ErrNoChannels", err)
	}
}

func TestParseTruncates(t *testing.T) {
	body := `#EXTM3U
#EXTINF:-1 tvg-id="one",One
https://stream.example/one.m3u8
#EXTINF:-1 tvg-id="two",Two
https://stream.example/two.m3u8
`
	result, err := Parse([]byte(body), Options{MaxChannels: 1})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !result.Truncated || len(result.Channels) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if !hasWarning(result, CodeTruncated) {
		t.Fatalf("warnings = %+v", result.Warnings)
	}
}

func TestParseEmptyPlaylist(t *testing.T) {
	_, err := Parse([]byte("#EXTM3U\n"), Options{})
	if !errors.Is(err, ErrNoChannels) {
		t.Fatalf("error %v, want ErrNoChannels", err)
	}

	var playlistErr *Error
	if !errors.As(err, &playlistErr) || playlistErr.Code != CodeEmptyPlaylist {
		t.Fatalf("error %v, want CodeEmptyPlaylist", err)
	}
}

func TestParseHLSManifest(t *testing.T) {
	body := `#EXTM3U
#EXT-X-TARGETDURATION:6
#EXT-X-STREAM-INF:BANDWIDTH=1280000
1080p/index.m3u8
`
	result, err := Parse([]byte(body), Options{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if result.Kind != KindHLSManifest || len(result.Channels) != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestParseExtGroup(t *testing.T) {
	body := `#EXTM3U
#EXTINF:-1 tvg-id="one",One
#EXTGRP:Sports
https://stream.example/one.m3u8
`
	result, err := Parse([]byte(body), Options{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if result.Channels[0].Group != "Sports" {
		t.Fatalf("group %q, want Sports", result.Channels[0].Group)
	}
}

func TestParseDefaultCountry(t *testing.T) {
	body := `#EXTM3U
#EXTINF:-1 tvg-id="one",One
https://stream.example/one.m3u8
`
	result, err := Parse([]byte(body), Options{DefaultCountry: "id"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if result.Channels[0].Country != "ID" {
		t.Fatalf("country %q, want ID", result.Channels[0].Country)
	}
}

func TestParseAttributes(t *testing.T) {
	attributes := parseAttributes(`tvg-id="a b" group-title=News http-referrer="https://x/?a=1&b=2" broken`)
	if attributes["tvg-id"] != "a b" || attributes["group-title"] != "News" {
		t.Fatalf("attributes = %+v", attributes)
	}
	if attributes["http-referrer"] != "https://x/?a=1&b=2" {
		t.Fatalf("referrer = %q", attributes["http-referrer"])
	}
}

func hasWarning(result Result, code Code) bool {
	for _, warning := range result.Warnings {
		if warning.Code == code {
			return true
		}
	}

	return false
}

func BenchmarkParse(b *testing.B) {
	var builder strings.Builder
	builder.WriteString("#EXTM3U\n")
	for i := range 1000 {
		fmt.Fprintf(
			&builder,
			"#EXTINF:-1 tvg-id=\"c%d\" group-title=\"News\",Channel %d (720p)\nhttps://stream.example/%d.m3u8\n",
			i,
			i,
			i,
		)
	}
	data := []byte(builder.String())

	b.ReportAllocs()
	for b.Loop() {
		if _, err := Parse(data, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func TestParseSkippedEntriesBoundedAndRedacted(t *testing.T) {
	var builder strings.Builder
	builder.WriteString("#EXTM3U\n")
	for i := range 30 {
		fmt.Fprintf(&builder, "#EXTINF:-1 tvg-id=\"p%d\",Private %d\n", i, i)
		builder.WriteString("http://192.168.1.10/live?token=secret\n")
	}

	result, err := Parse([]byte(builder.String()), Options{})
	if err != nil && !errors.Is(err, ErrNoChannels) {
		t.Fatalf("Parse: %v", err)
	}
	if result.Skipped != 30 || result.SkippedReasons[CodePrivateAddress] != 30 {
		t.Fatalf("skipped/reasons = %d %+v", result.Skipped, result.SkippedReasons)
	}
	if len(result.SkippedEntries) != maxSkippedEntries {
		t.Fatalf("entries %d, want %d", len(result.SkippedEntries), maxSkippedEntries)
	}
	for _, entry := range result.SkippedEntries {
		if entry.Host != "192.168.1.10" || entry.Scheme != "http" || strings.Contains(entry.Host, "token") {
			t.Fatalf("entry leaks or loses host: %+v", entry)
		}
	}
}
