package catalog

import (
	"slices"
	"testing"
)

func TestNormalizeSchemes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  []string
		wanted []string
	}{
		{name: "nil is strict", input: nil, wanted: []string{"http", "https"}},
		{name: "empty is strict", input: []string{}, wanted: []string{"http", "https"}},
		{
			name:   "sorted lowercased deduplicated",
			input:  []string{"HTTPS", " http ", "https", "", "HTTP"},
			wanted: []string{"http", "https"},
		},
		{
			name:   "blocked transports are dropped",
			input:  []string{"rtsp", "rtmp", "srt", "udp", "rtp", "multicast", "file", "HTTP"},
			wanted: []string{"http", "https"},
		},
		{
			name:   "an unpublished scheme cannot widen the surface",
			input:  []string{"hls", "dash", "https"},
			wanted: []string{"http", "https"},
		},
		{
			name:   "http and https are always present",
			input:  []string{"rtsp"},
			wanted: []string{"http", "https"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeSchemes(tc.input)
			if !slices.Equal(got, tc.wanted) {
				t.Fatalf("NormalizeSchemes(%v) = %v, want %v", tc.input, got, tc.wanted)
			}
		})
	}
}

func TestNormalizeSchemesIsDeterministic(t *testing.T) {
	input := []string{"https", "HTTP", "HTTPS", "http", "rtsp", "udp"}

	first := NormalizeSchemes(input)
	for range 20 {
		if next := NormalizeSchemes(input); !slices.Equal(first, next) {
			t.Fatalf("NormalizeSchemes is not deterministic: %v vs %v", first, next)
		}
	}
}

func TestSchemeAllowed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		schemes []string
		scheme  string
		want    bool
	}{
		{name: "http allowed by default", scheme: "http", want: true},
		{name: "https allowed by default", scheme: "HTTPS", want: true},
		{name: "file never allowed", scheme: "file", want: false},
		{name: "file never allowed by a list", schemes: []string{"file"}, scheme: "file", want: false},
		{name: "udp never allowed", schemes: []string{"udp"}, scheme: "udp", want: false},
		{name: "rtp never allowed", schemes: []string{"rtp"}, scheme: "rtp", want: false},
		{name: "rtsp never allowed", schemes: []string{"rtsp"}, scheme: "rtsp", want: false},
		{name: "srt never allowed", schemes: []string{"srt"}, scheme: "srt", want: false},
		{name: "empty scheme", scheme: "", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := SchemeAllowed(tc.schemes, tc.scheme); got != tc.want {
				t.Fatalf("SchemeAllowed(%v, %q) = %v, want %v", tc.schemes, tc.scheme, got, tc.want)
			}
		})
	}
}

func TestSchemeSupported(t *testing.T) {
	for _, tc := range []struct {
		scheme string
		want   bool
	}{
		{scheme: "http", want: true},
		{scheme: "HTTPS", want: true},
		{scheme: "rtsp"},
		{scheme: "rtmp"},
		{scheme: "srt"},
		{scheme: "udp"},
		{scheme: "file"},
		{scheme: ""},
	} {
		if got := SchemeSupported(tc.scheme); got != tc.want {
			t.Fatalf("SchemeSupported(%q) = %v, want %v", tc.scheme, got, tc.want)
		}
	}
}

func TestSourceEffectivePolicy(t *testing.T) {
	builtin := Source{
		Kind:           SourceBuiltin,
		AllowPrivate:   true,
		AllowedSchemes: []string{"rtsp", "http"},
	}
	policy := builtin.EffectivePolicy()
	if policy.AllowPrivate {
		t.Fatal("built-in source kept a consent")
	}
	if !slices.Equal(policy.Schemes, []string{"http", "https"}) {
		t.Fatalf("built-in schemes = %v, want http https", policy.Schemes)
	}

	user := Source{
		Kind:           SourceCustomStream,
		AllowPrivate:   true,
		AllowedSchemes: []string{"https", "http", "rtsp"},
	}
	policy = user.EffectivePolicy()
	if !policy.AllowPrivate {
		t.Fatal("user source lost its consent")
	}
	if !slices.Equal(policy.Schemes, []string{"http", "https"}) {
		t.Fatalf("user schemes = %v, want the blocked scheme removed", policy.Schemes)
	}
	if !policy.AllowsScheme("http") || policy.AllowsScheme("rtsp") {
		t.Fatalf("policy = %+v, want http only", policy)
	}
}

func TestStrictPolicyIsACopy(t *testing.T) {
	policy := StrictPolicy()
	policy.Schemes[0] = "rtsp"

	if next := StrictPolicy(); next.Schemes[0] != "http" {
		t.Fatalf("StrictPolicy shares its slice: %v", next.Schemes)
	}
}
