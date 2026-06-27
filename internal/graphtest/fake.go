// Package graphtest provides a fake graph.API for testing the backup, restore,
// and sync engines without network access. It stubs GET/list responses and
// records POSTs so tests can assert on the exact create payloads sent to Graph.
package graphtest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
)

var _ graph.API = (*Fake)(nil)

// PostCall records one POST made through the fake.
type PostCall struct {
	Version string
	Path    string
	Body    json.RawMessage
}

// Fake is an in-memory graph.API. Lists and Gets are keyed by "version path".
type Fake struct {
	Lists map[string][]json.RawMessage
	Gets  map[string]json.RawMessage
	Posts []PostCall

	// PostFunc optionally customizes POST responses; the default returns an
	// object with a generated id.
	PostFunc func(version, path string, body json.RawMessage) (json.RawMessage, error)

	n int
}

func key(version, path string) string { return version + " " + path }

// Get returns a stubbed object or an error if none is registered.
func (f *Fake) Get(_ context.Context, version, path string, _ url.Values) (json.RawMessage, error) {
	if v, ok := f.Gets[key(version, path)]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("fake: no GET stub for %q", key(version, path))
}

// ListAll returns a stubbed collection, or an empty list if none is registered.
func (f *Fake) ListAll(_ context.Context, version, path string, _ url.Values) ([]json.RawMessage, error) {
	return f.Lists[key(version, path)], nil
}

// Post records the call and returns a generated id (or PostFunc's response).
func (f *Fake) Post(_ context.Context, version, path string, body json.RawMessage) (json.RawMessage, error) {
	f.Posts = append(f.Posts, PostCall{Version: version, Path: path, Body: body})
	if f.PostFunc != nil {
		return f.PostFunc(version, path, body)
	}
	f.n++
	return json.RawMessage(fmt.Sprintf(`{"id":"new-%d"}`, f.n)), nil
}

// Patch records the call as a POST-like entry and returns a generated id.
func (f *Fake) Patch(ctx context.Context, version, path string, body json.RawMessage) (json.RawMessage, error) {
	return f.Post(ctx, version, path, body)
}

// PostsTo returns the bodies of POSTs made to a path (substring match).
func (f *Fake) PostsTo(pathContains string) []json.RawMessage {
	var out []json.RawMessage
	for _, p := range f.Posts {
		if strings.Contains(p.Path, pathContains) {
			out = append(out, p.Body)
		}
	}
	return out
}
