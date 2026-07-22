package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/graphtest"
	"github.com/ugurkocde/TenuVault-TUI/internal/policyops"
)

// TestRunSyncLive copies a live source policy into the target, asserting the
// create payload is cleaned, prefixed, and POSTed to the right endpoint.
func TestRunSyncLive(t *testing.T) {
	source := &graphtest.Fake{}
	target := &graphtest.Fake{}
	items := []Item{{
		Category: "DeviceConfigurations",
		TypeKey:  "deviceConfigurations",
		Name:     "BitLocker",
		Raw:      []byte(`{"@odata.type":"#microsoft.graph.windows10GeneralConfiguration","id":"src-1","displayName":"BitLocker","assignments":[{"x":1}]}`),
	}}

	results := Run(context.Background(), target, source, items, "[Synced] ", nil)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("results = %+v", results)
	}
	if len(target.Posts) != 1 {
		t.Fatalf("target POSTs = %d, want 1", len(target.Posts))
	}
	if !strings.Contains(target.Posts[0].Path, "/deviceManagement/deviceConfigurations") {
		t.Errorf("path = %q", target.Posts[0].Path)
	}
	var body map[string]any
	_ = json.Unmarshal(target.Posts[0].Body, &body)
	if body["displayName"] != "[Synced] BitLocker" {
		t.Errorf("displayName = %v", body["displayName"])
	}
	if _, ok := body["id"]; ok {
		t.Error("id must be stripped")
	}
	if _, ok := body["assignments"]; ok {
		t.Error("assignments must be stripped on sync")
	}
	if body["@odata.type"] != "#microsoft.graph.windows10GeneralConfiguration" {
		t.Error("@odata.type must be preserved for create routing")
	}
}

// TestRunSyncStripsScopeTags: scope tag IDs are tenant-local RBAC references,
// so sync must drop them; restore (same tenant, via policyops.Create directly)
// keeps them.
func TestRunSyncStripsScopeTags(t *testing.T) {
	raw := []byte(`{"@odata.type":"#microsoft.graph.windows10GeneralConfiguration","id":"src-1","displayName":"BitLocker","roleScopeTagIds":["7","12"]}`)
	target := &graphtest.Fake{}
	results := Run(context.Background(), target, &graphtest.Fake{},
		[]Item{{Category: "DeviceConfigurations", TypeKey: "deviceConfigurations", Name: "BitLocker", Raw: raw}}, "", nil)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("results = %+v", results)
	}
	var body map[string]any
	_ = json.Unmarshal(target.Posts[0].Body, &body)
	if _, ok := body["roleScopeTagIds"]; ok {
		t.Error("roleScopeTagIds must be stripped on cross-tenant sync")
	}

	restoreTarget := &graphtest.Fake{}
	if _, err := policyops.Create(context.Background(), restoreTarget, "DeviceConfigurations", raw, ""); err != nil {
		t.Fatalf("restore-path create: %v", err)
	}
	body = map[string]any{}
	_ = json.Unmarshal(restoreTarget.Posts[0].Body, &body)
	if _, ok := body["roleScopeTagIds"]; !ok {
		t.Error("roleScopeTagIds must be kept on same-tenant restore")
	}
}

// TestRunSyncWarnsOnPartialContent: a live source type that needs a per-item
// detail fetch (configurationPolicies / DetailByID) but whose detail GET fails
// is still created, with Warn set so the incomplete copy is surfaced.
func TestRunSyncWarnsOnPartialContent(t *testing.T) {
	source := &graphtest.Fake{} // no GET stub registered -> detail fetch fails
	target := &graphtest.Fake{}
	items := []Item{{
		Category: "ConfigurationPolicies",
		TypeKey:  "configurationPolicies",
		Name:     "Catalog",
		Raw:      []byte(`{"id":"src-1","name":"Catalog"}`),
	}}

	results := Run(context.Background(), target, source, items, "", nil)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("item should still be created, got error: %v", results[0].Err)
	}
	if !results[0].Warn {
		t.Error("expected Warn=true when source content could not be fully resolved")
	}
	if len(target.Posts) != 1 {
		t.Fatalf("the policy should still be created, got %d POSTs", len(target.Posts))
	}
}

