package libmpv

import "testing"

func TestHeaderOptionValue(t *testing.T) {
	for _, tc := range []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{name: "nil", headers: nil, want: ""},
		{name: "empty", headers: map[string]string{}, want: ""},
		{
			name:    "user agent",
			headers: map[string]string{"User-Agent": "mumu/0.1"},
			want:    "User-Agent: mumu/0.1",
		},
		{
			name:    "case insensitive names",
			headers: map[string]string{"user-agent": "mumu/0.1", "REFERER": "https://example.com/"},
			want:    "User-Agent: mumu/0.1,Referer: https://example.com/",
		},
		{
			name:    "playlist spellings",
			headers: map[string]string{"http-user-agent": "mumu/0.1", "http-referrer": "https://example.com/watch"},
			want:    "User-Agent: mumu/0.1,Referer: https://example.com/watch",
		},
		{
			name:    "fixed order regardless of map order",
			headers: map[string]string{"Referer": "https://example.com/", "User-Agent": "mumu/0.1"},
			want:    "User-Agent: mumu/0.1,Referer: https://example.com/",
		},
		{
			name:    "other headers are dropped",
			headers: map[string]string{"Cookie": "session=1", "Authorization": "Bearer x", "User-Agent": "mumu/0.1"},
			want:    "User-Agent: mumu/0.1",
		},
		{
			name:    "only rejected headers",
			headers: map[string]string{"Cookie": "session=1", "Proxy-Authorization": "Basic x"},
			want:    "",
		},
		{
			name:    "empty value is dropped",
			headers: map[string]string{"User-Agent": "   ", "Referer": "https://example.com/"},
			want:    "Referer: https://example.com/",
		},
		{
			name:    "carriage return is rejected",
			headers: map[string]string{"User-Agent": "mumu\rInjected: 1"},
			want:    "",
		},
		{
			name:    "line feed is rejected",
			headers: map[string]string{"Referer": "https://example.com/\nInjected: 1"},
			want:    "",
		},
		{
			name:    "other control characters are rejected",
			headers: map[string]string{"User-Agent": "mumu\x00", "Referer": "https://example.com/"},
			want:    "Referer: https://example.com/",
		},
		{
			name:    "comma would split the list",
			headers: map[string]string{"User-Agent": "mumu,Other: 1"},
			want:    "",
		},
		{
			name:    "backslash would escape the list",
			headers: map[string]string{"Referer": "https://example.com/\\"},
			want:    "",
		},
		{
			name:    "an over long value is rejected",
			headers: map[string]string{"User-Agent": longValue(maxHeaderFieldValue + 1)},
			want:    "",
		},
		{
			name:    "an exact length value is kept",
			headers: map[string]string{"User-Agent": longValue(maxHeaderFieldValue)},
			want:    "User-Agent: " + longValue(maxHeaderFieldValue),
		},
		{
			name:    "duplicate spellings resolve deterministically",
			headers: map[string]string{"User-Agent": "first", "user-agent": "second"},
			want:    "User-Agent: first",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := headerOptionValue(tc.headers); got != tc.want {
				t.Fatalf("headerOptionValue(%v) = %q, want %q", tc.headers, got, tc.want)
			}
		})
	}
}

func TestHeaderOptionValueIsDeterministic(t *testing.T) {
	headers := map[string]string{"User-Agent": "a", "Referer": "b", "Cookie": "c"}

	first := headerOptionValue(headers)
	for range 50 {
		if got := headerOptionValue(headers); got != first {
			t.Fatalf("headerOptionValue is not deterministic: %q vs %q", first, got)
		}
	}
}

func longValue(length int) string {
	value := make([]byte, length)
	for i := range value {
		value[i] = 'a'
	}

	return string(value)
}
