package logos

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mumu/internal/netguard"
)

func pngImage(t *testing.T, width, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}

	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}

	return buffer.Bytes()
}

func TestFetcherScalesAndCaches(t *testing.T) {
	requests := 0
	logo := pngImage(t, 256, 128)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(logo)
	}))
	defer server.Close()

	fetcher := NewFetcher(netguard.Policy{AllowPrivate: true, RequestTimeout: 2 * time.Second})
	first, err := fetcher.Load(t.Context(), server.URL+"/logo.png")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if first.Bounds().Dx() > maxLogoSize || first.Bounds().Dy() > maxLogoSize {
		t.Fatalf("bounds %v, want at most %d", first.Bounds(), maxLogoSize)
	}

	second, err := fetcher.Load(t.Context(), server.URL+"/logo.png")
	if err != nil {
		t.Fatalf("Load cached: %v", err)
	}
	if first != second {
		t.Fatal("cached load returned a different image")
	}
	if requests != 1 {
		t.Fatalf("requests %d, want 1", requests)
	}
}

func TestFetcherEvicts(t *testing.T) {
	requests := 0
	logo := pngImage(t, 64, 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(logo)
	}))
	defer server.Close()

	fetcher := NewFetcher(
		netguard.Policy{AllowPrivate: true, RequestTimeout: 2 * time.Second},
		WithCacheSize(1),
	)
	for _, path := range []string{"/one.png", "/two.png", "/one.png"} {
		if _, err := fetcher.Load(t.Context(), server.URL+path); err != nil {
			t.Fatalf("Load %s: %v", path, err)
		}
	}
	if requests != 3 {
		t.Fatalf("requests %d, want 3", requests)
	}
}

func TestFetcherRejectsBadResponses(t *testing.T) {
	dimensions := pngImage(t, 4097, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/text":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html></html>"))
		case "/huge":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(dimensions)
		default:
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("not a png"))
		}
	}))
	defer server.Close()

	fetcher := NewFetcher(netguard.Policy{AllowPrivate: true, RequestTimeout: 2 * time.Second})
	if _, err := fetcher.Load(t.Context(), server.URL+"/text"); !errors.Is(err, ErrNotImage) {
		t.Fatalf("text error %v, want ErrNotImage", err)
	}
	if _, err := fetcher.Load(t.Context(), server.URL+"/huge"); !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("huge error %v, want ErrImageTooLarge", err)
	}
	if _, err := fetcher.Load(t.Context(), server.URL+"/broken"); !errors.Is(err, ErrNotImage) {
		t.Fatalf("broken error %v, want ErrNotImage", err)
	}
}

func TestFetcherBlocksPrivateByDefault(t *testing.T) {
	fetcher := NewFetcher(netguard.Policy{RequestTimeout: time.Second})

	if _, err := fetcher.Load(t.Context(), "http://127.0.0.1:1/logo.png"); !errors.Is(err, netguard.ErrPrivateAddress) {
		t.Fatalf("error %v, want ErrPrivateAddress", err)
	}
}
