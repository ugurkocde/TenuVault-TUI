// Package jsonutil holds small JSON helpers shared by the backup, restore, and
// diff engines: filename sanitizing, display-name extraction, and recursive key
// stripping for noise-insensitive comparison and clean restores.
package jsonutil

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// SanitizeFilename makes a Graph display name safe to use as a file name.
func SanitizeFilename(name string) string {
	if name == "" {
		name = "unnamed"
	}
	repl := func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\n', '\r', '\t':
			return '_'
		}
		return r
	}
	out := strings.Map(repl, name)
	out = strings.TrimSpace(out)
	// Truncate on a rune boundary: byte-slicing could split a multi-byte
	// character (umlauts, CJK) and leave invalid UTF-8 in the filename.
	if len(out) > 180 {
		cut := 180
		for cut > 0 && !utf8.RuneStart(out[cut]) {
			cut--
		}
		out = out[:cut]
		out = strings.TrimSpace(out)
	}
	if out == "" {
		return "unnamed"
	}
	return out
}

// DisplayName extracts a human label from a policy, trying the given field then
// common fallbacks.
func DisplayName(raw json.RawMessage, field string) string {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	for _, key := range []string{field, "displayName", "name", "id"} {
		if key == "" {
			continue
		}
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// StripKeysFunc returns a copy of v with every key for which drop returns true
// removed recursively.
func StripKeysFunc(v any, drop func(string) bool) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if drop(k) {
				continue
			}
			out[k] = StripKeysFunc(val, drop)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = StripKeysFunc(val, drop)
		}
		return out
	default:
		return v
	}
}

// StripKeys returns a copy of v with the named keys removed recursively. Used to
// drop API noise and read-only fields before comparing or restoring.
func StripKeys(v any, keys map[string]bool) any {
	return StripKeysFunc(v, func(k string) bool { return keys[k] })
}

// IsODataAnnotation reports whether a key is an OData annotation that must be
// dropped before a write (e.g. "@odata.context", "x@odata.navigationLink"),
// while preserving "@odata.type" which Graph needs for polymorphic creates.
func IsODataAnnotation(key string) bool {
	if key == "@odata.type" {
		return false
	}
	return strings.Contains(key, "@odata.")
}

// Normalize parses raw JSON and strips the given keys for comparison.
func Normalize(raw json.RawMessage, keys map[string]bool) (any, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return StripKeys(v, keys), nil
}
