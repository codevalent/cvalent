package walker

import (
	"os"
	"path/filepath"
	"strings"
)

// WalkResult contains files grouped by language and skipped file counts.
type WalkResult struct {
	Files   map[string][]string // language -> file paths (relative to root)
	Skipped map[string]int      // extension -> count of unsupported files
}

// Walk traverses a directory tree and returns files grouped by language.
func Walk(root string, exclude []string, scopePath string) (*WalkResult, error) {
	result := &WalkResult{
		Files:   map[string][]string{},
		Skipped: map[string]int{},
	}

	walkRoot := root
	if scopePath != "" {
		walkRoot = filepath.Join(root, scopePath)
	}

	// Load .gitignore patterns
	gitignorePatterns := loadGitignore(root)

	err := filepath.Walk(walkRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(root, path)

		if info.IsDir() {
			base := filepath.Base(path)
			// Skip excluded directories
			if shouldExcludeDir(base, relPath, exclude, gitignorePatterns) {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file matches gitignore
		if matchesGitignore(relPath, gitignorePatterns) {
			return nil
		}

		ext := filepath.Ext(path)
		lang := extToLanguage(ext)
		if lang != "" {
			result.Files[lang] = append(result.Files[lang], relPath)
		} else if ext != "" {
			result.Skipped[ext]++
		}

		return nil
	})

	return result, err
}

func extToLanguage(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".java":
		return "java"
	case ".ts", ".tsx":
		return "typescript"
	case ".py":
		return "python"
	default:
		return ""
	}
}

func shouldExcludeDir(base, relPath string, exclude []string, gitignore []string) bool {
	if base == ".git" || base == ".cvalent" {
		return true
	}
	for _, pattern := range exclude {
		if base == pattern || strings.HasPrefix(relPath, pattern) {
			return true
		}
	}
	for _, pattern := range gitignore {
		if base == strings.TrimSuffix(pattern, "/") {
			return true
		}
	}
	return false
}

func loadGitignore(root string) []string {
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return nil
	}
	var patterns []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

func matchesGitignore(relPath string, patterns []string) bool {
	for _, pattern := range patterns {
		matched, _ := filepath.Match(pattern, filepath.Base(relPath))
		if matched {
			return true
		}
	}
	return false
}
