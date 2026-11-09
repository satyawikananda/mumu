package playlist

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mumu/internal/netguard"
)

func testLoader(t *testing.T) *Loader {
	t.Helper()

	return NewLoader(netguard.Policy{AllowPrivate: true, RequestTimeout: 2 * time.Second})
}

func TestLoadRemoteChannelList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/x-mpegurl")
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"one\",One\nhttps://stream.example/one.m3u8\n"))
	}))
	defer server.Close()

	loaded, err := testLoader(t).Load(t.Context(), server.URL+"/list.m3u", LoadOptions{AllowPrivate: true})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Kind != KindChannelList || len(loaded.Channels) != 1 || loaded.ETag != `"v1"` {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestLoadContentWinsOverExtension(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/x-mpegurl")
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1,One\nhttps://stream.example/one.m3u8\n"))
	}))
	defer server.Close()

	loaded, err := testLoader(t).Load(t.Context(), server.URL+"/list.m3u8", LoadOptions{AllowPrivate: true})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Kind != KindChannelList {
		t.Fatalf("kind %v, want channel list", loaded.Kind)
	}
}

func TestLoadHLSAndDirectStream(t *testing.T) {
	ts := make([]byte, 376)
	ts[0] = 0x47
	ts[188] = 0x47

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/live.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\nseg.ts\n"))
		case "/live.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write(ts)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html></html>"))
		}
	}))
	defer server.Close()

	loader := testLoader(t)

	hls, err := loader.Load(t.Context(), server.URL+"/live.m3u8", LoadOptions{AllowPrivate: true})
	if err != nil {
		t.Fatalf("Load HLS: %v", err)
	}
	if hls.Kind != KindHLSManifest {
		t.Fatalf("kind %v, want HLS", hls.Kind)
	}

	direct, err := loader.Load(t.Context(), server.URL+"/live.ts", LoadOptions{AllowPrivate: true})
	if err != nil {
		t.Fatalf("Load direct: %v", err)
	}
	if direct.Kind != KindDirectStream {
		t.Fatalf("kind %v, want direct stream", direct.Kind)
	}

	if _, err := loader.Load(t.Context(), server.URL+"/page", LoadOptions{AllowPrivate: true}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("HTML error %v, want ErrUnsupported", err)
	}
}

func TestLoadNotModified(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)

			return
		}
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1,One\nhttps://stream.example/one.m3u8\n"))
	}))
	defer server.Close()

	loaded, err := testLoader(t).Load(t.Context(), server.URL+"/list.m3u", LoadOptions{AllowPrivate: true, ETag: `"v1"`})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.NotModified {
		t.Fatalf("loaded = %+v, want not modified", loaded)
	}
}

func TestLoadLocalFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "list.m3u")
	body := "#EXTM3U\n#EXTINF:-1 tvg-id=\"one\",One\nstreams/one.m3u8\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loaded, err := testLoader(t).Load(t.Context(), path, LoadOptions{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Channels) != 1 {
		t.Fatalf("channels %d, want 1", len(loaded.Channels))
	}
	want := filepath.Join(dir, "streams", "one.m3u8")
	if loaded.Channels[0].Streams[0].URL != want {
		t.Fatalf("URL %q, want %q", loaded.Channels[0].Streams[0].URL, want)
	}

	if _, err := testLoader(t).Load(t.Context(), filepath.Join(dir, "missing.m3u"), LoadOptions{}); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("missing file error %v, want ErrUnreadable", err)
	}
}

func TestInspect(t *testing.T) {
	ts := make([]byte, 376)
	ts[0] = 0x47
	ts[188] = 0x47

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hls.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\nseg.ts\n"))
		case "/list.m3u":
			w.Header().Set("Content-Type", "audio/x-mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1,One\nhttps://stream.example/one.m3u8\n"))
		case "/live.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			_, _ = w.Write(ts)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html></html>"))
		}
	}))
	defer server.Close()

	loader := testLoader(t)

	info, err := loader.Inspect(t.Context(), server.URL+"/hls.m3u8", true)
	if err != nil || info.Kind != KindHLSManifest {
		t.Fatalf("Inspect HLS = %+v, %v", info, err)
	}

	info, err = loader.Inspect(t.Context(), server.URL+"/live.ts", true)
	if err != nil || info.Kind != KindDirectStream {
		t.Fatalf("Inspect direct = %+v, %v", info, err)
	}

	if _, err := loader.Inspect(t.Context(), server.URL+"/list.m3u", true); !errors.Is(err, ErrPlaylistNotStream) {
		t.Fatalf("Inspect list error %v, want ErrPlaylistNotStream", err)
	}

	if _, err := loader.Inspect(t.Context(), server.URL+"/page", true); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Inspect page error %v, want ErrUnsupported", err)
	}
}

func TestInspectBlocksPrivateByDefault(t *testing.T) {
	loader := NewLoader(netguard.Policy{RequestTimeout: time.Second})

	if _, err := loader.Inspect(t.Context(), "http://127.0.0.1:1/live.m3u8", false); !errors.Is(err, netguard.ErrPrivateAddress) {
		t.Fatalf("error %v, want ErrPrivateAddress", err)
	}
}
