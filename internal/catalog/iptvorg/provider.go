// Package iptvorg reads the public iptv-org country playlists and catalog
// metadata.
//
// The project is a discovery dataset, not a rights clearance. mumu presents
// its channels as a community list, never as verified or online.
package iptvorg

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"mumu/internal/netguard"
)

const (
	// BaseURL is the public iptv-org data host.
	BaseURL = "https://iptv-org.github.io"
	// CountriesURL lists the supported regions.
	CountriesURL = BaseURL + "/api/countries.json"
	// BlocklistURL lists channels iptv-org excludes for NSFW or DMCA reasons.
	BlocklistURL = BaseURL + "/api/blocklist.json"
	// Attribution is shown beside the community catalog.
	Attribution = "Channel data from iptv-org (CC0). Streams are hosted by their owners."

	countryPlaylistURL = BaseURL + "/iptv/countries/%s.m3u"
	maxCountriesBytes  = 512 << 10
	maxBlocklistBytes  = 512 << 10
	maxCountries       = 400
	maxBlockEntries    = 10000
	countryCodeLength  = 2
)

// BlockEntry is one upstream exclusion.
type BlockEntry struct {
	Channel string `json:"channel"`
	Reason  string `json:"reason"`
	Ref     string `json:"ref"`
}

// Country is one selectable region.
type Country struct {
	Code string
	Name string
	Flag string
}

// Provider builds iptv-org URLs and reads its bounded metadata.
type Provider struct {
	client  *netguard.Client
	baseURL string
}

// NewProvider returns a provider using the public host.
func NewProvider(client *netguard.Client) *Provider {
	return NewProviderWithBase(client, BaseURL)
}

// NewProviderWithBase returns a provider with an alternate host, for tests.
func NewProviderWithBase(client *netguard.Client, baseURL string) *Provider {
	if client == nil {
		panic("iptvorg: nil client")
	}

	return &Provider{
		client:  client,
		baseURL: strings.TrimSuffix(baseURL, "/"),
	}
}

// PlaylistURL returns the country playlist URL for code.
func (p *Provider) PlaylistURL(code string) (string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !validCountryCode(code) {
		return "", fmt.Errorf("%w: country code %q", ErrInvalidCountry, code)
	}

	return fmt.Sprintf(p.baseURL+"/iptv/countries/%s.m3u", strings.ToLower(code)), nil
}

// Countries returns the region list, sorted by name. Malformed or duplicate
// records are ignored instead of failing the whole list.
func (p *Provider) Countries(ctx context.Context) ([]Country, error) {
	response, err := p.client.Fetch(ctx, netguard.Request{
		URL:      p.baseURL + "/api/countries.json",
		MaxBytes: maxCountriesBytes,
		Accept:   "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("fetch countries: %w", err)
	}

	return ParseCountries(response.Body)
}

// BlockedChannels returns the upstream exclusion list. It is a policy input,
// not a rights review.
func (p *Provider) BlockedChannels(ctx context.Context) ([]BlockEntry, error) {
	response, err := p.client.Fetch(ctx, netguard.Request{
		URL:      p.baseURL + "/api/blocklist.json",
		MaxBytes: maxBlocklistBytes,
		Accept:   "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("fetch blocklist: %w", err)
	}

	return ParseBlockEntries(response.Body)
}

// ParseBlockEntries decodes the iptv-org exclusion list.
func ParseBlockEntries(data []byte) ([]BlockEntry, error) {
	var entries []BlockEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidMetadata, err)
	}
	if len(entries) > maxBlockEntries {
		entries = entries[:maxBlockEntries]
	}

	seen := make(map[string]struct{}, len(entries))
	clean := make([]BlockEntry, 0, len(entries))
	for _, entry := range entries {
		channel := strings.TrimSpace(entry.Channel)
		if channel == "" || len(channel) > 512 || hasControl(channel) {
			continue
		}
		if _, exists := seen[channel]; exists {
			continue
		}
		seen[channel] = struct{}{}
		clean = append(clean, BlockEntry{
			Channel: channel,
			Reason:  strings.TrimSpace(entry.Reason),
			Ref:     strings.TrimSpace(entry.Ref),
		})
	}

	return clean, nil
}

func hasControl(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}

	return false
}

// ParseCountries decodes the iptv-org country list.
func ParseCountries(data []byte) ([]Country, error) {
	var records []struct {
		Name string `json:"name"`
		Code string `json:"code"`
		Flag string `json:"flag"`
	}
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidMetadata, err)
	}
	if len(records) > maxCountries {
		records = records[:maxCountries]
	}

	seen := make(map[string]struct{}, len(records))
	countries := make([]Country, 0, len(records))
	for _, record := range records {
		code := strings.ToUpper(strings.TrimSpace(record.Code))
		if !validCountryCode(code) {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		name := strings.TrimSpace(record.Name)
		if name == "" {
			name = code
		}
		seen[code] = struct{}{}
		countries = append(countries, Country{
			Code: code,
			Name: name,
			Flag: strings.TrimSpace(record.Flag),
		})
	}
	if len(countries) == 0 {
		return nil, ErrInvalidMetadata
	}

	sort.Slice(countries, func(i, j int) bool {
		return strings.ToLower(countries[i].Name) < strings.ToLower(countries[j].Name)
	})

	return countries, nil
}

func validCountryCode(code string) bool {
	if len(code) != countryCodeLength {
		return false
	}
	for _, r := range code {
		if r < 'A' || r > 'Z' {
			return false
		}
	}

	return true
}
