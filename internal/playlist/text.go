package playlist

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// decodeText strips a UTF-8 BOM and falls back to Windows-1252 for the
// legacy encodings common in hand-maintained IPTV lists. The second result
// reports that fallback happened.
func decodeText(data []byte) (string, bool) {
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if utf8.Valid(data) {
		return string(data), false
	}

	decoded, err := charmap.Windows1252.NewDecoder().Bytes(data)
	if err != nil {
		return string(data), true
	}

	return string(decoded), true
}

func playlistName(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if len(line) >= len("#PLAYLIST:") && strings.EqualFold(line[:len("#PLAYLIST:")], "#PLAYLIST:") {
			return strings.TrimSpace(line[len("#PLAYLIST:"):])
		}
	}

	return ""
}
