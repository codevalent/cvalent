// Package config manages the per-repo .cvalent/ directory: config
// file, store path, and language detection.
//
// At Rung 0 the store filename is "store.db" (SQLite). The legacy
// "graph.db" filename is detected by Load and surfaces a clear error
// pointing the user at `cvalent migrate-store` — graceful degradation
// would just hide the broken state.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

const (
	DirName    = ".cvalent"
	ConfigFile = "config.json"
	StoreFile  = "store.db"
	// LegacyGraphFile is the pre-Rung-0 GoraphDB filename. The migrator
	// (cvalent migrate-store) is the only supported way to convert it.
	LegacyGraphFile = "graph.db"
	Version         = 2
)

// ErrLegacyStorePresent is returned by Load when the .cvalent directory
// still contains a graph.db file (the pre-Rung-0 GoraphDB store) and
// the new store.db has not been created. The CLI surfaces this with a
// "run `cvalent migrate-store` to upgrade" hint.
var ErrLegacyStorePresent = errors.New("config: legacy graph.db detected — run `cvalent migrate-store`")

type Config struct {
	Version   int      `json:"version"`
	Languages []string `json:"languages"`
	Exclude   []string `json:"exclude,omitempty"`
}

func DefaultConfig() Config {
	return Config{
		Version:   Version,
		Languages: []string{},
		Exclude:   []string{"vendor", "node_modules", ".git", "__pycache__", "dist", "build"},
	}
}

// Init creates the .cvalent directory with config and gitignore.
// root is the project root directory.
func Init(root string) (*Config, error) {
	dir := filepath.Join(root, DirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	// Auto-detect languages
	langs := detectLanguages(root)

	cfg := DefaultConfig()
	cfg.Languages = langs

	// Write config
	if err := writeConfig(dir, &cfg); err != nil {
		return nil, err
	}

	// Write gitignore
	gitignore := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gitignore, []byte(StoreFile+"\n"), 0644); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Load reads the config from .cvalent/config.json. If a legacy
// graph.db file is present and no store.db exists yet, returns
// ErrLegacyStorePresent so the caller can prompt the user to run the
// migrator. Old-shape configs (Version < 2) load fine but log a
// warning pointing at migrate-store.
func Load(root string) (*Config, error) {
	if hasLegacyOnly(root) {
		return nil, ErrLegacyStorePresent
	}
	path := filepath.Join(root, DirName, ConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Version < Version {
		log.Printf("cvalent: config %s is at version %d (current %d) — run `cvalent migrate-store` to upgrade",
			path, cfg.Version, Version)
	}
	return &cfg, nil
}

func hasLegacyOnly(root string) bool {
	dir := filepath.Join(root, DirName)
	legacy := filepath.Join(dir, LegacyGraphFile)
	store := filepath.Join(dir, StoreFile)
	if _, err := os.Stat(legacy); err != nil {
		return false
	}
	if _, err := os.Stat(store); err == nil {
		// Both present; the migrator wrote a backup-and-rename event we
		// missed but the new store wins.
		return false
	}
	return true
}

// Exists checks if .cvalent/config.json exists.
func Exists(root string) bool {
	path := filepath.Join(root, DirName, ConfigFile)
	_, err := os.Stat(path)
	return err == nil
}

// StorePath returns the path to the SQLite store file.
func StorePath(root string) string {
	return filepath.Join(root, DirName, StoreFile)
}

// LegacyGraphPath returns the path to the pre-Rung-0 GoraphDB file.
// The migrator uses this to find the source store.
func LegacyGraphPath(root string) string {
	return filepath.Join(root, DirName, LegacyGraphFile)
}

// GraphPath is preserved as an alias for StorePath so call sites that
// haven't been updated keep working. New code should call StorePath.
func GraphPath(root string) string { return StorePath(root) }

var _ = fmt.Sprintf // keep fmt referenced even if all helpers move

func writeConfig(dir string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ConfigFile), data, 0644)
}

func detectLanguages(root string) []string {
	exts := map[string]string{
		".go":   "go",
		".java": "java",
		".ts":   "typescript",
		".tsx":  "typescript",
		".py":   "python",
	}
	found := map[string]bool{}

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "vendor" || base == "node_modules" || base == DirName {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if lang, ok := exts[ext]; ok {
			found[lang] = true
		}
		return nil
	})

	var langs []string
	for _, lang := range []string{"go", "java", "typescript", "python"} {
		if found[lang] {
			langs = append(langs, lang)
		}
	}
	return langs
}
