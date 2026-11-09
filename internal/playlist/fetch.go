package playlist

import (
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"mumu/internal/catalog"
	"mumu/internal/netguard"
)

const (
	defaultMaxBytes = 8 << 20
	sniffBytes      = 64 << 10
)

// Loader reads playlists through netguard and classifies the body.
type Loader struct {
	clients map[bool]*netguard.Client
}

// NewLoader returns a loader whose catalog client follows policy. A separate
// client that allows private addresses serves sources the user chose.
func NewLoader(policy netguard.Policy) *Loader {
	policy = policy.WithDefaults()
	userPolicy := policy
	userPolicy.AllowPrivate = true

	return &Loader{
		clients: map[bool]*netguard.Client{
			false: netguard.New(policy),
			true:  netguard.New(userPolicy),
		},
	}
}

// LoadOptions bounds one source read.
type LoadOptions struct {
	AllowPrivate   bool
	MaxBytes       int64
	ETag           string
	DefaultCountry string
}

// LoadResult is a parsed source, or the classification of a single stream.
type LoadResult struct {
	Result
	NotModified bool
	ETag        string
	FinalURL    string
	ContentType string
	StatusCode  int
}

// StreamInfo classifies a direct stream URL without downloading the media.
type StreamInfo struct {
	Kind        Kind
	ContentType string
	FinalURL    string
	StatusCode  int
}

// Load reads a local file or a remote URL and normalizes a channel list.
func (l *Loader) Load(ctx context.Context, location string, opts LoadOptions) (LoadResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.MaxBytes <= 0 || opts.MaxBytes > defaultMaxBytes {
		opts.MaxBytes = defaultMaxBytes
	}

	path, local := localPath(location)
	if local {
		data, err := readLocal(path, opts.MaxBytes)
		if err != nil {
			return LoadResult{}, err
		}

		return l.classify(data, "", LoadResult{}, Options{
			DefaultCountry:    opts.DefaultCountry,
			BaseDir:           filepath.Dir(path),
			AllowLocalPaths:   true,
			AllowPrivateHosts: true,
		})
	}

	response, err := l.clients[opts.AllowPrivate].Fetch(ctx, netguard.Request{
		URL:      location,
		MaxBytes: opts.MaxBytes,
		ETag:     opts.ETag,
	})
	if err != nil {
		return LoadResult{}, mapFetchError(err)
	}

	meta := LoadResult{
		ETag:        response.ETag,
		FinalURL:    response.FinalURL,
		ContentType: response.ContentType,
		StatusCode:  response.StatusCode,
	}
	if response.NotModified {
		meta.NotModified = true

		return meta, nil
	}

	return l.classify(response.Body, response.ContentType, meta, Options{
		DefaultCountry:    opts.DefaultCountry,
		AllowPrivateHosts: opts.AllowPrivate,
	})
}

// Inspect classifies a URL as an HLS manifest, a direct stream or a channel
// list. It reads only the first bytes and never autoplays.
func (l *Loader) Inspect(ctx context.Context, rawURL string, allowPrivate bool) (StreamInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	client := l.clients[allowPrivate]
	response, err := client.Fetch(ctx, netguard.Request{
		URL:      rawURL,
		MaxBytes: sniffBytes,
		Range:    "bytes=0-65535",
		Accept:   "*/*",
	})
	if errors.Is(err, netguard.ErrStatus) {
		// A server that refuses ranges may still answer a plain GET.
		response, err = client.Fetch(ctx, netguard.Request{
			URL:      rawURL,
			MaxBytes: sniffBytes,
			Accept:   "*/*",
		})
	}
	if err != nil {
		return StreamInfo{}, mapFetchError(err)
	}

	kind := Detect(response.Body, response.ContentType)
	info := StreamInfo{
		Kind:        kind,
		ContentType: response.ContentType,
		FinalURL:    response.FinalURL,
		StatusCode:  response.StatusCode,
	}

	switch kind {
	case KindHLSManifest, KindDirectStream:
		return info, nil
	case KindChannelList:
		return info, &Error{
			Code:    CodeStreamNotPlaylist,
			Err:     ErrPlaylistNotStream,
			Message: "URL points to a channel playlist; add it as a playlist",
		}
	default:
		return info, &Error{
			Code:    CodeUnsupported,
			Err:     ErrUnsupported,
			Message: "URL is not a supported stream",
		}
	}
}

