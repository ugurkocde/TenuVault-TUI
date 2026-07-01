package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/graphtest"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
)

func TestRunWritesBackup(t *testing.T) {
	dc, _ := catalog.ByKey("deviceConfigurations")
	dcat, _ := catalog.ByKey("deviceCategories")
	f := &graphtest.Fake{Lists: map[string][]json.RawMessage{
		"beta " + dc.ListPath: {
			[]byte(`{"@odata.type":"#microsoft.graph.windows10GeneralConfiguration","id":"1","displayName":"BitLocker"}`),
			[]byte(`{"@odata.type":"#microsoft.graph.windows10GeneralConfiguration","id":"2","displayName":"Defender"}`),
		},
		"beta " + dcat.ListPath: {
			[]byte(`{"id":"c1","displayName":"Sales"}`),
		},
	}}

	root := t.TempDir()
	res, err := Run(context.Background(), f, []catalog.PolicyType{dc, dcat},
		Options{Root: root, Tenant: graph.Tenant{ID: "t1", DisplayName: "Lab"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "Success" {
		t.Errorf("status = %q", res.Status)
	}
	if res.ItemCounts["DeviceConfigurations"] != 2 || res.ItemCounts["DeviceCategories"] != 1 {
		t.Errorf("counts = %+v", res.ItemCounts)
	}

	// Files written under category folders.
	if _, err := os.Stat(filepath.Join(res.Path, "DeviceConfigurations", "BitLocker.json")); err != nil {
		t.Errorf("BitLocker.json missing: %v", err)
	}
	// Manifest is valid and reflects the run.
	data, err := os.ReadFile(filepath.Join(res.Path, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta store.Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.TenantName != "Lab" || meta.Status != "Success" {
		t.Errorf("metadata = %+v", meta)
	}
}

// TestRunCancelledKeepsPartialBackup cancels the run up front: nothing is
// fetched, but the folder still gets a manifest with Status "Cancelled" so it
// is never left as an orphan without metadata.
func TestRunCancelledKeepsPartialBackup(t *testing.T) {
	dc, _ := catalog.ByKey("deviceConfigurations")
	f := &graphtest.Fake{Lists: map[string][]json.RawMessage{
		"beta " + dc.ListPath: {
			[]byte(`{"id":"1","displayName":"BitLocker"}`),
		},
	}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	root := t.TempDir()
	res, err := Run(ctx, f, []catalog.PolicyType{dc},
		Options{Root: root, Tenant: graph.Tenant{ID: "t1"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "Cancelled" {
		t.Errorf("status = %q, want Cancelled", res.Status)
	}
	data, err := os.ReadFile(filepath.Join(res.Path, "metadata.json"))
	if err != nil {
		t.Fatalf("metadata.json missing after cancel: %v", err)
	}
	var meta store.Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Status != "Cancelled" {
		t.Errorf("manifest status = %q, want Cancelled", meta.Status)
	}
}

// gauge wraps a graph.API and tracks how many detail Gets run at once.
type gauge struct {
	graph.API
	mu       sync.Mutex
	inFlight int
	max      int
}

func (g *gauge) Get(ctx context.Context, version, path string, q url.Values) (json.RawMessage, error) {
	g.mu.Lock()
	g.inFlight++
	if g.inFlight > g.max {
		g.max = g.inFlight
	}
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		g.inFlight--
		g.mu.Unlock()
	}()
	time.Sleep(20 * time.Millisecond)
	return g.API.Get(ctx, version, path, q)
}

func TestRunFetchesDetailConcurrently(t *testing.T) {
	sc, _ := catalog.ByKey("configurationPolicies")
	const n = 8
	f := &graphtest.Fake{
		Lists: map[string][]json.RawMessage{},
		Gets:  map[string]json.RawMessage{},
	}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("p%d", i)
		f.Lists["beta "+sc.ListPath] = append(f.Lists["beta "+sc.ListPath],
			json.RawMessage(fmt.Sprintf(`{"id":%q,"name":"Policy %d"}`, id, i)))
		f.Gets["beta "+sc.ListPath+"/"+id] = json.RawMessage(fmt.Sprintf(`{"id":%q,"name":"Policy %d","settings":[]}`, id, i))
	}
	g := &gauge{API: f}

	var events []Event
	res, err := Run(context.Background(), g, []catalog.PolicyType{sc},
		Options{Root: t.TempDir(), Tenant: graph.Tenant{ID: "t1", DisplayName: "Lab"}},
		func(e Event) { events = append(events, e) })
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "Success" || res.ItemCounts["ConfigurationPolicies"] != n {
		t.Errorf("status = %q, counts = %+v", res.Status, res.ItemCounts)
	}
	files, err := os.ReadDir(filepath.Join(res.Path, "ConfigurationPolicies"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != n {
		t.Errorf("wrote %d files, want %d", len(files), n)
	}
	if g.max > 4 {
		t.Errorf("max in-flight Gets = %d, want <= 4", g.max)
	}
	if g.max < 2 {
		t.Errorf("max in-flight Gets = %d, want >= 2 (no concurrency)", g.max)
	}

	// Progress must be monotonic and end with a Done event at Total.
	prev := 0
	for _, e := range events {
		if e.Current < prev {
			t.Fatalf("progress went backwards: %d after %d", e.Current, prev)
		}
		prev = e.Current
	}
	last := events[len(events)-1]
	if !last.Done || last.Current != n || last.Total != n {
		t.Errorf("last event = %+v", last)
	}
}

// cancelOnGet cancels the run's context as soon as the first detail fetch
// starts, simulating the user aborting mid-category.
type cancelOnGet struct {
	graph.API
	cancel context.CancelFunc
	once   sync.Once
}

func (c *cancelOnGet) Get(ctx context.Context, version, path string, q url.Values) (json.RawMessage, error) {
	c.once.Do(c.cancel)
	return c.API.Get(ctx, version, path, q)
}

func TestRunStopsOnCancellation(t *testing.T) {
	sc, _ := catalog.ByKey("configurationPolicies")
	f := &graphtest.Fake{
		Lists: map[string][]json.RawMessage{},
		Gets:  map[string]json.RawMessage{},
	}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("p%d", i)
		f.Lists["beta "+sc.ListPath] = append(f.Lists["beta "+sc.ListPath],
			json.RawMessage(fmt.Sprintf(`{"id":%q,"name":"Policy %d"}`, id, i)))
		f.Gets["beta "+sc.ListPath+"/"+id] = json.RawMessage(fmt.Sprintf(`{"id":%q,"name":"Policy %d"}`, id, i))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	res, err := Run(ctx, &cancelOnGet{API: f, cancel: cancel}, []catalog.PolicyType{sc},
		Options{Root: t.TempDir(), Tenant: graph.Tenant{ID: "t1", DisplayName: "Lab"}}, nil)
	if err != nil {
		t.Fatalf("cancelled run should finalize, not fail: %v", err)
	}
	if res.Status != "Cancelled" {
		t.Errorf("status = %q, want Cancelled", res.Status)
	}
	if _, err := os.Stat(filepath.Join(res.Path, "metadata.json")); err != nil {
		t.Errorf("metadata.json missing after mid-run cancel: %v", err)
	}
}
