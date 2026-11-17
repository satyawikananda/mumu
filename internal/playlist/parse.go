package playlist

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"mumu/internal/catalog"
	"mumu/internal/netguard"
)

type entry struct {
	line    int
	attrs   map[string]string
	headers map[string]string
	name    string
}

type parser struct {
	opts       Options
	result     Result
	seenHeader bool
	pending    *entry
	byProvider map[string]int
}

// Parse reads a channel list or an HLS manifest. An HLS body returns
// KindHLSManifest without channels; callers treat it as a single stream.
func Parse(data []byte, opts Options) (Result, error) {
	opts = opts.WithDefaults()
	text, fallback := decodeText(data)

	result := Result{
		Kind:           KindChannelList,
		Channels:       []catalog.Channel{},
		Warnings:       []Warning{},
		SkippedReasons: map[Code]int{},
		SkippedEntries: []SkippedEntry{},
	}

	switch Detect(data, "") {
	case KindHLSManifest:
		result.Kind = KindHLSManifest
		result.Name = playlistName(text)

		return result, nil
	case KindChannelList:
	default:
		return Result{Kind: KindUnknown, Channels: []catalog.Channel{}},
			&Error{Code: CodeUnsupported, Err: ErrUnsupported, Message: "source is not a channel playlist"}
	}

	p := &parser{
		opts:       opts,
		result:     result,
		byProvider: map[string]int{},
	}
	if fallback {
		p.warn(0, CodeEncodingFallback, "playlist is not UTF-8; decoded as Windows-1252")
	}

	if err := p.run(text); err != nil {
		return p.result, err
	}

	return p.result, nil
}

func (p *parser) run(text string) error {
	start := 0
	lineNumber := 0

	for start <= len(text) {
		end := strings.IndexByte(text[start:], '\n')
		var raw string
		if end < 0 {
			raw = text[start:]
			start = len(text) + 1
		} else {
			raw = text[start : start+end]
			start += end + 1
		}
		lineNumber++

		raw = strings.TrimSuffix(raw, "\r")
		if len(raw) > p.opts.MaxLineBytes {
			p.warn(lineNumber, CodeMalformedLine, "line is too long")
			continue
		}

		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		p.line(lineNumber, line)
		if p.result.Truncated {
			break
		}
	}

	p.flushPending()

	if !p.seenHeader {
		p.warn(0, CodeMissingHeader, "playlist does not start with #EXTM3U")
	}
	if len(p.result.Channels) == 0 {
		return &Error{
			Code:    CodeEmptyPlaylist,
			Err:     ErrNoChannels,
			Message: "playlist has no usable channels",
		}
	}

	return nil
}

func (p *parser) line(number int, line string) {
	switch {
	case line == "#EXTM3U":
		p.seenHeader = true
	case strings.HasPrefix(line, "#EXTINF:"):
		p.flushPending()
		parsed, err := parseExtInf(line)
		if err != nil {
			p.warn(number, CodeMalformedLine, "cannot read #EXTINF")
			p.recordSkipped(number, "", CodeMalformedLine, "")

			return
		}
		parsed.line = number
		p.applyHeaders(&parsed, parsed.attrs)
		p.pending = &parsed
	case strings.HasPrefix(line, "#EXTGRP:"):
		if p.pending != nil {
			group := strings.TrimSpace(strings.TrimPrefix(line, "#EXTGRP:"))
			if group != "" {
				p.pending.attrs["group-title"] = group
			}
		}
	case strings.HasPrefix(line, "#EXTVLCOPT:"):
		if p.pending != nil {
			key, value, found := strings.Cut(strings.TrimPrefix(line, "#EXTVLCOPT:"), "=")
			if !found {
				p.warn(number, CodeMalformedLine, "cannot read #EXTVLCOPT")
			} else if name, ok := headerName(key); ok {
				if p.pending.headers == nil {
					p.pending.headers = map[string]string{}
				}
				p.pending.headers[name] = strings.TrimSpace(value)
			}
		}
	case strings.HasPrefix(line, "#PLAYLIST:"):
		p.result.Name = strings.TrimSpace(strings.TrimPrefix(line, "#PLAYLIST:"))
	case strings.HasPrefix(line, "#"):
		// Unknown directives are preserved by neither the model nor the UI.
	default:
		p.addURL(number, line)
	}
}

