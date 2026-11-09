package playlist

import "testing"

func TestDetect(t *testing.T) {
	ts := make([]byte, 376)
	ts[0] = 0x47
	ts[188] = 0x47

	for _, tc := range []struct {
		name        string
		body        string
		contentType string
		want        Kind
	}{
		{name: "channel list", body: "#EXTM3U\n#EXTINF:-1,One\nhttps://x/one.m3u8\n", want: KindChannelList},
		{name: "hls", body: "#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\nseg.ts\n", want: KindHLSManifest},
		{name: "list with mpegurl type", body: "#EXTM3U\n#EXTINF:-1,One\nhttps://x/one.m3u8\n", contentType: "audio/x-mpegurl", want: KindChannelList},
		{name: "hls content type", body: "#EXTM3U\n", contentType: "application/vnd.apple.mpegurl", want: KindHLSManifest},
		{name: "video type", body: "random", contentType: "video/mp4", want: KindDirectStream},
		{name: "html", body: "<html><body>no</body></html>", contentType: "text/html", want: KindUnknown},
		{name: "empty", body: "", want: KindUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Detect([]byte(tc.body), tc.contentType); got != tc.want {
				t.Fatalf("Detect = %v, want %v", got, tc.want)
			}
		})
	}

	if got := Detect(ts, ""); got != KindDirectStream {
		t.Fatalf("TS detect = %v, want direct stream", got)
	}
	if got := Detect([]byte("\x00\x00\x00\x18ftypmp42"), ""); got != KindDirectStream {
		t.Fatalf("MP4 detect = %v, want direct stream", got)
	}
}