func (l *Loader) classify(data []byte, contentType string, meta LoadResult, opts Options) (LoadResult, error) {
	kind := Detect(data, contentType)
	meta.Kind = kind
	meta.Channels = []catalog.Channel{}
	meta.Warnings = []Warning{}

	switch kind {
	case KindHLSManifest:
		text, _ := decodeText(data)
		meta.Name = playlistName(text)

		return meta, nil
	case KindDirectStream:
		return meta, nil
	case KindChannelList:
		parsed, err := Parse(data, opts)
		meta.Result = parsed
		if err != nil {
			return meta, err
		}

		return meta, nil
	default:
		return meta, &Error{
			Code:    CodeUnsupported,
			Err:     ErrUnsupported,
			Message: "source is not a playlist or supported media",
		}
	}
}

func localPath(location string) (string, bool) {
	location = strings.TrimSpace(location)
	if strings.HasPrefix(strings.ToLower(location), "file://") {
		parsed, err := url.Parse(location)
		if err != nil {
			return "", true
		}

		return parsed.Path, true
	}
	if !strings.Contains(location, "://") {
		return location, true
	}

	return "", false
}

func readLocal(path string, maxBytes int64) ([]byte, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, &Error{Code: CodeUnreadable, Err: ErrUnreadable, Message: "local path is empty"}
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, &Error{Code: CodeUnreadable, Err: errors.Join(ErrUnreadable, err), Message: "cannot open the selected file"}
	}
	if info.IsDir() {
		return nil, &Error{Code: CodeUnreadable, Err: ErrUnreadable, Message: "selected path is a directory"}
	}
	if info.Size() > maxBytes {
		return nil, &Error{Code: CodeTooLarge, Err: ErrTooLarge, Message: "selected file is too large"}
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, &Error{Code: CodeUnreadable, Err: errors.Join(ErrUnreadable, err), Message: "cannot open the selected file"}
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, &Error{Code: CodeUnreadable, Err: errors.Join(ErrUnreadable, err), Message: "cannot read the selected file"}
	}
	if int64(len(data)) > maxBytes {
		return nil, &Error{Code: CodeTooLarge, Err: ErrTooLarge, Message: "selected file is too large"}
	}

	return data, nil
}

func mapFetchError(err error) error {
	var playlistErr *Error
	if errors.As(err, &playlistErr) {
		return err
	}

	switch {
	case errors.Is(err, netguard.ErrResponseTooLarge):
		return &Error{Code: CodeTooLarge, Err: errors.Join(ErrTooLarge, err), Message: "source is larger than the limit"}
	case errors.Is(err, netguard.ErrUnsupportedScheme), errors.Is(err, netguard.ErrInvalidURL):
		return &Error{Code: CodeInvalidURL, Err: errors.Join(ErrUnsupported, err), Message: "source URL is invalid"}
	case errors.Is(err, netguard.ErrPrivateAddress):
		return &Error{Code: CodePrivateAddress, Err: errors.Join(ErrUnsupported, err), Message: "local or private source address is not allowed"}
	case errors.Is(err, netguard.ErrCredentials):
		return &Error{Code: CodeCredentials, Err: errors.Join(ErrUnsupported, err), Message: "source URL embeds credentials"}
	case errors.Is(err, netguard.ErrStatus):
		return &Error{Code: CodeUnsupported, Err: errors.Join(ErrUnsupported, err), Message: "source server returned an error"}
	default:
		return &Error{Code: CodeUnreadable, Err: errors.Join(ErrUnreadable, err), Message: "cannot read source"}
	}
}
