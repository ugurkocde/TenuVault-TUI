// Package jsonutil holds small JSON helpers shared by the backup, restore, and
// diff engines: filename sanitizing, display-name extraction, and recursive key
// stripping for noise-insensitive comparison and clean restores.
package jsonutil

import (
	"encoding/json"
	"strings"
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
	if len(out) > 180 {
		out = out[:180]
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

// StripKeys returns a copy of v with the named keys removed recursively. Used to
// drop API noise and read-only fields before comparing or restoring.
func StripKeys(v any, keys map[string]bool) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if keys[k] {
				continue
			}
			out[k] = StripKeys(val, keys)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = StripKeys(val, keys)
		}
		return out
	default:
		return v
	}
}

// Normalize parses raw JSON and strips the given keys for comparison.
func Normalize(raw json.RawMessage, keys map[string]bool) (any, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return StripKeys(v, keys), nil
}
