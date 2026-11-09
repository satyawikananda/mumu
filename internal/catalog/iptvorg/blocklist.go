package iptvorg

import (
	_ "embed"
	"encoding/json"
	"net/url"
	"strings"

	"mumu/internal/catalog"
)

//go:embed blocklist.json
var blocklistData []byte

// Blocklist removes channels and hosts that must not appear in the community
// catalog. It starts empty; entries are added as they are identified.
type Blocklist struct {
	Channels map[string]struct{}
	Hosts    []string
}

// DefaultBlocklist returns the reviewed local blocklist.
func DefaultBlocklist() (Blocklist, error) {
	return ParseBlocklist(blocklistData)
}

// ParseBlocklist decodes a blocklist document.
func ParseBlocklist(data []byte) (Blocklist, error) {
	var document struct {
		Channels []string `json:"channels"`
		Hosts    []string `json:"hosts"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return Blocklist{}, ErrInvalidMetadata
	}

	blocked := Blocklist{
		Channels: make(map[string]struct{}, len(document.Channels)),
		Hosts:    make([]string, 0, len(document.Hosts)),
	}
	for _, id := range document.Channels {
		id = strings.TrimSpace(id)
		if id != "" {
			blocked.Channels[id] = struct{}{}
		}
	}
	for _, host := range document.Hosts {
		host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
		if host != "" {
			blocked.Hosts = append(blocked.Hosts, host)
		}
	}

	return blocked, nil
}

// Blocks reports whether the channel or any of its streams is excluded.
func (b Blocklist) Blocks(channel catalog.Channel) bool {
	if b.hasChannel(channel.ProviderID) || b.hasChannel(channel.EPGID) {
		return true
	}
	for _, stream := range channel.Streams {
		if b.blocksURL(stream.URL) {
			return true
		}
	}

	return false
}

func (b Blocklist) hasChannel(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	if _, exists := b.Channels[id]; exists {
		return true
	}

	// Playlist IDs often add a quality suffix, while the blocklist uses the
	// base channel ID.
	if base, _, found := strings.Cut(id, "@"); found {
		_, exists := b.Channels[base]

		return exists
	}

	return false
}

// Filter removes blocked channels and returns how many it dropped.
func (b Blocklist) Filter(channels []catalog.Channel) ([]catalog.Channel, int) {
	kept := make([]catalog.Channel, 0, len(channels))
	blocked := 0

	for _, channel := range channels {
		if b.Blocks(channel) {
			blocked++

			continue
		}
		kept = append(kept, channel)
	}

	return kept, blocked
}

// WithEntries returns a copy of b extended with upstream exclusions.
func (b Blocklist) WithEntries(entries []BlockEntry) Blocklist {
	if len(entries) == 0 {
		return b
	}

	channels := make(map[string]struct{}, len(b.Channels)+len(entries))
	for channel := range b.Channels {
		channels[channel] = struct{}{}
	}
	for _, entry := range entries {
		if entry.Channel != "" {
			channels[entry.Channel] = struct{}{}
		}
	}

	return Blocklist{
		Channels: channels,
		Hosts:    append([]string(nil), b.Hosts...),
	}
}

func (b Blocklist) blocksURL(raw string) bool {
	if len(b.Hosts) == 0 {
		return false
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" {
		return false
	}

	for _, blocked := range b.Hosts {
		if host == blocked || strings.HasSuffix(host, "."+blocked) {
			return true
		}
	}

	return false
}
