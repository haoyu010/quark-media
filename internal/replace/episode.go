package replace

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Episode patterns aligned with QAS extract_episode_number main paths.
var (
	reSpecSxxEyyDot = regexp.MustCompile(`(?i)[Ss](\d+)[Ee](\d{1,2})[._\-/]\d{1,2}`)
	reSxxExx        = regexp.MustCompile(`(?i)[Ss]\d{1,2}[Ee](\d{1,3})`)
	reEP            = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])(?:EP|E)[\s._-]*(\d{1,3})(?:[^A-Za-z0-9]|$)`)
	reCNEpisode     = regexp.MustCompile(`第\s*(\d{1,3})\s*[集话回]`)
	reEndEpisode    = regexp.MustCompile(`(?i)(?:^|[^0-9])(\d{1,3})\s*(?:集|话)(?:[^0-9]|$)`)
	// strip tech specs before loose number match (subset of QAS list)
	reTechSpecs = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:240|360|480|540|720|900|960|1080|1440|2160|4320)[pP]`),
		regexp.MustCompile(`(?i)\b[248]\s*K\b`),
		regexp.MustCompile(`(?i)\b[Hh]\.?(?:264|265)\b`),
		regexp.MustCompile(`(?i)\b[Xx](?:264|265)\b`),
		regexp.MustCompile(`(?i)\b(?:23\.976|29\.97|59\.94|24|25|30|50|60)\s*FPS\b`),
		regexp.MustCompile(`(?i)\b(?:5\.1|7\.1|2\.0)\b`),
	}
	reDateLike = regexp.MustCompile(`(?:19|20)\d{2}[._\-/]?\d{1,2}[._\-/]?\d{1,2}`)
)

// ExtractEpisodeNumber ports QAS extract_episode_number core rules.
func ExtractEpisodeNumber(filename string) *int {
	if strings.TrimSpace(filename) == "" {
		return nil
	}
	base := filepath.Base(filename)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if stem == "" {
		stem = base
	}

	// SxxEyy.zz first
	if m := reSpecSxxEyyDot.FindStringSubmatch(stem); len(m) > 2 {
		n, _ := strconv.Atoi(m[2])
		return intPtr(n)
	}
	if m := reSxxExx.FindStringSubmatch(stem); len(m) > 1 {
		n, _ := strconv.Atoi(m[1])
		return intPtr(n)
	}
	if m := reCNEpisode.FindStringSubmatch(stem); len(m) > 1 {
		n, _ := strconv.Atoi(m[1])
		return intPtr(n)
	}
	if m := reEP.FindStringSubmatch(stem); len(m) > 1 {
		n, _ := strconv.Atoi(m[1])
		return intPtr(n)
	}
	if m := reEndEpisode.FindStringSubmatch(stem); len(m) > 1 {
		n, _ := strconv.Atoi(m[1])
		return intPtr(n)
	}

	// clean tech + dates then try trailing episode-like numbers
	clean := stem
	for _, re := range reTechSpecs {
		clean = re.ReplaceAllString(clean, " ")
	}
	clean = reDateLike.ReplaceAllString(clean, " ")
	// common: "Show.12" or "Show 12"
	reLoose := regexp.MustCompile(`(?:^|[\s._\-\[\(])(\d{1,3})(?:[\s._\-\]\)]|$)`)
	matches := reLoose.FindAllStringSubmatch(clean, -1)
	if len(matches) > 0 {
		// take last reasonable episode candidate 1..200
		for i := len(matches) - 1; i >= 0; i-- {
			n, _ := strconv.Atoi(matches[i][1])
			if n >= 1 && n <= 200 {
				return intPtr(n)
			}
		}
	}
	return nil
}

func intPtr(n int) *int { return &n }

// EpisodeFromFileInfo ports QAS _episode_from_file_info.
func EpisodeFromFileInfo(fileInfo map[string]any) *int {
	if fileInfo == nil {
		return nil
	}
	for _, key := range []string{"renamed_to", "file_name", "original_name", "name"} {
		name := asStr(fileInfo[key])
		if name == "" {
			continue
		}
		if ep := ExtractEpisodeNumber(name); ep != nil {
			return ep
		}
	}
	return nil
}