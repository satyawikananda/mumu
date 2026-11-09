package catalog

import "strings"

// Filter narrows a catalog query. Zero values mean "no restriction" except
// EnabledOnly, which callers set explicitly.
type Filter struct {
	Query       string
	Country     string
	Category    string
	SourceID    string
	EnabledOnly bool
	Limit       int
	Offset      int
}

// Match reports whether a channel passes the text, country, category and
// source parts of the filter. Limit and Offset apply to a query, not a row.
func (f Filter) Match(channel Channel) bool {
	if f.SourceID != "" && channel.SourceID != f.SourceID {
		return false
	}
	if f.Country != "" && !strings.EqualFold(channel.Country, f.Country) {
		return false
	}
	if f.Category != "" && !strings.EqualFold(channel.Group, f.Category) {
		return false
	}
	if f.Query == "" {
		return true
	}

	query := strings.ToLower(f.Query)

	return strings.Contains(strings.ToLower(channel.Name), query) ||
		strings.Contains(strings.ToLower(channel.Group), query) ||
		strings.Contains(strings.ToLower(channel.Country), query)
}
