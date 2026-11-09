package sources

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"mumu/internal/catalog"
	"mumu/internal/netguard"
	"mumu/internal/playlist"
	"mumu/internal/storage"
)

const policyPlaylist = `#EXTM3U
#EXTINF:-1 tvg-id="public",Public (720p)
https://stream.example/public.m3u8
#EXTINF:-1 tvg-id="lan",LAN camera
http://192.168.1.5/live.m3u8
#EXTINF:-1 tvg-id="camera",RTSP camera
rtsp://camera.example/live
`

func usePolicyPlaylist(t *testing.T, server *manageServer) {
	t.Helper()

	server.mu.Lock()
	server.playlist = policyPlaylist
	server.mu.Unlock()
}

// preview reads the shared test server playlist and returns the parsed preview.
func previewPolicySource(t *testing.T) (*Service, Preview) {
	t.Helper()

	service, server, _ := newManageService(t)
	usePolicyPlaylist(t, server)

	preview, err := service.PreviewRemote(t.Context(), server.URL+"/list.m3u", "Camera list")
	if err != nil {
		t.Fatalf("PreviewRemote: %v", err)
	}

	return service, preview
}

func TestPreviewRemoteReportsPrivateHostsAndSchemes(t *testing.T) {
	_, preview := previewPolicySource(t)

	if !preview.NeedsPrivate {
		t.Fatal("preview did not report the private stream host")
	}
	if len(preview.Schemes) != 2 || preview.Schemes[0] != "http" || preview.Schemes[1] != "https" {
		t.Fatalf("schemes = %v, want http https", preview.Schemes)
	}
	if preview.AllowPrivate {
		t.Fatal("preview defaulted to consent")
	}
	if len(preview.Channels) != 2 {
		t.Fatalf("channels = %+v, want public and LAN", preview.Channels)
	}
	if preview.SkippedReasons[playlist.CodeUnsupportedScheme] != 1 {
		t.Fatalf("skipped reasons = %+v, want the RTSP entry", preview.SkippedReasons)
	}
}

func TestSaveWithoutConsentDropsPrivateChannels(t *testing.T) {
	service, preview := previewPolicySource(t)

	result, err := service.SaveSource(t.Context(), preview)
	if err != nil {
		t.Fatalf("SaveSource: %v", err)
	}
	if result.Dropped != 1 || result.Channels != 1 {
		t.Fatalf("result = %+v, want one kept and one dropped", result)
	}
	if result.Source.AllowPrivate {
		t.Fatal("source stored a consent the user never gave")
	}
	if len(result.Source.AllowedSchemes) != 2 ||
		result.Source.AllowedSchemes[0] != "http" ||
		result.Source.AllowedSchemes[1] != "https" {
		t.Fatalf("schemes = %v, want http https", result.Source.AllowedSchemes)
	}

	channels, err := service.Channels(t.Context(), result.Source.ID)
	if err != nil {
		t.Fatalf("Channels: %v", err)
	}
	if len(channels) != 1 || channels[0].ProviderID != "public" {
		t.Fatalf("channels = %+v, want only the public channel", channels)
	}
}

func TestSaveWithConsentStoresPolicy(t *testing.T) {
	service, preview := previewPolicySource(t)
	preview.AllowPrivate = true

	result, err := service.SaveSource(t.Context(), preview)
	if err != nil {
		t.Fatalf("SaveSource: %v", err)
	}
	if result.Dropped != 0 || result.Channels != 2 {
		t.Fatalf("result = %+v, want both channels kept", result)
	}
	if !result.Source.AllowPrivate {
		t.Fatal("consent was not stored")
	}

	channels, err := service.Channels(t.Context(), result.Source.ID)
	if err != nil {
		t.Fatalf("Channels: %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("channels = %+v, want both", channels)
	}
}

// A consent granted at import time must survive every later refresh.
func TestStoredPolicySurvivesRefresh(t *testing.T) {
	service, preview := previewPolicySource(t)
	preview.AllowPrivate = true

	source, err := service.Save(t.Context(), preview)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !source.AllowPrivate {
		t.Fatal("Save did not store consent")
	}

	result, err := service.Refresh(t.Context(), source.ID)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if result.Channels != 2 || result.Dropped != 0 {
		t.Fatalf("result = %+v, want both channels after refresh", result)
	}
	if !result.Source.AllowPrivate {
		t.Fatal("refresh lost the stored consent")
	}

	list, err := service.ListUserSources(t.Context())
	if err != nil {
		t.Fatalf("ListUserSources: %v", err)
	}
	if len(list) != 1 || !list[0].AllowPrivate {
		t.Fatalf("stored sources = %+v, want a stored consent", list)
	}
}

func TestRefreshKeepsStrictPolicyWithoutConsent(t *testing.T) {
	service, preview := previewPolicySource(t)

	source, err := service.Save(t.Context(), preview)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if source.AllowPrivate {
		t.Fatal("Save stored consent without a preview consent")
	}

	result, err := service.Refresh(t.Context(), source.ID)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if result.Channels != 1 {
		t.Fatalf("channels = %d, want the private channel to stay dropped", result.Channels)
	}
	if result.Source.AllowPrivate {
		t.Fatal("refresh widened the stored policy")
	}
}

func TestPreviewStreamReportsPolicy(t *testing.T) {
	service, server, _ := newManageService(t)

	preview, err := service.PreviewStream(t.Context(), server.URL+"/live.m3u8", "Live")
	if err != nil {
		t.Fatalf("PreviewStream: %v", err)
	}
	if !preview.NeedsPrivate {
		t.Fatal("loopback stream did not report a private host")
	}
	if len(preview.Schemes) != 1 || preview.Schemes[0] != "http" {
		t.Fatalf("schemes = %v, want http", preview.Schemes)
	}

	if _, err := service.SaveSource(t.Context(), preview); !errors.Is(err, ErrEmptyPreview) {
		t.Fatalf("save error %v, want ErrEmptyPreview without consent", err)
	}
}

func TestPreviewStreamRejectsUnsupportedScheme(t *testing.T) {
	service, _, _ := newManageService(t)

	_, err := service.PreviewStream(t.Context(), "rtsp://camera.example/live", "Camera")
	if !errors.Is(err, netguard.ErrUnsupportedScheme) {
		t.Fatalf("error %v, want ErrUnsupportedScheme", err)
	}
}

func TestSaveRejectsPreviewOutsidePolicy(t *testing.T) {
	service, _, _ := newManageService(t)

	_, err := service.SaveSource(t.Context(), Preview{
		Kind:        catalog.SourceRemotePlaylist,
		Location:    "https://example.com/list.m3u",
		DisplayName: "Camera list",
		Channels: []catalog.Channel{{
			ProviderID: "camera",
			Name:       "Camera",
			Streams:    []catalog.Stream{{URL: "rtsp://camera.example/live"}},
		}},
	})
	if !errors.Is(err, ErrEmptyPreview) {
		t.Fatalf("error %v, want ErrEmptyPreview", err)
	}
}

// Built-in channels are managed by Discover and can never opt into a policy.
func TestSaveRejectsBuiltinPreview(t *testing.T) {
	service, _, _ := newManageService(t)

	_, err := service.SaveSource(t.Context(), Preview{
		Kind:        catalog.SourceBuiltin,
		Location:    "https://iptv.example/id.m3u",
		DisplayName: "Community",
		Channels: []catalog.Channel{{
			ProviderID: "news",
			Name:       "News",
			Streams:    []catalog.Stream{{URL: "https://stream.example/news.m3u8"}},
		}},
	})
	if !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("error %v, want storage.ErrInvalid", err)
	}
}

