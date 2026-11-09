package playlist

import "strings"

// parseExtInf splits "#EXTINF:<duration> <attributes>,<title>". The title may
// contain commas; only a comma outside a quoted attribute value ends the
// attribute list.
func parseExtInf(line string) (entry, error) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "#EXTINF:"))
	if rest == "" {
		return entry{}, ErrMalformed
	}

	attributes := rest
	title := ""
	if index := indexOutsideQuotes(rest, ','); index >= 0 {
		attributes = rest[:index]
		title = strings.TrimSpace(rest[index+1:])
	}
	attributes = strings.TrimSpace(attributes)

	// Drop the duration token when present.
	if space := strings.IndexByte(attributes, ' '); space >= 0 {
		attributes = attributes[space+1:]
	} else if !strings.Contains(attributes, "=") {
		attributes = ""
	}

	return entry{
		attrs: parseAttributes(attributes),
		name:  title,
	}, nil
}

// parseAttributes reads key=value and key="value" pairs. Unknown keys are
// kept; the parser decides which ones it understands.
func parseAttributes(input string) map[string]string {
	attributes := map[string]string{}

	for i := 0; i < len(input); {
		for i < len(input) && (input[i] == ' ' || input[i] == '\t') {
			i++
		}
		if i >= len(input) {
			break
		}

		start := i
		for i < len(input) && input[i] != '=' && input[i] != ' ' && input[i] != '\t' {
			i++
		}
		key := strings.ToLower(strings.TrimSpace(input[start:i]))
		if key == "" || i >= len(input) || input[i] != '=' {
			continue
		}
		i++

		var value string
		if i < len(input) && input[i] == '"' {
			i++
			var builder strings.Builder
			for i < len(input) {
				if input[i] == '\\' && i+1 < len(input) {
					builder.WriteByte(input[i+1])
					i += 2

					continue
				}
				if input[i] == '"' {
					i++

					break
				}
				builder.WriteByte(input[i])
				i++
			}
			value = builder.String()
		} else {
			start = i
			for i < len(input) && input[i] != ' ' && input[i] != '\t' {
				i++
			}
			value = input[start:i]
		}

		if value != "" {
			attributes[key] = value
		}
	}

	return attributes
}

func indexOutsideQuotes(input string, target byte) int {
	quoted := false
	for i := 0; i < len(input); i++ {
		switch input[i] {
		case '\\':
			if quoted {
				i++
			}
		case '"':
			quoted = !quoted
		default:
			if input[i] == target && !quoted {
				return i
			}
		}
	}

	return -1
}

func attribute(attributes map[string]string, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(attributes[strings.ToLower(name)]); value != "" {
			return value
		}
	}

	return ""
}
