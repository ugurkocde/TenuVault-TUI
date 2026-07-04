package store

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// fixtureBackup writes a two-file backup with a valid checksums sidecar.
func fixtureBackup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cat := filepath.Join(dir, "DeviceConfigurations")
	if err := os.MkdirAll(cat, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for name, body := range map[string]string{"A": `{"a":1}`, "B": `{"b":2}`} {
		path := filepath.Join(cat, name+".json")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		files["DeviceConfigurations/"+name+".json"] = sha256Hex([]byte(body))
	}
	if err := WriteChecksums(dir, files); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestVerifyIntact(t *testing.T) {
	dir := fixtureBackup(t)
	rep, err := Verify(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() || rep.OK != 2 {
		t.Errorf("report = %+v, want clean with 2 ok", rep)
	}
}

func TestVerifyMismatch(t *testing.T) {
	dir := fixtureBackup(t)
	if err := os.WriteFile(filepath.Join(dir, "DeviceConfigurations", "A.json"), []byte(`{"a":999}`), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := Verify(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Mismatched) != 1 || rep.Mismatched[0] != "DeviceConfigurations/A.json" {
		t.Errorf("mismatched = %v", rep.Mismatched)
	}
	if rep.Clean() {
		t.Error("tampered backup reported clean")
	}
}

func TestVerifyMissingAndExtra(t *testing.T) {
	dir := fixtureBackup(t)
	if err := os.Remove(filepath.Join(dir, "DeviceConfigurations", "B.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "DeviceConfigurations", "Stray.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := Verify(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Missing) != 1 || rep.Missing[0] != "DeviceConfigurations/B.json" {
		t.Errorf("missing = %v", rep.Missing)
	}
	if len(rep.Extra) != 1 || rep.Extra[0] != "DeviceConfigurations/Stray.json" {
		t.Errorf("extra = %v", rep.Extra)
	}
}

func TestVerifyNoChecksums(t *testing.T) {
	dir := t.TempDir()
	if _, err := Verify(dir); err == nil {
		t.Fatal("want a distinct error for a backup without checksums")
	}
}

// sha256Hex is a test helper mirroring Verify's hashing.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
