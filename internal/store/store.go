// Package store is the local backup repository: it lists backup folders, reads
// their manifests, and reads individual policy files. The on-disk layout is
// TenuVault-compatible (per-policy JSON under category folders + metadata.json).
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ugurkocde/TenuVault-TUI/internal/fileutil"
)

// Metadata is the backup manifest, matching the TenuVault portal runbook shape.
type Metadata struct {
	BackupDate      string         `json:"BackupDate"`
	BackupFolder    string         `json:"BackupFolder"`
	StartTime       string         `json:"StartTime"`
	EndTime         string         `json:"EndTime"`
	Duration        string         `json:"Duration"`
	DurationSeconds int            `json:"DurationSeconds"`
	ItemCounts      map[string]int `json:"ItemCounts"`
	Status          string         `json:"Status"`
	RunbookVersion  string         `json:"RunbookVersion"`
	BackupFormat    string         `json:"BackupFormat"`
	TenantID        string         `json:"TenantId,omitempty"`
	TenantName      string         `json:"TenantName,omitempty"`
}

// Backup is a single backup folder on disk.
type Backup struct {
	Folder string // e.g. backup-2026-06-26-143200
	Path   string // absolute path
	Meta   Metadata
}

// Total returns the number of policies recorded in the manifest.
func (b Backup) Total() int {
	n := 0
	for _, c := range b.Meta.ItemCounts {
		n += c
	}
	return n
}

// List returns all backups under root, newest first.
func List(root string) ([]Backup, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var backups []Backup
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "backup-") {
			continue
		}
		b := Backup{Folder: e.Name(), Path: filepath.Join(root, e.Name())}
		data, err := os.ReadFile(filepath.Join(b.Path, "metadata.json"))
		if os.IsNotExist(err) {
			backups = append(backups, b)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read metadata for %s: %w", b.Folder, err)
		}
		if err := json.Unmarshal(data, &b.Meta); err != nil {
			return nil, fmt.Errorf("decode metadata for %s: %w", b.Folder, err)
		}
		backups = append(backups, b)
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].Folder > backups[j].Folder })
	return backups, nil
}

// Categories returns the category subfolders present in a backup.
func (b Backup) Categories() ([]string, error) {
	entries, err := os.ReadDir(b.Path)
	if err != nil {
		return nil, err
	}
	var cats []string
	for _, e := range entries {
		if e.IsDir() {
			cats = append(cats, e.Name())
		}
	}
	sort.Strings(cats)
	return cats, nil
}

// PolicyFile is a single backed-up policy.
type PolicyFile struct {
	Name string // file name without .json
	Path string
}

// Policies returns the policy files in a category folder.
func (b Backup) Policies(category string) ([]PolicyFile, error) {
	dir := filepath.Join(b.Path, category)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []PolicyFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		files = append(files, PolicyFile{
			Name: strings.TrimSuffix(e.Name(), ".json"),
			Path: filepath.Join(dir, e.Name()),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}

// Read returns the raw JSON of a policy file.
func Read(path string) (json.RawMessage, error) {
	return os.ReadFile(path)
}

// ChecksumsFile is the integrity sidecar written next to metadata.json. It is
// deliberately not part of metadata.json: the manifest matches the TenuVault
// portal runbook shape and stays untouched, and backups from older versions
// simply lack the sidecar.
const ChecksumsFile = "checksums.json"

// Checksums records the hash of every policy file written by a backup, keyed
// by "Category/File.json" (forward slashes on every platform).
type Checksums struct {
	Algorithm string            `json:"algorithm"`
	Files     map[string]string `json:"files"`
}

// WriteChecksums writes the integrity sidecar into a backup folder.
func WriteChecksums(dir string, files map[string]string) error {
	if files == nil {
		files = map[string]string{}
	}
	data, err := json.MarshalIndent(Checksums{Algorithm: "sha256", Files: files}, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFileAtomic(filepath.Join(dir, ChecksumsFile), data, 0o644)
}

// ReadChecksums loads the integrity sidecar of a backup folder.
func ReadChecksums(dir string) (Checksums, error) {
	var c Checksums
	data, err := os.ReadFile(filepath.Join(dir, ChecksumsFile))
	if err != nil {
		if os.IsNotExist(err) {
			return c, fmt.Errorf("no checksums recorded (backup predates integrity support)")
		}
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("decode %s: %w", ChecksumsFile, err)
	}
	return c, nil
}

// VerifyReport is the outcome of an integrity check.
type VerifyReport struct {
	OK         int
	Mismatched []string // recorded hash differs from the file on disk
	Missing    []string // recorded but absent on disk
	Extra      []string // policy file on disk that was never recorded
}

// Clean reports whether the backup matches its recorded checksums exactly.
func (r VerifyReport) Clean() bool {
	return len(r.Mismatched) == 0 && len(r.Missing) == 0 && len(r.Extra) == 0
}

// Verify re-hashes every policy file in a backup folder against its recorded
// checksum and flags files that appeared after the backup was written.
func Verify(dir string) (VerifyReport, error) {
	c, err := ReadChecksums(dir)
	if err != nil {
		return VerifyReport{}, err
	}
	var rep VerifyReport
	for rel, want := range c.Files {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			rep.Missing = append(rep.Missing, rel)
			continue
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != want {
			rep.Mismatched = append(rep.Mismatched, rel)
			continue
		}
		rep.OK++
	}
	// A policy file on disk that was never recorded means the backup was
	// altered after it was written.
	b := Backup{Path: dir}
	cats, err := b.Categories()
	if err != nil {
		return rep, fmt.Errorf("list backup categories: %w", err)
	}
	for _, cat := range cats {
		files, err := b.Policies(cat)
		if err != nil {
			return rep, fmt.Errorf("list policies in %s: %w", cat, err)
		}
		for _, f := range files {
			if _, ok := c.Files[cat+"/"+f.Name+".json"]; !ok {
				rep.Extra = append(rep.Extra, cat+"/"+f.Name+".json")
			}
		}
	}
	sort.Strings(rep.Mismatched)
	sort.Strings(rep.Missing)
	sort.Strings(rep.Extra)
	return rep, nil
}

// Cleanup removes backup folders older than the given number of days, based on
// the timestamp encoded in the folder name. Returns how many were removed.
func Cleanup(root string, days int) (int, error) {
	if days <= 0 {
		return 0, nil
	}
	backups, err := List(root)
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	removed := 0
	var removeErrs []error
	for _, b := range backups {
		ts, err := backupFolderTime(b.Folder)
		if err != nil {
			continue
		}
		if ts.Before(cutoff) {
			if err := os.RemoveAll(b.Path); err == nil {
				removed++
			} else {
				removeErrs = append(removeErrs, fmt.Errorf("remove %s: %w", b.Folder, err))
			}
		}
	}
	return removed, errors.Join(removeErrs...)
}

func backupFolderTime(folder string) (time.Time, error) {
	raw := strings.TrimPrefix(folder, "backup-")
	if ts, err := time.Parse("2006-01-02-150405.000000000", raw); err == nil {
		return ts, nil
	}
	if dot := strings.IndexByte(raw, '.'); dot >= 0 {
		if dash := strings.LastIndexByte(raw, '-'); dash > dot {
			if _, err := strconv.Atoi(raw[dash+1:]); err == nil {
				raw = raw[:dash]
				if ts, err := time.Parse("2006-01-02-150405.000000000", raw); err == nil {
					return ts, nil
				}
			}
		}
	}
	return time.Parse("2006-01-02-150405", raw)
}
