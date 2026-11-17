package player

import (
	"errors"
	"testing"
)

// The playback boundary re-applies the source policy, so a private host that
// never carried user consent is rejected before it reaches the engine.
func TestNormalizeRequestRejectsPrivateHostWithoutConsent(t *testing.T) {
	_, err := normalizeRequest(Request{Variants: []Variant{{
		Source: Source{URL: "http://127.0.0.1:8080/live.m3u8"},
	}}})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v, want ErrInvalid", err)
	}
}

// A source whose stored policy carries explicit consent may use a private host.
func TestNormalizeRequestAllowsPrivateHostWithConsent(t *testing.T) {
	request, err := normalizeRequest(Request{Variants: []Variant{{
		Source: Source{URL: "http://127.0.0.1:8080/live.m3u8", AllowPrivate: true},
	}}})
	if err != nil {
		t.Fatalf("normalizeRequest: %v", err)
	}
	if len(request.Variants) != 1 || !request.Variants[0].Source.AllowPrivate {
		t.Fatalf("request %+v, want the consented source", request)
	}
}

// URL credentials are refused at the playback boundary as well as at import.
func TestNormalizeRequestRejectsCredentials(t *testing.T) {
	_, err := normalizeRequest(Request{Variants: []Variant{{
		Source: Source{URL: "https://user:secret@stream.example/live.m3u8"},
	}}})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v, want ErrInvalid", err)
	}
}

// The existing scheme gate still rejects a non-HTTP source.
func TestNormalizeRequestRejectsUnsupportedScheme(t *testing.T) {
	_, err := normalizeRequest(Request{Variants: []Variant{{
		Source: Source{URL: "file:///etc/passwd"},
	}}})
	if !errors.Is(err, ErrUnsupportedScheme) && !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v, want a scheme rejection", err)
	}
}
