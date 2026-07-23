//go:build ignore
// +build ignore

package organize

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func GroupFiles(watchDir string, files []FileInput) []ScanGroup {
	byKey := map[string]*ScanGroup{}
	for _, f := range files {
		name := scanResourceName(watchDir, f)
		key := strings.ToLower(name)
		if key == "" {
			key = strings.ToLower(f.Name)
		}
		g := byKey[key]
		if g == nil {
			g = &ScanGroup{ResourceName: name}
			byKey[key] = g
		}
		g.Files = append(g.Files, f)
	}
	groups := make([]ScanGroup, 0, len(byKey))
	for _, g := range byKey {
		sort.SliceStable(g.Files, func(i, j int) bool {
			return scanFileSortKey(g.Files[i]) < scanFileSortKey(g.Files[j])
		})
		groups = append(groups, *g)
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].ResourceName < groups[j].ResourceName })
	return groups
}

func scanFileSortKey(f FileInput) string {
	if strings.TrimSpace(f.Path) != "" {
		return strings.ToLower(filepath.ToSlash(f.Path))
	}
	return strings.ToLower(f.Name)
}

func scanResourceName(watchDir string, f FileInput) string {
	path := strings.TrimSpace(f.Path)
	if path != "" {
		if rel, err := filepath.Rel(watchDir, path); err == nil && rel != "." && !relEscapesBase(rel) {
			parts := splitPathParts(rel)
			if len(parts) > 1 {
				return strings.TrimSpace(parts[0])
			}
			if len(parts) == 1 {
				return cleanRootFileResourceName(parts[0])
			}
		}
	}
	return cleanRootFileResourceName(f.Name)
}

func splitPathParts(path string) []string {
	path = filepath.ToSlash(path)
	raw := strings.Split(path, "/")
	parts := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" && p != "." {
			parts = append(parts, p)
		}
	}
	return parts
}

var rootEpisodeTokenPattern = regexp.MustCompile(`(?i)(S\d{1,2}[ ._-]*E\d{1,3}|\d{1,2}x\d{1,3}|第\s*\d{1,2}\s*季.*?第\s*\d{1,3}\s*[集话]|EP?\s*0*\d{1,3}|第\s*\d{1,3}\s*[集话回期]|part[ ._-]?\d+|cd[ ._-]?\d+|disc[ ._-]?\d+)`)

var rootNoiseTokenPattern = regexp.MustCompile(`(?i)\b(2160p|1080p|720p|4k|hdr|web[- ]?dl|bluray|x264|x265|h264|h265|hevc|aac|ddp|atmos|nf|amzn)\b`)
var squareBracketPattern = regexp.MustCompile(`\[[^\]]+\]`)
var roundBracketPattern = regexp.MustCompile(`\([^)]*\)`)

func cleanRootFileResourceName(name string) string {
	stem := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	cleaned := rootEpisodeTokenPattern.ReplaceAllString(stem, " ")
	cleaned = strings.ReplaceAll(cleaned, ".", " ")
	cleaned = strings.ReplaceAll(cleaned, "_", " ")
	cleaned = squareBracketPattern.ReplaceAllString(cleaned, " ")
	cleaned = roundBracketPattern.ReplaceAllStringFunc(cleaned, func(token string) string {
		if extractYearFromString(token) != "" {
			return " " + strings.Trim(token, "()") + " "
		}
		return " "
	})
	cleaned = rootNoiseTokenPattern.ReplaceAllString(cleaned, " ")
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	if cleaned == "" {
		if p, err := ParseFilename(stem); err == nil && p.Title != "" {
			cleaned = p.Title
		}
	}
	if cleaned == "" {
		cleaned = stem
	}
	return strings.TrimSpace(cleaned)
}

func extractYearFromString(s string) string {
	for i := 0; i <= len(s)-4; i++ {
		if s[i] >= '0' && s[i] <= '9' && s[i+1] >= '0' && s[i+1] <= '9' && s[i+2] >= '0' && s[i+2] <= '9' && s[i+3] >= '0' && s[i+3] <= '9' {
			year := s[i : i+4]
			if year >= "1900" && year <= "2099" {
				return year
			}
		}
	}
	return ""
}

func ShouldSkipDir(path, watchDir, targetDir string) bool {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(targetDir) == "" {
		return false
	}
	path = filepath.Clean(path)
	watchDir = filepath.Clean(watchDir)
	targetDir = filepath.Clean(targetDir)
	if targetDir == watchDir {
		return false
	}
	if rel, err := filepath.Rel(watchDir, targetDir); err != nil || rel == "." || relEscapesBase(rel) {
		return false
	}
	if path == targetDir {
		return true
	}
	return false
}

func relEscapesBase(rel string) bool {
	rel = filepath.ToSlash(rel)
	return rel == ".." || strings.HasPrefix(rel, "../")
}