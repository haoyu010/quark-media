//go:build ignore
// +build ignore

package organize

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

const (
	defaultMovieTemplate = "{{title}} ({{year}})/{{title}} ({{year}}) - {{resolution}}.{{videoCodec}}.{{audioCodec}}{{ext}}"
	defaultTVShowDir     = "{{title}} ({{year}})"
	defaultTVSeasonDir   = "Season {{seasonPad}}"
	defaultTVFile        = "{{title}} - S{{seasonPad}}E{{episodePad}} - {{resolution}}.{{videoCodec}}.{{audioCodec}}{{ext}}"
	maxFilenameLen       = 200
	maxPathLen           = 250
)

var reInvalidChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

func GeneratePath(match *TMDBMatchResult, parsed *ParsedMedia, tmpl *NamingTemplate) (*OrganizedPath, error) {
	return GeneratePathWithCategory(match, parsed, tmpl, "")
}

func GeneratePathWithCategory(match *TMDBMatchResult, parsed *ParsedMedia, tmpl *NamingTemplate, category string) (*OrganizedPath, error) {
	if parsed == nil {
		return nil, ErrNilParsedMedia
	}

	if tmpl == nil {
		tmpl = defaultNamingTemplate()
	}

	var fullPath string
	var err error

	if parsed.IsMovie {
		fullPath, err = renderMoviePath(match, parsed, tmpl, category)
	} else {
		fullPath, err = renderTVPath(match, parsed, tmpl, category)
	}
	if err != nil {
		return nil, err
	}

	fullPath = cleanPathChars(fullPath)
	fullPath = truncatePath(fullPath)

	dirPath := filepath.Dir(fullPath)
	fileName := filepath.Base(fullPath)

	return &OrganizedPath{
		FullPath: fullPath,
		DirPath:  dirPath,
		FileName: fileName,
	}, nil
}

func defaultNamingTemplate() *NamingTemplate {
	return &NamingTemplate{
		Movie:       defaultMovieTemplate,
		TVShowDir:   defaultTVShowDir,
		TVSeasonDir: defaultTVSeasonDir,
		TVFile:      defaultTVFile,
	}
}

func renderMoviePath(match *TMDBMatchResult, parsed *ParsedMedia, tmpl *NamingTemplate, category string) (string, error) {
	movieTmpl := tmpl.Movie
	if movieTmpl == "" {
		movieTmpl = defaultMovieTemplate
	}

	title := parsed.Title
	year := parsed.Year
	if match != nil && match.Matched {
		if match.Title != "" {
			title = match.Title
		}
		if match.Year > 0 {
			year = match.Year
		}
	}

	data := buildTemplateData(title, year, match, parsed, category)
	rendered, err := renderTemplate(movieTmpl, data)
	if err != nil {
		return "", err
	}

	if tmpl.EnableCategory && category != "" {
		rendered = filepath.Join("电影", category, rendered)
	}

	return rendered, nil
}

func renderTVPath(match *TMDBMatchResult, parsed *ParsedMedia, tmpl *NamingTemplate, category string) (string, error) {
	showDirTmpl := tmpl.TVShowDir
	if showDirTmpl == "" {
		showDirTmpl = defaultTVShowDir
	}
	seasonDirTmpl := tmpl.TVSeasonDir
	if seasonDirTmpl == "" {
		seasonDirTmpl = defaultTVSeasonDir
	}
	fileTmpl := tmpl.TVFile
	if fileTmpl == "" {
		fileTmpl = defaultTVFile
	}

	title := parsed.Title
	year := parsed.Year
	if match != nil && match.Matched {
		if match.Title != "" {
			title = match.Title
		}
		if match.Year > 0 {
			year = match.Year
		}
	}

	data := buildTemplateData(title, year, match, parsed, category)

	fileName, err := renderTemplate(fileTmpl, data)
	if err != nil {
		return "", fmt.Errorf("render tv file: %w", err)
	}

	showDir, err := renderTemplate(showDirTmpl, data)
	if err != nil {
		return "", fmt.Errorf("render show dir: %w", err)
	}

	seasonDir, err := renderTemplate(seasonDirTmpl, data)
	if err != nil {
		return "", fmt.Errorf("render season dir: %w", err)
	}

	fullPath := filepath.Join(showDir, seasonDir, fileName)

	if tmpl.EnableCategory && category != "" {
		fullPath = filepath.Join("电视剧", category, fullPath)
	}

	return fullPath, nil
}

