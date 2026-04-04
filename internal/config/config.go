package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	DirName     = ".cvalent"
	ConfigFile  = "config.json"
	GraphFile   = "graph.db"
	Version     = 1
)

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
	if err := os.WriteFile(gitignore, []byte(GraphFile+"\n"), 0644); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Load reads the config from .cvalent/config.json.
func Load(root string) (*Config, error) {
	path := filepath.Join(root, DirName, ConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Exists checks if .cvalent/config.json exists.
func Exists(root string) bool {
	path := filepath.Join(root, DirName, ConfigFile)
	_, err := os.Stat(path)
	return err == nil
}

// GraphPath returns the path to the graph database file.
func GraphPath(root string) string {
	return filepath.Join(root, DirName, GraphFile)
}

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
