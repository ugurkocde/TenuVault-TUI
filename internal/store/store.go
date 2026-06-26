// Package store is the local backup repository: it lists backup folders, reads
// their manifests, and reads individual policy files. The on-disk layout is
// TenuVault-compatible (per-policy JSON under category folders + metadata.json).
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
		if data, err := os.ReadFile(filepath.Join(b.Path, "metadata.json")); err == nil {
			_ = json.Unmarshal(data, &b.Meta)
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
	for _, b := range backups {
		ts, err := time.Parse("2006-01-02-150405", strings.TrimPrefix(b.Folder, "backup-"))
		if err != nil {
			continue
		}
		if ts.Before(cutoff) {
			if err := os.RemoveAll(b.Path); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}