// TestRunSyncNoWarnOnBackupSource: backup-sourced items never set Warn (nothing
// to enrich; the file is the full content).
func TestRunSyncNoWarnOnBackupSource(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/p.json"
	if err := os.WriteFile(path, []byte(`{"@odata.type":"#microsoft.graph.windows10CompliancePolicy","id":"b1","displayName":"Compliance"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	target := &graphtest.Fake{}
	results := Run(context.Background(), target, nil,
		[]Item{{Category: "CompliancePolicies", Name: "Compliance", Path: path}}, "", nil)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("results = %+v", results)
	}
	if results[0].Warn {
		t.Error("backup-sourced item should not set Warn")
	}
}

// TestRunSyncFromBackup copies a policy read from a backup file (no source).
func TestRunSyncFromBackup(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/p.json"
	if err := os.WriteFile(path, []byte(`{"@odata.type":"#microsoft.graph.windows10CompliancePolicy","id":"b1","displayName":"Compliance"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	target := &graphtest.Fake{}
	results := Run(context.Background(), target, nil,
		[]Item{{Category: "CompliancePolicies", Name: "Compliance", Path: path}}, "", nil)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("results = %+v", results)
	}
	if len(target.Posts) != 1 || !strings.Contains(target.Posts[0].Path, "deviceCompliancePolicies") {
		t.Errorf("posts = %+v", target.Posts)
	}
}

// postGauge wraps a graph.API and tracks how many Posts run at once.
type postGauge struct {
	graph.API
	mu       sync.Mutex
	inFlight int
	max      int
}

func (g *postGauge) Post(ctx context.Context, version, path string, body json.RawMessage) (json.RawMessage, error) {
	g.mu.Lock()
	g.inFlight++
	if g.inFlight > g.max {
		g.max = g.inFlight
	}
	g.mu.Unlock()
	time.Sleep(20 * time.Millisecond) // let other workers overlap
	defer func() {
		g.mu.Lock()
		g.inFlight--
		g.mu.Unlock()
	}()
	return g.API.Post(ctx, version, path, body)
}

// writeBackupItems writes n compliance policies to a temp backup dir and
// returns the items.
func writeBackupItems(t *testing.T, n int) []Item {
	t.Helper()
	dir := t.TempDir()
	items := make([]Item, n)
	for i := range items {
		name := fmt.Sprintf("Policy %02d", i)
		path := fmt.Sprintf("%s/p%02d.json", dir, i)
		body := fmt.Sprintf(`{"@odata.type":"#microsoft.graph.windows10CompliancePolicy","id":"b%d","displayName":%q}`, i, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		items[i] = Item{Category: "CompliancePolicies", Name: name, Path: path}
	}
	return items
}

// TestRunSyncConcurrent proves creates overlap (bounded by the pool) and that
// results come back in input order despite concurrent completion.
func TestRunSyncConcurrent(t *testing.T) {
	items := writeBackupItems(t, 8)
	g := &postGauge{API: &graphtest.Fake{}}

	results := Run(context.Background(), g, nil, items, "", nil)

	if len(results) != 8 {
		t.Fatalf("results = %d, want 8", len(results))
	}
	for i, r := range results {
		if r.Err != nil {
			t.Fatalf("item %d failed: %v", i, r.Err)
		}
		if r.Item.Name != items[i].Name {
			t.Errorf("result %d = %q, want %q (input order)", i, r.Item.Name, items[i].Name)
		}
	}
	if g.max < 2 || g.max > syncWorkers {
		t.Errorf("max concurrent posts = %d, want 2..%d", g.max, syncWorkers)
	}
}

// TestRunSyncProgressMonotonic asserts completed-count progress that never
// decreases and ends at Total.
func TestRunSyncProgressMonotonic(t *testing.T) {
	items := writeBackupItems(t, 6)
	var events []Event
	Run(context.Background(), &graphtest.Fake{}, nil, items, "", func(e Event) {
		events = append(events, e) // progress is documented as serialized
	})
	if len(events) != 6 {
		t.Fatalf("events = %d, want 6", len(events))
	}
	for i, e := range events {
		if e.Current != i+1 || e.Total != 6 {
			t.Errorf("event %d = %d/%d, want %d/6", i, e.Current, e.Total, i+1)
		}
	}
}

// TestRunSyncCancelMidRun cancels while creates are in flight: every item gets
// a result, attempted ones succeed or fail on their own terms, unattempted
// ones carry context.Canceled.
func TestRunSyncCancelMidRun(t *testing.T) {
	items := writeBackupItems(t, 12)
	ctx, cancel := context.WithCancel(context.Background())
	fake := &graphtest.Fake{}
	first := true
	var mu sync.Mutex
	fake.PostFunc = func(version, path string, body json.RawMessage) (json.RawMessage, error) {
		mu.Lock()
		if first {
			first = false
			mu.Unlock()
			cancel()
			return json.RawMessage(`{"id":"new-1"}`), nil
		}
		mu.Unlock()
		return json.RawMessage(`{"id":"new-x"}`), nil
	}

	results := Run(ctx, fake, nil, items, "", nil)

	if len(results) != 12 {
		t.Fatalf("results = %d, want 12 (no result may be dropped)", len(results))
	}
	cancelled := 0
	for i, r := range results {
		if r.Item.Name != items[i].Name {
			t.Errorf("result %d out of order", i)
		}
		if r.NewID == "" && r.Err == nil {
			t.Errorf("result %d has neither an id nor an error", i)
		}
		if errors.Is(r.Err, context.Canceled) {
			cancelled++
		}
	}
	if cancelled == 0 {
		t.Error("expected at least one item reported as cancelled")
	}
}

// TestRunSyncMultiPartSequentialWithinItem: an administrative template's
// updateDefinitionValues POST must always come after its own create POST, even
// with the pool running other items concurrently.
func TestRunSyncMultiPartSequentialWithinItem(t *testing.T) {
	dir := t.TempDir()
	items := make([]Item, 4)
	for i := range items {
		name := fmt.Sprintf("ADMX %d", i)
		path := fmt.Sprintf("%s/a%d.json", dir, i)
		body := fmt.Sprintf(`{"id":"g%d","displayName":%q,"definitionValues":[{"enabled":true,"definition":{"id":"def-%d"}}]}`, i, name, i)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		items[i] = Item{Category: "GroupPolicyConfigurations", Name: name, Path: path}
	}
	fake := &graphtest.Fake{}
	// Deterministic ids derived from the created name, so each child call can
	// be traced back to its own create regardless of pool scheduling.
	fake.PostFunc = func(version, path string, body json.RawMessage) (json.RawMessage, error) {
		if strings.Contains(path, "/updateDefinitionValues") {
			return json.RawMessage(`{}`), nil
		}
		var m map[string]any
		_ = json.Unmarshal(body, &m)
		name, _ := m["displayName"].(string)
		return json.RawMessage(fmt.Sprintf(`{"id":%q}`, "id-"+name)), nil
	}
	results := Run(context.Background(), fake, nil, items, "", nil)
	for i, r := range results {
		if r.Err != nil {
			t.Fatalf("item %d failed: %v", i, r.Err)
		}
	}

	// For every created id, the child updateDefinitionValues call must appear
	// after the create that returned that id.
	createdAt := map[string]int{} // id -> index of its create POST
	for i, p := range fake.Posts {
		if strings.HasSuffix(p.Path, "/groupPolicyConfigurations") {
			var m map[string]any
			_ = json.Unmarshal(p.Body, &m)
			name, _ := m["displayName"].(string)
			createdAt["id-"+name] = i
		}
	}
	updates := 0
	for i, p := range fake.Posts {
		if strings.Contains(p.Path, "/updateDefinitionValues") {
			updates++
			parts := strings.Split(p.Path, "/")
			id := parts[len(parts)-2]
			at, ok := createdAt[id]
			if !ok {
				t.Errorf("updateDefinitionValues for unknown id %q", id)
				continue
			}
			if i < at {
				t.Errorf("updateDefinitionValues at post %d precedes its create at %d", i, at)
			}
		}
	}
	if updates != 4 {
		t.Errorf("updateDefinitionValues posts = %d, want 4", updates)
	}
}