func TestStreamAllowed(t *testing.T) {
	strict := catalog.StrictPolicy()
	consented := catalog.NewPolicy(true, []string{"http", "https", "rtsp"})

	for _, tc := range []struct {
		name   string
		kind   catalog.SourceKind
		policy catalog.Policy
		url    string
		want   bool
	}{
		{name: "public https", kind: catalog.SourceRemotePlaylist, policy: strict, url: "https://stream.example/a.m3u8", want: true},
		{name: "public http", kind: catalog.SourceRemotePlaylist, policy: strict, url: "http://stream.example/a.m3u8", want: true},
		{name: "private without consent", kind: catalog.SourceRemotePlaylist, policy: strict, url: "http://192.168.1.5/a.m3u8", want: false},
		{name: "loopback without consent", kind: catalog.SourceRemotePlaylist, policy: strict, url: "http://127.0.0.1/a.m3u8", want: false},
		{name: "private with consent", kind: catalog.SourceRemotePlaylist, policy: consented, url: "http://192.168.1.5/a.m3u8", want: true},
		{name: "local name with consent", kind: catalog.SourceRemotePlaylist, policy: consented, url: "http://box.local/a.m3u8", want: true},
		{name: "rtsp even with consent", kind: catalog.SourceRemotePlaylist, policy: consented, url: "rtsp://camera.example/live", want: false},
		{name: "udp even with consent", kind: catalog.SourceRemotePlaylist, policy: consented, url: "udp://239.0.0.1:1234", want: false},
		{name: "file url remote", kind: catalog.SourceRemotePlaylist, policy: consented, url: "file:///tmp/a.m3u8", want: false},
		{name: "file url local playlist", kind: catalog.SourceLocalPlaylist, policy: strict, url: "file:///tmp/a.m3u8", want: false},
		{name: "path in local playlist", kind: catalog.SourceLocalPlaylist, policy: strict, url: "/tmp/a.m3u8", want: true},
		{name: "path in remote playlist", kind: catalog.SourceRemotePlaylist, policy: strict, url: "/tmp/a.m3u8", want: false},
		{name: "credentials", kind: catalog.SourceRemotePlaylist, policy: strict, url: "https://user:pass@stream.example/a.m3u8", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := streamAllowed(tc.kind, tc.policy, tc.url); got != tc.want {
				t.Fatalf("streamAllowed(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}

// The preview never fetches a stream, so reporting a private host cannot start
// playback or a probe.
func TestPreviewRemoteDoesNotRequestStreams(t *testing.T) {
	var streamRequests atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/list.m3u" {
			streamRequests.Add(1)

			http.NotFound(w, r)

			return
		}
		w.Header().Set("Content-Type", "audio/x-mpegurl")
		_, _ = w.Write([]byte(policyPlaylist))
	}))
	t.Cleanup(server.Close)

	service, _, _ := newManageService(t)
	if _, err := service.PreviewRemote(t.Context(), server.URL+"/list.m3u", "Camera list"); err != nil {
		t.Fatalf("PreviewRemote: %v", err)
	}
	if requests := streamRequests.Load(); requests != 0 {
		t.Fatalf("preview requested %d stream URLs, want none", requests)
	}
}