func (p *parser) addURL(number int, raw string) {
	if p.pending == nil {
		p.warn(number, CodeMalformedLine, "URL without #EXTINF")
		p.recordSkipped(number, "", CodeMalformedLine, raw)

		return
	}

	pending := *p.pending
	p.pending = nil

	name := strings.TrimSpace(pending.name)
	if name == "" {
		name = attribute(pending.attrs, "tvg-name")
	}

	streamURL, err := normalizeStream(raw, p.opts)
	if err != nil {
		p.warn(number, codeFor(err), errorMessage(err))
		p.recordSkipped(number, name, codeFor(err), raw)

		return
	}

	if name == "" {
		p.warn(number, CodeMissingName, "channel has no name")
		p.recordSkipped(number, "", CodeMissingName, raw)

		return
	}

	headers, err := catalog.SanitizeHeaders(pending.headers)
	if err != nil {
		p.warn(number, CodeInvalidHeader, "playback headers are invalid")
		p.recordSkipped(number, name, CodeInvalidHeader, raw)

		return
	}

	providerID, err := catalog.ProviderID(attribute(pending.attrs, "tvg-id"), streamURL)
	if err != nil {
		p.warn(number, codeFor(err), errorMessage(err))
		p.recordSkipped(number, name, codeFor(err), raw)

		return
	}

	stream := catalog.Stream{
		URL:     streamURL,
		Quality: qualityOf(name, attribute(pending.attrs, "tvg-id")),
		HLS:     isHLSURL(streamURL),
		Live:    true,
		Headers: headers,
	}

	if index, exists := p.byProvider[providerID]; exists {
		channel := &p.result.Channels[index]
		for _, existing := range channel.Streams {
			if existing.URL == streamURL {
				return
			}
		}
		if channel.Name != name {
			p.warn(number, CodeDuplicateProvider, "same provider ID with a different name")
		}
		channel.Streams = append(channel.Streams, stream)

		return
	}

	if len(p.result.Channels) >= p.opts.MaxChannels {
		p.result.Truncated = true
		p.warn(number, CodeTruncated, fmt.Sprintf("playlist is limited to %d channels", p.opts.MaxChannels))

		return
	}

	country := strings.TrimSpace(attribute(pending.attrs, "tvg-country"))
	if country == "" {
		country = strings.TrimSpace(p.opts.DefaultCountry)
	}
	if len(country) == 2 {
		country = strings.ToUpper(country)
	}

	channel := catalog.Channel{
		ProviderID:   providerID,
		Name:         name,
		Group:        attribute(pending.attrs, "group-title"),
		Country:      country,
		Language:     attribute(pending.attrs, "tvg-language"),
		LogoURL:      attribute(pending.attrs, "tvg-logo"),
		EPGID:        attribute(pending.attrs, "tvg-id"),
		AllowPrivate: p.opts.AllowPrivateHosts,
		Streams:      []catalog.Stream{stream},
	}
	p.result.Channels = append(p.result.Channels, channel)
	p.byProvider[providerID] = len(p.result.Channels) - 1
}

func (p *parser) applyHeaders(e *entry, attrs map[string]string) {
	for key, value := range attrs {
		name, ok := headerName(key)
		if !ok {
			continue
		}
		if e.headers == nil {
			e.headers = map[string]string{}
		}
		e.headers[name] = value
	}
}

func (p *parser) flushPending() {
	if p.pending == nil {
		return
	}

	p.warn(p.pending.line, CodeMissingURL, "channel has no URL")
	p.recordSkipped(p.pending.line, p.pendingChannelName(), CodeMissingURL, "")
	p.pending = nil
}

// recordSkipped counts a skipped entry and keeps a bounded, credential-free
// description for the import preview.
func (p *parser) recordSkipped(line int, name string, code Code, raw string) {
	p.result.Skipped++
	if p.result.SkippedReasons == nil {
		p.result.SkippedReasons = map[Code]int{}
	}
	p.result.SkippedReasons[code]++

	if len(p.result.SkippedEntries) >= maxSkippedEntries {
		return
	}

	scheme, host := describeURL(raw)
	p.result.SkippedEntries = append(p.result.SkippedEntries, SkippedEntry{
		Line:   line,
		Name:   strings.TrimSpace(name),
		Reason: code,
		Scheme: scheme,
		Host:   host,
	})
}

