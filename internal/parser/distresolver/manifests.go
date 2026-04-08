package distresolver

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// GoManifestSpec parses module declarations from go.mod.
//
// Format: a line starting with `module ` followed by the module path.
// Anything after a `//` comment is ignored.
var GoManifestSpec = ManifestSpec{
	Filenames: []string{"go.mod"},
	Parse: func(path string, data []byte) (string, error) {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "module ") {
				continue
			}
			rest := strings.TrimSpace(strings.TrimPrefix(line, "module"))
			if i := strings.Index(rest, "//"); i != -1 {
				rest = strings.TrimSpace(rest[:i])
			}
			rest = strings.Trim(rest, "\"")
			return rest, nil
		}
		return "", scanner.Err()
	},
}

// NpmManifestSpec parses the "name" field from package.json.
//
// A workspace root with no "name" field returns "" so the resolver keeps
// walking upward.
var NpmManifestSpec = ManifestSpec{
	Filenames: []string{"package.json"},
	Parse: func(path string, data []byte) (string, error) {
		var raw struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return "", fmt.Errorf("invalid package.json: %w", err)
		}
		return strings.TrimSpace(raw.Name), nil
	},
}

// PythonManifestSpec parses pyproject.toml, setup.cfg, or setup.py for
// the distribution name. The TOML parsing is intentionally minimal — we
// only need the `name = "..."` field under `[project]` (PEP 621) or
// `[tool.poetry]`. setup.cfg uses an INI-ish syntax with `name = ...`
// under `[metadata]`. setup.py is a free-form Python script; we look
// for `name="..."` or `name='...'` as a regex.
var PythonManifestSpec = ManifestSpec{
	Filenames: []string{"pyproject.toml", "setup.cfg", "setup.py"},
	Parse: func(path string, data []byte) (string, error) {
		switch {
		case strings.HasSuffix(path, "pyproject.toml"):
			return parsePyprojectTOML(data)
		case strings.HasSuffix(path, "setup.cfg"):
			return parseSetupCfg(data)
		case strings.HasSuffix(path, "setup.py"):
			return parseSetupPy(data)
		}
		return "", nil
	},
}

func parsePyprojectTOML(data []byte) (string, error) {
	// Look for `name = "..."` under `[project]` or `[tool.poetry]`.
	var section string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	nameLine := regexp.MustCompile(`^\s*name\s*=\s*["']([^"']+)["']`)
	for scanner.Scan() {
		line := scanner.Text()
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			section = strings.Trim(trim, "[]")
			continue
		}
		if section != "project" && section != "tool.poetry" {
			continue
		}
		if m := nameLine.FindStringSubmatch(line); m != nil {
			return m[1], nil
		}
	}
	return "", scanner.Err()
}

func parseSetupCfg(data []byte) (string, error) {
	var section string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			section = strings.Trim(trim, "[]")
			continue
		}
		if section != "metadata" {
			continue
		}
		if eq := strings.Index(trim, "="); eq != -1 {
			key := strings.TrimSpace(trim[:eq])
			val := strings.TrimSpace(trim[eq+1:])
			if key == "name" {
				return val, nil
			}
		}
	}
	return "", scanner.Err()
}

var setupPyName = regexp.MustCompile(`name\s*=\s*["']([^"']+)["']`)

func parseSetupPy(data []byte) (string, error) {
	if m := setupPyName.FindSubmatch(data); m != nil {
		return string(m[1]), nil
	}
	return "", nil
}

// JavaManifestSpec parses pom.xml (Maven) or build.gradle / build.gradle.kts
// (Gradle) for the artifact identity.
//
// Maven: returns "<groupId>:<artifactId>" pulling from the `<project>`
// element (and walking up the `<parent>` element if needed).
//
// Gradle: looks for `group` and `rootProject.name` / project name. Falls
// back to "" so the walker keeps going if neither is present.
var JavaManifestSpec = ManifestSpec{
	Filenames: []string{"pom.xml", "build.gradle.kts", "build.gradle"},
	Parse: func(path string, data []byte) (string, error) {
		if strings.HasSuffix(path, "pom.xml") {
			return parsePomXML(data)
		}
		return parseGradle(data)
	},
}

type pomProject struct {
	XMLName    xml.Name `xml:"project"`
	GroupID    string   `xml:"groupId"`
	ArtifactID string   `xml:"artifactId"`
	Parent     struct {
		GroupID string `xml:"groupId"`
	} `xml:"parent"`
}

func parsePomXML(data []byte) (string, error) {
	var p pomProject
	if err := xml.Unmarshal(data, &p); err != nil {
		return "", fmt.Errorf("invalid pom.xml: %w", err)
	}
	group := p.GroupID
	if group == "" {
		group = p.Parent.GroupID
	}
	if p.ArtifactID == "" {
		return "", nil
	}
	if group == "" {
		return "", errors.New("pom.xml: missing groupId and parent groupId")
	}
	return group + ":" + p.ArtifactID, nil
}

var (
	gradleGroup = regexp.MustCompile(`(?m)^\s*group\s*=?\s*["']([^"']+)["']`)
	gradleName  = regexp.MustCompile(`(?m)^\s*rootProject\.name\s*=\s*["']([^"']+)["']`)
)

func parseGradle(data []byte) (string, error) {
	g := gradleGroup.FindSubmatch(data)
	n := gradleName.FindSubmatch(data)
	if g == nil || n == nil {
		return "", nil
	}
	return string(g[1]) + ":" + string(n[1]), nil
}
