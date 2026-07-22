package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

func TestCreateBackupDirAvoidsSameInstantCollision(t *testing.T) {
	root := t.TempDir()
	start := time.Date(2026, 7, 9, 12, 0, 0, 123, time.UTC)
	folderA, pathA, err := createBackupDir(root, start)
	if err != nil {
		t.Fatal(err)
	}
	folderB, pathB, err := createBackupDir(root, start)
	if err != nil {
		t.Fatal(err)
	}
	if folderA == folderB || pathA == pathB {
		t.Fatalf("colliding folders: %q and %q", folderA, folderB)
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

// TestRunWritesChecksums re-hashes every written policy file against the
// checksums.json sidecar, including a duplicate display name that lands under
// a " (2)" file.
func TestRunWritesChecksums(t *testing.T) {
	dc, _ := catalog.ByKey("deviceConfigurations")
	f := &graphtest.Fake{Lists: map[string][]json.RawMessage{
		"beta " + dc.ListPath: {
			[]byte(`{"@odata.type":"#microsoft.graph.windows10GeneralConfiguration","id":"1","displayName":"BitLocker"}`),
			[]byte(`{"@odata.type":"#microsoft.graph.windows10GeneralConfiguration","id":"2","displayName":"BitLocker"}`),
		},
	}}

	root := t.TempDir()
	res, err := Run(context.Background(), f, []catalog.PolicyType{dc},
		Options{Root: root, Tenant: graph.Tenant{ID: "t1"}}, nil)
	if err != nil {
		t.Fatal(err)
	}

	sums, err := store.ReadChecksums(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if sums.Algorithm != "sha256" {
		t.Errorf("algorithm = %q", sums.Algorithm)
	}
	if len(sums.Files) != 2 {
		t.Fatalf("recorded files = %v, want 2 (duplicate name must be recorded under its real file)", sums.Files)
	}
	for rel, want := range sums.Files {
		data, err := os.ReadFile(filepath.Join(res.Path, rel))
		if err != nil {
			t.Fatalf("recorded file %s unreadable: %v", rel, err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != want {
			t.Errorf("checksum mismatch for %s", rel)
		}
	}

	rep, err := store.Verify(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() || rep.OK != 2 {
		t.Errorf("fresh backup does not verify: %+v", rep)
	}
}

// getCounter wraps a graph.API counting per-path GET calls.
type getCounter struct {
	graph.API
	mu    sync.Mutex
	calls map[string]int
}

func (g *getCounter) Get(ctx context.Context, version, path string, q url.Values) (json.RawMessage, error) {
	g.mu.Lock()
	g.calls[path]++
	g.mu.Unlock()
	return g.API.Get(ctx, version, path, q)
}

// TestBackupResolvesGroupNames: with assignments included, group ids from
// assignment targets are resolved to names in groups.json, one GET per group
// id no matter how many policies target it.
func TestBackupResolvesGroupNames(t *testing.T) {
	dc, _ := catalog.ByKey("deviceConfigurations")
	f := &graphtest.Fake{
		Lists: map[string][]json.RawMessage{
			"beta " + dc.ListPath: {
				[]byte(`{"@odata.type":"#microsoft.graph.windows10GeneralConfiguration","id":"1","displayName":"BitLocker"}`),
				[]byte(`{"@odata.type":"#microsoft.graph.windows10GeneralConfiguration","id":"2","displayName":"Defender"}`),
			},
			"beta " + dc.ListPath + "/1/assignments": {
				[]byte(`{"id":"a1","target":{"@odata.type":"#microsoft.graph.groupAssignmentTarget","groupId":"g1"}}`),
				[]byte(`{"id":"a2","target":{"@odata.type":"#microsoft.graph.allDevicesAssignmentTarget"}}`),
			},
			"beta " + dc.ListPath + "/2/assignments": {
				[]byte(`{"id":"a3","target":{"@odata.type":"#microsoft.graph.exclusionGroupAssignmentTarget","groupId":"g1"}}`),
			},
		},
		Gets: map[string]json.RawMessage{
			"v1.0 /groups/g1": []byte(`{"displayName":"Pilot Devices"}`),
		},
	}
	g := &getCounter{API: f, calls: map[string]int{}}

	root := t.TempDir()
	res, err := Run(context.Background(), g, []catalog.PolicyType{dc},
		Options{Root: root, Tenant: graph.Tenant{ID: "t1"}, IncludeAssignments: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "Success" {
		t.Errorf("status = %q", res.Status)
	}

	data, err := os.ReadFile(filepath.Join(res.Path, "groups.json"))
	if err != nil {
		t.Fatalf("groups.json missing: %v", err)
	}
	var names map[string]string
	if err := json.Unmarshal(data, &names); err != nil {
		t.Fatal(err)
	}
	if names["g1"] != "Pilot Devices" || len(names) != 1 {
		t.Errorf("groups.json = %v", names)
	}
	if n := g.calls["/groups/g1"]; n != 1 {
		t.Errorf("group g1 fetched %d times, want 1 (cache)", n)
	}

	// Assignments are embedded in the policy file itself.
	pdata, err := os.ReadFile(filepath.Join(res.Path, "DeviceConfigurations", "BitLocker.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pdata), `"groupId": "g1"`) {
		t.Errorf("policy file missing embedded assignment target:\n%s", pdata)
	}
}

// deny403 wraps a graph.API and rejects /groups lookups with a 403.
type deny403 struct{ graph.API }

func (d *deny403) Get(ctx context.Context, version, path string, q url.Values) (json.RawMessage, error) {
	if strings.HasPrefix(path, "/groups/") {
		return nil, &graph.StatusError{Code: 403, Method: "GET", Path: path, Detail: "Authorization_RequestDenied: Insufficient privileges"}
	}
	return d.API.Get(ctx, version, path, q)
}

// TestBackupDegradesWithoutGroupScope: a 403 on the first group lookup
// disables name resolution; the backup still succeeds with raw ids and a
// single note in backup.log, and no groups.json is written.
func TestBackupDegradesWithoutGroupScope(t *testing.T) {
	dc, _ := catalog.ByKey("deviceConfigurations")
	f := &graphtest.Fake{Lists: map[string][]json.RawMessage{
		"beta " + dc.ListPath: {
			[]byte(`{"@odata.type":"#microsoft.graph.windows10GeneralConfiguration","id":"1","displayName":"BitLocker"}`),
		},
		"beta " + dc.ListPath + "/1/assignments": {
			[]byte(`{"id":"a1","target":{"groupId":"g1"}}`),
		},
	}}

	root := t.TempDir()
	res, err := Run(context.Background(), &deny403{API: f}, []catalog.PolicyType{dc},
		Options{Root: root, Tenant: graph.Tenant{ID: "t1"}, IncludeAssignments: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "Success" {
		t.Errorf("status = %q, want Success despite the missing group scope", res.Status)
	}
	if _, err := os.Stat(filepath.Join(res.Path, "groups.json")); !os.IsNotExist(err) {
		t.Error("groups.json written despite no resolvable names")
	}
	logData, err := os.ReadFile(filepath.Join(res.Path, "backup.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logData), "Group.Read.All not granted") {
		t.Errorf("backup.log missing the degrade note:\n%s", logData)
	}
	pdata, _ := os.ReadFile(filepath.Join(res.Path, "DeviceConfigurations", "BitLocker.json"))
	if !strings.Contains(string(pdata), `"groupId": "g1"`) {
		t.Error("raw group id lost from the policy file")
	}
}

// TestSettingsCatalogFilesArePortalImportable pins the file shape the Intune
// portal's Create > Import policy accepts: the entity @odata.context present,
// no stray expand annotations, and settings kept as {id, settingInstance}.
func TestSettingsCatalogFilesArePortalImportable(t *testing.T) {
	sc, _ := catalog.ByKey("configurationPolicies")
	f := &graphtest.Fake{
		Lists: map[string][]json.RawMessage{
			"beta " + sc.ListPath: {[]byte(`{"id":"p1","name":"Baseline"}`)},
		},
		Gets: map[string]json.RawMessage{
			"beta " + sc.ListPath + "/p1": []byte(`{
				"@odata.context":"https://graph.microsoft.com/beta/$metadata#deviceManagement/configurationPolicies(settings())/$entity",
				"id":"p1","name":"Baseline","platforms":"windows10","technologies":"mdm","settingCount":1,
				"settings@odata.context":"https://graph.microsoft.com/beta/$metadata#...settings",
				"settings":[{"id":"0","settingInstance":{"@odata.type":"#microsoft.graph.deviceManagementConfigurationChoiceSettingInstance","settingDefinitionId":"d1"}}]}`),
		},
	}
	res, err := Run(context.Background(), f, []catalog.PolicyType{sc},
		Options{Root: t.TempDir(), Tenant: graph.Tenant{ID: "t1", DisplayName: "Lab"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(res.Path, "ConfigurationPolicies", "Baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["@odata.context"] != "https://graph.microsoft.com/beta/$metadata#deviceManagement/configurationPolicies/$entity" {
		t.Errorf("@odata.context = %v, want the portal entity context", m["@odata.context"])
	}
	if _, ok := m["settings@odata.context"]; ok {
		t.Error("settings@odata.context must be stripped for portal import")
	}
	setting := m["settings"].([]any)[0].(map[string]any)
	if setting["id"] != "0" {
		t.Errorf("settings[0].id = %v, want kept as in the portal export", setting["id"])
	}
	inst := setting["settingInstance"].(map[string]any)
	if inst["@odata.type"] != "#microsoft.graph.deviceManagementConfigurationChoiceSettingInstance" {
		t.Error("settingInstance @odata.type must be preserved")
	}
}

// TestRunWithZeroPoliciesWarns: a run where every selected category comes back
// empty is almost always the wrong tenant or category selection, so it must
// not present as a clean Success.
func TestRunWithZeroPoliciesWarns(t *testing.T) {
	dc, _ := catalog.ByKey("deviceConfigurations")
	f := &graphtest.Fake{Lists: map[string][]json.RawMessage{
		"beta " + dc.ListPath: {},
	}}
	res, err := Run(context.Background(), f, []catalog.PolicyType{dc},
		Options{Root: t.TempDir(), Tenant: graph.Tenant{ID: "t1", DisplayName: "Lab"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "CompletedWithWarnings" {
		t.Fatalf("status = %q, want CompletedWithWarnings for a zero-policy run", res.Status)
	}
}
