// Package logos fetches and bounds channel logo images.
//
// Decoding and scaling happen off the UI thread. The result is an image.Image;
// the native UI converts it to a bitmap on the main thread and falls back to
// initials when a logo is missing or unsupported.
package logos

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // Decode
	_ "image/jpeg" // Decode
	_ "image/png"  // Decode
	"strings"
	"sync"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // Decode

	"mumu/internal/netguard"
)

const (
	maxLogoBytes     = 2 << 20
	maxLogoDimension = 4096
	maxLogoPixels    = 4_000_000
	maxLogoSize      = 128
	defaultCacheSize = 128
)

// Sentinel errors for logo loading.
var (
	// ErrNotImage reports a response that is not a supported raster image.
	ErrNotImage = errors.New("logo is not a supported image")
	// ErrImageTooLarge reports image dimensions or pixels over the limit.
	ErrImageTooLarge = errors.New("logo image is too large")
)

// Fetcher loads logos with a bounded response, decode and memory cache.
type Fetcher struct {
	client  *netguard.Client
	mu      sync.Mutex
	cache   map[string]*list.Element
	order   *list.List
	pending map[string]*call
	limit   int
}

type cachedLogo struct {
	url   string
	image image.Image
}

type call struct {
	done  chan struct{}
	image image.Image
	err   error
}

// Option customizes a Fetcher.
type Option func(*Fetcher)

// WithCacheSize bounds how many decoded logos are kept.
func WithCacheSize(size int) Option {
	return func(f *Fetcher) {
		if size > 0 {
			f.limit = size
		}
	}
}

// NewFetcher returns a fetcher using policy. Catalog logos should keep private
// networks disabled.
func NewFetcher(policy netguard.Policy, opts ...Option) *Fetcher {
	fetcher := &Fetcher{
		client:  netguard.New(policy),
		cache:   map[string]*list.Element{},
		order:   list.New(),
		pending: map[string]*call{},
		limit:   defaultCacheSize,
	}
	for _, opt := range opts {
		opt(fetcher)
	}

	return fetcher
}

// Load returns a downscaled logo, reusing a cached decode when possible.
func (f *Fetcher) Load(ctx context.Context, rawURL string) (image.Image, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("%w: empty logo URL", netguard.ErrInvalidURL)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	logo, wait, owner := f.begin(rawURL)
	if logo != nil {
		return logo, nil
	}
	if !owner {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-wait.done:
			return wait.image, wait.err
		}
	}

	imageData, err := f.fetch(ctx, rawURL)
	f.complete(rawURL, wait, imageData, err)
	if err != nil {
		return nil, err
	}

	return imageData, nil
}

func (f *Fetcher) begin(rawURL string) (image.Image, *call, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if element, exists := f.cache[rawURL]; exists {
		f.order.MoveToFront(element)
		logo := element.Value.(*cachedLogo)

		return logo.image, nil, false
	}
	if pending, exists := f.pending[rawURL]; exists {
		return nil, pending, false
	}

	pending := &call{done: make(chan struct{})}
	f.pending[rawURL] = pending

	return nil, pending, true
}

func (f *Fetcher) complete(rawURL string, pending *call, logo image.Image, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	pending.image = logo
	pending.err = err
	if err == nil {
		f.storeLocked(rawURL, logo)
	}
	delete(f.pending, rawURL)
	close(pending.done)
}

func (f *Fetcher) storeLocked(rawURL string, logo image.Image) {
	if element, exists := f.cache[rawURL]; exists {
		f.order.MoveToFront(element)

		return
	}

	element := f.order.PushFront(&cachedLogo{url: rawURL, image: logo})
	f.cache[rawURL] = element
	if f.order.Len() <= f.limit {
		return
	}

	oldest := f.order.Back()
	if oldest == nil {
		return
	}
	f.order.Remove(oldest)
	entry := oldest.Value.(*cachedLogo)
	delete(f.cache, entry.url)
}

func (f *Fetcher) fetch(ctx context.Context, rawURL string) (image.Image, error) {
	response, err := f.client.Fetch(ctx, netguard.Request{
		URL:      rawURL,
		MaxBytes: maxLogoBytes,
		Accept:   "image/*",
	})
	if err != nil {
		return nil, err
	}
	if !looksLikeImage(response.ContentType) {
		return nil, fmt.Errorf("%w: %s", ErrNotImage, response.ContentType)
	}

	config, _, err := image.DecodeConfig(bytes.NewReader(response.Body))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotImage, err)
	}
	if config.Width <= 0 || config.Height <= 0 ||
		config.Width > maxLogoDimension || config.Height > maxLogoDimension ||
		config.Width*config.Height > maxLogoPixels {
		return nil, ErrImageTooLarge
	}

	decoded, _, err := image.Decode(bytes.NewReader(response.Body))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotImage, err)
	}

	return scale(decoded), nil
}

func looksLikeImage(contentType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if mediaType == "" || mediaType == "application/octet-stream" {
		return true
	}

	return strings.HasPrefix(mediaType, "image/") && mediaType != "image/svg+xml"
}

func scale(source image.Image) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= maxLogoSize && height <= maxLogoSize {
		return source
	}

	scaleRatio := float64(maxLogoSize) / float64(max(width, height))
	targetWidth := max(1, int(float64(width)*scaleRatio))
	targetHeight := max(1, int(float64(height)*scaleRatio))

	target := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	draw.CatmullRom.Scale(target, target.Bounds(), source, bounds, draw.Over, nil)

	return target
}
