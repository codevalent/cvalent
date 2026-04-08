package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInit_FreshRepo(t *testing.T) {
	root := t.TempDir()
	cfg, err := Init(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != Version {
		t.Errorf("version=%d", cfg.Version)
	}
	if !Exists(root) {
		t.Errorf("Exists() should be true after init")
	}
	if _, err := os.Stat(StorePath(root)); err == nil {
		t.Errorf("StorePath should not exist until build")
	}
	gitignore := filepath.Join(root, DirName, ".gitignore")
	data, err := os.ReadFile(gitignore)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != StoreFile+"\n" {
		t.Errorf("gitignore = %q", string(data))
	}
}

func TestLoad_DetectsLegacyOnly(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, LegacyGraphFile), []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(`{"version":1,"languages":["go"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(root)
	if !errors.Is(err, ErrLegacyStorePresent) {
		t.Fatalf("expected ErrLegacyStorePresent, got %v", err)
	}
}

func TestLoad_OldVersionLogsWarning(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Old config plus a fresh store.db so the legacy detection misses.
	if err := os.WriteFile(filepath.Join(dir, StoreFile), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(`{"version":1,"languages":["go"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 1 {
		t.Errorf("expected version 1 to round-trip, got %d", cfg.Version)
	}
}

func TestStorePath(t *testing.T) {
	got := StorePath("/tmp/repo")
	want := filepath.Join("/tmp/repo", DirName, StoreFile)
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