func (p *parser) pendingChannelName() string {
	if p.pending == nil {
		return ""
	}
	if name := strings.TrimSpace(p.pending.name); name != "" {
		return name
	}

	return attribute(p.pending.attrs, "tvg-name")
}

func describeURL(raw string) (scheme, host string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}

	return strings.ToLower(parsed.Scheme), parsed.Hostname()
}

func (p *parser) warn(line int, code Code, message string) {
	if len(p.result.Warnings) >= p.opts.MaxWarnings {
		p.result.OmittedWarnings++

		return
	}

	p.result.Warnings = append(p.result.Warnings, Warning{
		Line:    line,
		Code:    code,
		Message: message,
	})
}

func headerName(key string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "http-referrer", "http-referer", "referrer", "referer":
		return "Referer", true
	case "http-user-agent", "user-agent":
		return "User-Agent", true
	default:
		return "", false
	}
}

func normalizeStream(raw string, opts Options) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("%w: empty URL", ErrMalformed)
	}
	if strings.ContainsAny(raw, "\r\n\x00") {
		return "", fmt.Errorf("%w: URL has control characters", ErrMalformed)
	}

	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("%w: %w", ErrMalformed, err)
		}

		scheme := strings.ToLower(parsed.Scheme)
		switch {
		case scheme == "file":
			// A local path is a local-playlist decision only; a remote
			// source policy never adds it.
			if !opts.AllowLocalPaths {
				return "", netguard.ErrUnsupportedScheme
			}
			if parsed.User != nil {
				return "", netguard.ErrCredentials
			}

			return normalizeLocal(parsed.Path, opts)
		case !catalog.SchemeAllowed(opts.AllowedSchemes, scheme):
			return "", fmt.Errorf("%w: %q", netguard.ErrUnsupportedScheme, scheme)
		default:
			policy := netguard.Policy{AllowPrivate: opts.AllowPrivateHosts}
			validated, err := policy.ValidateURL(raw)
			if err != nil {
				return "", err
			}
			validated.Fragment = ""
			validated.Scheme = strings.ToLower(validated.Scheme)
			validated.Host = strings.ToLower(validated.Host)

			return validated.String(), nil
		}
	}

	if !opts.AllowLocalPaths {
		return "", netguard.ErrUnsupportedScheme
	}

	return normalizeLocal(raw, opts)
}

func normalizeLocal(path string, opts Options) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("%w: empty path", ErrMalformed)
	}
	if !filepath.IsAbs(path) && opts.BaseDir != "" {
		path = filepath.Join(opts.BaseDir, path)
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrMalformed, err)
	}

	return filepath.Clean(absolute), nil
}

func codeFor(err error) Code {
	switch {
	case errors.Is(err, netguard.ErrPrivateAddress):
		return CodePrivateAddress
	case errors.Is(err, netguard.ErrCredentials), errors.Is(err, catalog.ErrCredentials):
		return CodeCredentials
	case errors.Is(err, netguard.ErrUnsupportedScheme), errors.Is(err, catalog.ErrUnsupportedScheme):
		return CodeUnsupportedScheme
	default:
		return CodeInvalidURL
	}
}

func errorMessage(err error) string {
	switch {
	case errors.Is(err, netguard.ErrPrivateAddress):
		return "local or private stream address is not allowed"
	case errors.Is(err, netguard.ErrCredentials):
		return "stream URL embeds credentials"
	case errors.Is(err, netguard.ErrUnsupportedScheme):
		return "stream URL scheme is not supported"
	case errors.Is(err, netguard.ErrInvalidURL):
		return "stream URL is invalid"
	default:
		return "stream URL is invalid"
	}
}

func qualityOf(name, tvgID string) string {
	for i := 0; i < len(name); i++ {
		if name[i] != '(' {
			continue
		}
		for j := i + 1; j < len(name); j++ {
			if name[j] == ')' && j > i+1 && name[j-1] == 'p' {
				return name[i+1 : j]
			}
		}
	}

	if index := strings.LastIndex(tvgID, "@"); index >= 0 && index+1 < len(tvgID) {
		return tvgID[index+1:]
	}

	return ""
}

func isHLSURL(raw string) bool {
	path := raw
	if parsed, err := url.Parse(raw); err == nil {
		path = parsed.Path
	}

	return strings.HasSuffix(strings.ToLower(path), ".m3u8")
}
