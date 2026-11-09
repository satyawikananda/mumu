package netguard

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func testClient(allowPrivate bool) *Client {
	return New(Policy{AllowPrivate: allowPrivate, RequestTimeout: 2 * time.Second})
}

func TestValidateURL(t *testing.T) {
	policy := Policy{}

	for _, tc := range []struct {
		name    string
		raw     string
		wantErr error
	}{
		{name: "https", raw: "https://example.com/list.m3u"},
		{name: "http", raw: "http://example.com/list.m3u"},
		{name: "ftp", raw: "ftp://example.com/list.m3u", wantErr: ErrUnsupportedScheme},
		{name: "file", raw: "file:///tmp/list.m3u", wantErr: ErrUnsupportedScheme},
		{name: "credentials", raw: "https://user:pass@example.com/list.m3u", wantErr: ErrCredentials},
		{name: "missing host", raw: "https:///list.m3u", wantErr: ErrInvalidURL},
		{name: "loopback", raw: "http://127.0.0.1/list.m3u", wantErr: ErrPrivateAddress},
		{name: "private", raw: "http://10.1.2.3/list.m3u", wantErr: ErrPrivateAddress},
		{name: "localhost", raw: "http://localhost:8080/list.m3u", wantErr: ErrPrivateAddress},
		{name: "local name", raw: "http://printer.local/list.m3u", wantErr: ErrPrivateAddress},
		{name: "control", raw: "https://example.com/\x00", wantErr: ErrInvalidURL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := policy.ValidateURL(tc.raw)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateURL: %v", err)
				}

				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestCheckAddress(t *testing.T) {
	blocked := []string{"127.0.0.1", "::1", "10.0.0.1", "192.168.1.10", "172.16.0.1", "169.254.1.1", "fe80::1", "0.0.0.0", "224.0.0.1"}
	for _, raw := range blocked {
		address := netip.MustParseAddr(raw)
		if err := (Policy{}).CheckAddress(address); !errors.Is(err, ErrPrivateAddress) {
			t.Fatalf("%s: error %v, want ErrPrivateAddress", raw, err)
		}
		if err := (Policy{AllowPrivate: true}).CheckAddress(address); err != nil {
			t.Fatalf("%s with AllowPrivate: %v", raw, err)
		}
	}

	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "2001:4860:4860::8888"} {
		if err := (Policy{}).CheckAddress(netip.MustParseAddr(raw)); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
	}
}

func TestFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/list.m3u":
			w.Header().Set("Content-Type", "audio/x-mpegurl")
			w.Header().Set("ETag", `"v1"`)
			_, _ = w.Write([]byte("#EXTM3U\n"))
		case "/etag":
			if r.Header.Get("If-None-Match") == `"v1"` {
				w.WriteHeader(http.StatusNotModified)

				return
			}
			_, _ = w.Write([]byte("#EXTM3U\n"))
		case "/redirect":
			http.Redirect(w, r, "/list.m3u", http.StatusFound)
		case "/bad-redirect":
			http.Redirect(w, r, "ftp://example.com/list.m3u", http.StatusFound)
		case "/large":
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte(strings.Repeat("x", 100)))
		case "/slow":
			time.Sleep(200 * time.Millisecond)
			_, _ = w.Write([]byte("late"))
		default:
			http.Error(w, "boom", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := testClient(true)

	response, err := client.Fetch(t.Context(), Request{URL: server.URL + "/list.m3u", MaxBytes: 1024})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(response.Body) != "#EXTM3U\n" || response.ETag != `"v1"` || response.ContentType != "audio/x-mpegurl" {
		t.Fatalf("response = %+v", response)
	}

	notModified, err := client.Fetch(t.Context(), Request{URL: server.URL + "/etag", ETag: `"v1"`})
	if err != nil {
		t.Fatalf("Fetch 304: %v", err)
	}
	if !notModified.NotModified || notModified.Body != nil {
		t.Fatalf("304 response = %+v", notModified)
	}

	redirected, err := client.Fetch(t.Context(), Request{URL: server.URL + "/redirect"})
	if err != nil {
		t.Fatalf("Fetch redirect: %v", err)
	}
	if !strings.HasSuffix(redirected.FinalURL, "/list.m3u") {
		t.Fatalf("final URL %q", redirected.FinalURL)
	}

	if _, err := client.Fetch(t.Context(), Request{URL: server.URL + "/bad-redirect"}); !errors.Is(err, ErrUnsupportedScheme) {
		t.Fatalf("bad redirect error %v, want ErrUnsupportedScheme", err)
	}
	if _, err := client.Fetch(t.Context(), Request{URL: server.URL + "/large", MaxBytes: 10}); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("large error %v, want ErrResponseTooLarge", err)
	}
	if _, err := client.Fetch(t.Context(), Request{URL: server.URL + "/missing"}); !errors.Is(err, ErrStatus) {
		t.Fatalf("status error %v, want ErrStatus", err)
	}

	slow := New(Policy{AllowPrivate: true, RequestTimeout: 30 * time.Millisecond})
	if _, err := slow.Fetch(t.Context(), Request{URL: server.URL + "/slow"}); err == nil {
		t.Fatal("slow request succeeded, want timeout")
	}
}

func TestFetchBlocksPrivateDestination(t *testing.T) {
	client := testClient(false)

	if _, err := client.Fetch(t.Context(), Request{URL: "http://127.0.0.1:1/list.m3u"}); !errors.Is(err, ErrPrivateAddress) {
		t.Fatalf("error %v, want ErrPrivateAddress", err)
	}
}