func buildTemplateData(title string, year int, match *TMDBMatchResult, parsed *ParsedMedia, category string) map[string]string {
	season := parsed.Season
	if season == 0 {
		season = 1
	}
	episode := parsed.Episode
	if episode == 0 {
		episode = 1
	}
	episodePad := fmt.Sprintf("%02d", episode)
	// 多集（如 E01-E03）渲染成 Emby 标准区间，取首尾集号；单集保持 "01" 不变。
	if len(parsed.Episodes) > 1 {
		first := parsed.Episodes[0]
		last := parsed.Episodes[len(parsed.Episodes)-1]
		if last != first {
			episodePad = fmt.Sprintf("%02d-E%02d", first, last)
		}
	}

	ext := parsed.Extension
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	tmdbID := ""
	if match != nil && match.TMDBID > 0 {
		tmdbID = fmt.Sprintf("%d", match.TMDBID)
	}

	return map[string]string{
		"title":        title,
		"year":         formatYear(year),
		"season":       fmt.Sprintf("%d", season),
		"seasonPad":    fmt.Sprintf("%02d", season),
		"episode":      fmt.Sprintf("%d", episode),
		"episodePad":   episodePad,
		"resolution":   parsed.Resolution,
		"videoCodec":   parsed.VideoCodec,
		"audioCodec":   parsed.AudioCodec,
		"source":       parsed.Source,
		"quality":      parsed.Quality,
		"group":        parsed.ReleaseGroup,
		"ext":          ext,
		"category":     category,
		"dynamicRange": parsed.DynamicRange,
		"tmdbId":       tmdbID,
	}
}

func formatYear(year int) string {
	if year > 0 {
		return fmt.Sprintf("%d", year)
	}
	return ""
}

func renderTemplate(tmplStr string, data map[string]string) (string, error) {
	funcMap := template.FuncMap{}
	for k, v := range data {
		val := v
		funcMap[k] = func() string { return val }
	}

	tmpl, err := template.New("media").Funcs(funcMap).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, nil); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return cleanEmptySegments(buf.String()), nil
}

func cleanEmptySegments(s string) string {
	for strings.Contains(s, "...") {
		s = strings.ReplaceAll(s, "...", "..")
	}
	s = strings.ReplaceAll(s, "..", ".")

	// 悬挂分隔符: " - ." → 去掉 " - ", ". - " → 去掉 " - "
	s = regexp.MustCompile(`\s+-\s*\.\s*`).ReplaceAllString(s, ".")
	s = regexp.MustCompile(`\.\s*-\s*`).ReplaceAllString(s, ".")

	// 尾部/首部悬挂分隔符
	s = regexp.MustCompile(`\s+-\s*$`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`^\s*-\s+`).ReplaceAllString(s, "")

	return s
}

func cleanPathChars(path string) string {
	parts := strings.Split(path, string(filepath.Separator))
	for i, part := range parts {
		cleaned := reInvalidChars.ReplaceAllString(part, " ")
		cleaned = reMultiSpace.ReplaceAllString(cleaned, " ")
		cleaned = strings.TrimSpace(cleaned)
		parts[i] = cleaned
	}
	return strings.Join(parts, string(filepath.Separator))
}

func truncatePath(path string) string {
	if len(path) <= maxPathLen {
		return path
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	maxNameLen := maxFilenameLen - len(ext) - 1
	if maxNameLen < 1 {
		maxNameLen = 1
	}

	if len(name) > maxNameLen {
		// 按 rune 边界截断，避免把中文/多字节字符截成乱码。
		runes := []rune(name)
		byteCount := 0
		cut := len(runes)
		for i, r := range runes {
			size := len(string(r))
			if byteCount+size > maxNameLen {
				cut = i
				break
			}
			byteCount += size
		}
		name = string(runes[:cut])
		name = strings.TrimRight(name, " .-_")
	}

	truncated := name + ext
	if dir != "." {
		truncated = filepath.Join(dir, truncated)
	}

	return truncated
}
