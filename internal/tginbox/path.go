package tginbox

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	yearRe    = regexp.MustCompile(`(?:\(|（|\b)((?:19|20)\d{2})(?:\)|）|\b)`)
	seasonRe  = regexp.MustCompile(`(?i)(?:S(\d{1,2})E\d{1,4}|S(\d{1,2})|Season\s*(\d{1,2})|第\s*(\d{1,2})\s*季)`)
	epRe      = regexp.MustCompile(`(?i)(?:S\d{1,2}E\d{1,4}|E[\s._-]?\d{1,4}|\d{1,3}\s*集|第\s*\d+\s*[集话回]|更新至|全集)`)
	shareIDRe = regexp.MustCompile(`(?i)(?:pan\.)?quark\.cn/s/([A-Za-z0-9]+)`)
)

func NormalizeShareID(shareURL string) string {
	m := shareIDRe.FindStringSubmatch(shareURL)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func ExtractYear(text string) string {
	m := yearRe.FindStringSubmatch(text)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func ExtractSeason(text string) int {
	m := seasonRe.FindStringSubmatch(text)
	if len(m) == 0 {
		return 0
	}
	for i := 1; i < len(m); i++ {
		if m[i] == "" {
			continue
		}
		n, _ := strconv.Atoi(m[i])
		if n > 0 {
			return n
		}
	}
	return 0
}

func LooksLikeSeries(text string) bool {
	if epRe.MatchString(text) {
		return true
	}
	if seasonRe.MatchString(text) {
		return true
	}
	low := strings.ToLower(text)
	for _, k := range []string{"全集", "更新至", "连载", "剧集", "电视剧", "season", "s01", "s02", "集"} {
		if strings.Contains(low, k) {
			return true
		}
	}
	return false
}

func SanitizeTitle(title string) string {
	title = strings.ReplaceAll(title, "\\n", " ")
	title = strings.ReplaceAll(title, "\n", " ")
	title = strings.ReplaceAll(title, "\r", " ")
	title = strings.TrimSpace(title)
	repl := strings.NewReplacer(
		"/", " ", "\\", " ", ":", " ", "*", " ", "?", " ", "\"", " ",
		"<", " ", ">", " ", "|", " ",
	)
	title = strings.TrimSpace(repl.Replace(title))
	title = regexp.MustCompile(`\s+`).ReplaceAllString(title, " ")
	if title == "" {
		return "TG收链资源"
	}
	if len([]rune(title)) > 80 {
		r := []rune(title)
		title = string(r[:80])
	}
	return title
}

func BuildSavePath(root, contentType, title, year string, season int, libraryCategory string) string {
	root = normalizePath(root)
	title = SanitizeTitle(title)
	if season <= 0 {
		season = 1
	}
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if contentType == "" {
		if LooksLikeSeries(title) {
			contentType = "tv"
		} else {
			contentType = "movie"
		}
	}
	category := map[string]string{
		"movie": "电影", "tv": "电视剧", "anime": "电视剧",
		"variety": "综艺", "documentary": "纪录片",
	}[contentType]
	if category == "" {
		category = "电影"
	}
	var rel string
	switch contentType {
	case "movie":
		folder := title
		if year != "" {
			folder = title + " (" + year + ")"
		}
		if libraryCategory != "" {
			rel = joinPath("电影", libraryCategory, folder)
		} else {
			rel = joinPath(category, folder)
		}
	case "tv", "anime":
		seasonDir := "Season " + pad2(season)
		if libraryCategory != "" {
			rel = joinPath("电视剧", libraryCategory, title, seasonDir)
		} else {
			rel = joinPath(category, title, seasonDir)
		}
	default:
		rel = joinPath(category, title)
	}
	if root == "" {
		return rel
	}
	return joinPath(root, rel)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func normalizePath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return strings.Trim(p, "/")
}

func joinPath(parts ...string) string {
	var out []string
	for _, p := range parts {
		p = normalizePath(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "/")
}
