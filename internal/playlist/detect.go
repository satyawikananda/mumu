package playlist

import (
	"bytes"
	"strings"
)

const sniffSize = 64 << 10

// Detect classifies a body by its content first and its Content-Type second.
// A .m3u8 extension is not evidence on its own.
func Detect(data []byte, contentType string) Kind {
	sample := data
	if len(sample) > sniffSize {
		sample = sample[:sniffSize]
	}
	trimmed := bytes.TrimSpace(sample)
	hasHeader := bytes.HasPrefix(trimmed, []byte("#EXTM3U"))
	hasExtInf := hasLinePrefix(sample, []byte("#EXTINF"))
	hasHLS := hasLinePrefix(sample, []byte("#EXT-X-"))
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))

	if hasHLS && (hasHeader || hasExtInf) {
		return KindHLSManifest
	}
	if hasExtInf {
		return KindChannelList
	}
	if hasHeader {
		if strings.Contains(mediaType, "mpegurl") {
			return KindHLSManifest
		}

		return KindChannelList
	}

	switch {
	case strings.Contains(mediaType, "mpegurl"):
		return KindHLSManifest
	case strings.HasPrefix(mediaType, "video/"),
		strings.HasPrefix(mediaType, "audio/"),
		mediaType == "application/mp2t",
		mediaType == "application/octet-stream" && looksLikeMedia(sample):
		return KindDirectStream
	}

	if looksLikeMedia(sample) {
		return KindDirectStream
	}
	if looksBinary(sample) {
		return KindDirectStream
	}

	return KindUnknown
}

func hasLinePrefix(data, prefix []byte) bool {
	lineStart := true
	for i := 0; i < len(data); i++ {
		if lineStart && bytes.HasPrefix(data[i:], prefix) {
			return true
		}
		lineStart = data[i] == '\n'
	}

	return false
}

func looksLikeMedia(sample []byte) bool {
	switch {
	case len(sample) >= 189 && sample[0] == 0x47 && sample[188] == 0x47:
		return true // MPEG-TS
	case len(sample) >= 12 && string(sample[4:8]) == "ftyp":
		return true // ISO base media
	case bytes.HasPrefix(sample, []byte("ID3")):
		return true // MP3 with ID3
	case bytes.HasPrefix(sample, []byte{0x1a, 0x45, 0xdf, 0xa3}):
		return true // Matroska/WebM
	case bytes.HasPrefix(sample, []byte{0xff, 0xf1}), bytes.HasPrefix(sample, []byte{0xff, 0xf9}):
		return true // ADTS AAC
	default:
		return false
	}
}

func looksBinary(sample []byte) bool {
	if len(sample) == 0 {
		return false
	}

	control := 0
	for _, b := range sample {
		switch {
		case b == '\n', b == '\r', b == '\t':
		case b < 0x20:
			control++
		}
	}

	return control*20 > len(sample)
}
