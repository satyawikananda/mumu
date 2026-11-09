package playlist

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mumu/internal/netguard"
)

// http and https are always part of a normalized allowlist, so a stored policy
// can only add a proven transport; it can never remove the reviewed ones.
func TestParseAllowsHTTPAndHTTPSForEveryPolicy(t *testing.T) {
	body := `#EXTM3U
#EXTINF:-1 tvg-id="secure",Secure
https://stream.example/secure.m3u8
#EXTINF:-1 tvg-id="plain",Plain
http://stream.example/plain.m3u8
`

	for _, schemes := range [][]string{nil, {"https"}, {"rtsp"}, {"udp", "file"}} {
		result, err := Parse([]byte(body), Options{AllowedSchemes: schemes})
		if err != nil {
			t.Fatalf("Parse(%v): %v", schemes, err)
		}
		if len(result.Channels) != 2 {
			t.Fatalf("Parse(%v) channels = %+v, want both http and https", schemes, result.Channels)
		}
	}
}

// A stored policy can never re-enable a transport mumu has not proven.
func TestParseNeverAcceptsBlockedSchemesFromPolicy(t *testing.T) {
	body := `#EXTM3U
#EXTINF:-1 tvg-id="rtsp",RTSP
rtsp://camera.example/live
#EXTINF:-1 tvg-id="udp",UDP
udp://239.0.0.1:1234
#EXTINF:-1 tvg-id="file",File
file:///tmp/live.m3u8
#EXTINF:-1 tvg-id="srt",SRT
srt://camera.example:9000
#EXTINF:-1 tvg-id="public",Public
https://stream.example/public.m3u8
`
	opts := Options{
		AllowPrivateHosts: true,
		AllowedSchemes:    []string{"rtsp", "udp", "srt", "file", "http", "https"},
	}

	result, err := Parse([]byte(body), opts)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Channels) != 1 || result.Channels[0].ProviderID != "public" {
		t.Fatalf("channels = %+v, want only the http entry", result.Channels)
	}
}

// A local playlist may still use a local path, whatever the scheme allowlist
// says: the file dialog is the consent for it.
func TestParseLocalPathsIgnoreSchemeAllowlist(t *testing.T) {
	dir := t.TempDir()
	body := `#EXTM3U
#EXTINF:-1 tvg-id="local",Local
file://` + dir + `/local.m3u8
#EXTINF:-1 tvg-id="relative",Relative
channels/relative.m3u8
`

	result, err := Parse([]byte(body), Options{
		AllowLocalPaths: true,
		BaseDir:         dir,
		AllowedSchemes:  []string{"https"},
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Channels) != 2 {
		t.Fatalf("channels = %+v, want both local entries", result.Channels)
	}
}

// The document read and the stream policy are separate: a private playlist URL
// is the user's own choice, while the streams inside it stay strict.
func TestLoadAppliesStreamPolicySeparately(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/x-mpegurl")
		_, _ = w.Write([]byte(`#EXTM3U
#EXTINF:-1 tvg-id="lan",LAN
http://192.168.1.5/live.m3u8
#EXTINF:-1 tvg-id="public",Public
https://stream.example/public.m3u8
`))
	}))
	t.Cleanup(server.Close)

	loader := NewLoader(netguard.Policy{AllowPrivate: true, RequestTimeout: 2 * time.Second})

	strict, err := loader.Load(t.Context(), server.URL+"/list.m3u", LoadOptions{AllowPrivate: true})
	if err != nil {
		t.Fatalf("Load strict: %v", err)
	}
	if len(strict.Channels) != 1 || strict.Channels[0].ProviderID != "public" {
		t.Fatalf("strict channels = %+v, want the public one", strict.Channels)
	}
	if strict.SkippedReasons[CodePrivateAddress] != 1 {
		t.Fatalf("skip reasons = %+v, want CodePrivateAddress", strict.SkippedReasons)
	}

	consented, err := loader.Load(t.Context(), server.URL+"/list.m3u", LoadOptions{
		AllowPrivate:        true,
		AllowPrivateStreams: true,
	})
	if err != nil {
		t.Fatalf("Load consented: %v", err)
	}
	if len(consented.Channels) != 2 {
		t.Fatalf("consented channels = %+v, want both", consented.Channels)
	}
}

func TestParseRejectsUnsupportedSchemeWithDiagnostics(t *testing.T) {
	body := `#EXTM3U
#EXTINF:-1 tvg-id="camera",Camera
rtsp://camera.example/live
`

	_, err := Parse([]byte(body), Options{})
	if !errors.Is(err, ErrNoChannels) {
		t.Fatalf("error %v, want ErrNoChannels", err)
	}
}
