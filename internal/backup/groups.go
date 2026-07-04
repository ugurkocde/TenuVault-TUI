package backup

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sync"

	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
)

// groupResolver resolves assignment group ids to display names via
// GET /v1.0/groups/{id}?$select=displayName, caching per run so each group is
// fetched once even when many policies target it. Group names are a nicety
// that must never fail a backup: on the first permission failure (401/403,
// i.e. Group.Read.All not granted) the resolver disables itself and the
// backup keeps raw ids; other lookup failures (deleted groups) are cached
// negatively and skipped.
type groupResolver struct {
	c        graph.API
	mu       sync.Mutex
	names    map[string]string // id -> displayName ("" = lookup failed)
	disabled bool
}

func newGroupResolver(c graph.API) *groupResolver {
	return &groupResolver{c: c, names: map[string]string{}}
}

// resolve looks up one group id unless it is cached or the resolver is
// disabled. Safe for concurrent use by the backup workers: the id is reserved
// in the cache before the HTTP call, so each group is fetched exactly once.
func (r *groupResolver) resolve(ctx context.Context, id string) {
	r.mu.Lock()
	if r.disabled {
		r.mu.Unlock()
		return
	}
	if _, seen := r.names[id]; seen {
		r.mu.Unlock()
		return
	}
	r.names[id] = "" // reserve; a failed lookup stays negative-cached
	r.mu.Unlock()

	raw, err := r.c.Get(ctx, "v1.0", "/groups/"+id, url.Values{"$select": {"displayName"}})
	if err != nil {
		var se *graph.StatusError
		if errors.As(err, &se) && (se.Code == http.StatusUnauthorized || se.Code == http.StatusForbidden) {
			r.mu.Lock()
			r.disabled = true
			r.mu.Unlock()
		}
		return
	}
	var m struct {
		DisplayName string `json:"displayName"`
	}
	if json.Unmarshal(raw, &m) == nil && m.DisplayName != "" {
		r.mu.Lock()
		r.names[id] = m.DisplayName
		r.mu.Unlock()
	}
}

// resolved returns the successfully resolved id -> name map.
func (r *groupResolver) resolved() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(r.names))
	for id, name := range r.names {
		if name != "" {
			out[id] = name
		}
	}
	return out
}

// wasDisabled reports whether lookups were stopped by a permission failure.
func (r *groupResolver) wasDisabled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.disabled
}

// assignmentGroupIDs extracts the group ids targeted by a policy's embedded
// assignments (group and exclusion-group targets carry groupId; allDevices /
// allLicensedUsers targets have none).
func assignmentGroupIDs(raw json.RawMessage) []string {
	var doc struct {
		Assignments []struct {
			Target map[string]any `json:"target"`
		} `json:"assignments"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	var ids []string
	for _, a := range doc.Assignments {
		if id, ok := a.Target["groupId"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
