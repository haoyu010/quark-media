//go:build ignore
// +build ignore

package organize

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	reResolution         = regexp.MustCompile(`(?i)\b(2160p|2160i|1080p|1080i|720p|720i|576p|576i|480p|480i|4k|uhd)\b`)
	reVideoCodec         = regexp.MustCompile(`(?i)(?:\b(?:x?265|hevc|x?264|avc|av1|mpeg[24]?)\b|\bh[\.\s_-]?(?:265|264)\b)`)
	reAudioCodec         = regexp.MustCompile(`(?i)\b(truehd[\.\s_-]*atmos[\s._-]*[\d.]*|truehd[\s._-]*[\d.]*|atmos[\s._-]*[\d.]*|dts[\.\s_-]*hd[\.\s_-]*ma[\s._-]*[\d.]*|dts[\.\s_-]*hd[\s._-]*[\d.]*|dts[\s._-]*[\d.]*|ddp[\s._-]*[\d.]*[\s._-]*atmos|ddp[\s._-]*[\d.]*|eac3[\s._-]*[\d.]*|dd[\s._-]*[\d.]*|aac[\s._-]*[\d.]*|ac3|flac|opus|lpcm|pcm)\b`)
	reSource             = regexp.MustCompile(`(?i)\b(blu[\.\s_-]?ray[\.\s_-]?remux|bluray[\.\s_-]?remux|remux|blu[\.\s_-]?ray|bluray|bdmv|web[\.\s_-]?dl|webrip|web[\.\s_-]?dlrip|hdtv|dvdrip|dvd|hdrip|bdrip|bd[\.\s_-]?rip|hddvd|pdtv|sdtv|tvrip|cam|ts|tc|scr|r5)\b`)
	reSeasonEp           = regexp.MustCompile(`(?i)S(\d{1,4})E(\d{1,4})(?:E(\d{1,4}))?`)
	reSeasonOnly         = regexp.MustCompile(`(?i)\bS(\d{1,4})\b`)
	reEpisodeOnly        = regexp.MustCompile(`(?i)\bEP?(\d{1,4})(?:-EP?(\d{1,4}))?\b`)
	reSeasonWord         = regexp.MustCompile(`(?i)\bseason[\.\s]*(\d{1,4})\b`)
	reEpisodeWord        = regexp.MustCompile(`(?i)\bepisode[\.\s]*(\d{1,4})\b`)
	reYear               = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	reQuality            = regexp.MustCompile(`(?i)\b(remux|encode|proper|repack|extended[\.\s]?cut|unrated|directors?[\.\s]?cut|theatrical|imax)\b`)
	reReleaseGrp         = regexp.MustCompile(`(?:^|[\s._-])-?([A-Za-z0-9_]{2,20})(?:\.[\w]+)?$`)
	reReleaseGrpExplicit = regexp.MustCompile(`(?i)(?:^|[\s._-])(?:-|@)([\p{L}\p{N}][\p{L}\p{N}_-]{1,31})$`)
	reSeparator          = regexp.MustCompile(`[._]+`)
	reMultiSpace         = regexp.MustCompile(`\s{2,}`)
	reAtGroup            = regexp.MustCompile(`(?i)[\s._]*@[A-Za-z0-9_]+`)
	reBracketTag         = regexp.MustCompile(`\[([^\]]{1,30})\]`)
	reTMDBTag            = regexp.MustCompile(`(?i)(?:[\{\[]\s*tmdb(?:id)?[-_ .]?\d+\s*[\}\]]|\btmdb(?:id)?[-_ .]?\d+\b)`)
	reBitDepth           = regexp.MustCompile(`(?i)\b\d{1,2}[\s._-]*bit\b`)

	reEmptyParens   = regexp.MustCompile(`[\(（]\s*[\)）]`)
	reEmptyBrackets = regexp.MustCompile(`\[\s*\]`)
	reInlineDash    = regexp.MustCompile(`\s+[-–—]\s+`)
	reTrailingDash  = regexp.MustCompile(`\s+[-–—]$`)

	// 新增：流媒体来源识别
	reStreaming = regexp.MustCompile(`(?i)\b(dsnp|disney|nf|netflix|amzn|amazon|hiveweb|maxplus|cr|crunchyroll|mytvsuper|hidive|hulu|atvp|aptv|pcok|pmtp|peacock|hbomax|hmax|max|iview|kktv|linetv|viu|wetv|youku|iqiyi|baha|bglobal|bilibili|funimation|abema|abemax)\b`)

	reDynamicRange = regexp.MustCompile(`(?i)(杜比[\s._-]*(?:视界|视觉|vision)|\b(dolby[\.\s_-]?vision|dolbyvision|dovi|dv|hdr10\+|hdr10|hdr[\.\s_-]?vivid|hdr|hlg|sdr)\b)`)

	reHQ = regexp.MustCompile(`(?i)(?:hq|high[\.\s_-]?quality|高码|高码率)`)

	// 新增：FPS
	reFPS = regexp.MustCompile(`(?i)\b(\d{2,3})\s*fps\b`)

	reFileSize = regexp.MustCompile(`(?i)(?:^|[\s._-])\d+(?:[\.\s]\d+)?\s*(?:kb|mb|gb|tb)\b`)

	reMarketingWords = regexp.MustCompile(`(?i)(无损|hifi声|hifi|仅秒传|蓝光原盘|原盘|内封简繁|内封简中|内封中字|内嵌简中|内封|简繁|繁中|简中|中字|简体|繁体|多国音轨|多国语音|多语音轨|多语言|国粤双语|粤国双语|杜比音效|杜比视界|杜比全景|杜比环绕|杜比影院)`)

	// 新增：中文季集
	reChineseSeasonEp = regexp.MustCompile(`第\s*(\d{1,2})\s*季.*?第\s*(\d{1,3})\s*[集话]`)
	reChineseEpisode  = regexp.MustCompile(`第\s*(\d{1,3})\s*[集话回期]`)
	reChineseSeason   = regexp.MustCompile(`第\s*(\d{1,2})\s*季`)

	// 新增：Part/分片
	// 注意：[上下] 只匹配独立的上下（前后是分隔符或边界），避免误匹配 "北上"、"上海" 等词
	rePart = regexp.MustCompile(`(?i)(?:\bpart[ ._-]?\d+|\bcd[ ._-]?\d+|\bdisc[ ._-]?\d+|(?:^|[._\s-])[上下](?:[集部]|[._\s-]|$))`)

	reResidualSeasonToken = regexp.MustCompile(`(?i)(?:^|[\s+._-])S\d{1,4}(?:$|[\s+._-])`)
	reEpisodeCountTag     = regexp.MustCompile(`(?i)(?:^|[\s._-])(?:\d{1,4}\s*[集话]全|全\s*\d{1,4}\s*[集话])(?:$|[\s._-])`)
	reFrameRateMode       = regexp.MustCompile(`(?i)\bHFR\b`)
	reEpisodeExtraTag     = regexp.MustCompile(`(?i)(加更|先导片|花絮|全员现状番外|番外|彩蛋|预告片?|幕后)`)
	reLooseChannelsToken  = regexp.MustCompile(`(?i)(?:^|[\s._-])\d+\s+\d+(?:$|[\s._-])`)

	reJunkWords = regexp.MustCompile(`(?i)\b(10[\s._-]*bit|hdr10|hdr|dv|dovi|dolby|atmos|flac|aac[\s._-]*[\d.]*|dts[\s._-]*[\d.]*|ddp[\s._-]*[\d.]*|dd[\s._-]*[\d.]*|truehd|hevc|x265|x264|h265|h264|av1|avc|web|blu[\s._-]?ray|bluray|remux|hdtv|dvdrip|webrip|webdl|720p|720i|1080p|1080i|2160p|2160i|576p|576i|480p|480i|4k|uhd|ultra[\s._-]*hd|proper|repack|extended|unrated|directors?cut|theatrical|imax|高码|高码率|mp3|hfr|iq|edr|p\d{1,2}|tv|bd|dsnp|disney|netflix|amzn|amazon|hiveweb|maxplus|mytvsuper|cr|crunchyroll|hidive|hulu|atvp|aptv|pcok|pmtp|peacock|hbomax|hmax|max|iview|kktv|linetv|viu|wetv|youku|iqiyi|baha|bglobal|bilibili|funimation|abema|abemax|vivid)\b`)

	reTitleResidue      = regexp.MustCompile(`(?i)\b(2160[pi]|1080[pi]|720[pi]|576[pi]|480[pi]|4k|uhd|web[\s._-]?dl|webrip|blu[\s._-]?ray|bluray|remux|hdtv|x?265|x?264|h[\s._-]?265|h[\s._-]?264|hevc|avc|aac|ddp|dts|truehd|atmos|hdr10\+?|hdr|dovi|dv|sdr)\b`)
	reTitleOnlyYear     = regexp.MustCompile(`^(19\d{2}|20\d{2})$`)
	reTrailingGroupLike = regexp.MustCompile(`\s*[-–—]\s*[\p{L}\p{N}_]{1,24}$`)

	reYearInParens = regexp.MustCompile(`\s*[\(（]\s*(?:19\d{2}|20\d{2})\s*[\)）]\s*`)
)

var (
	titleBoundaryPatterns = []*regexp.Regexp{
		reChineseSeasonEp,
		reChineseEpisode,
		reChineseSeason,
		reSeasonEp,
		reSeasonWord,
		reSeasonOnly,
		reEpisodeWord,
		reEpisodeOnly,
		reResolution,
		reSource,
		reVideoCodec,
		reAudioCodec,
		reDynamicRange,
		reStreaming,
		reFPS,
		reBitDepth,
		reHQ,
	}
	tailMetaPatterns = []*regexp.Regexp{
		reChineseSeasonEp,
		reChineseEpisode,
		reChineseSeason,
		reSeasonEp,
		reSeasonWord,
		reSeasonOnly,
		reEpisodeWord,
		reEpisodeOnly,
		reResolution,
		reSource,
		reVideoCodec,
		reAudioCodec,
		reDynamicRange,
		reStreaming,
		reFPS,
		reBitDepth,
		reHQ,
	}
)

func ParseFilename(filename string) (*ParsedMedia, error) {
	if filename == "" {
		return nil, ErrEmptyFilename
	}

	media := &ParsedMedia{
		Original: filename,
	}

	ext := filepath.Ext(filename)
	media.Extension = ext
	name := strings.TrimSuffix(filename, ext)
	if reFileSize.MatchString(filename) || reVideoCodec.MatchString(filename) && !reVideoCodec.MatchString(name) {
		name = filename
		media.Extension = ""
	}

	// 在提取技术参数之前先清理 @Group 和 [TAG]，避免干扰音频通道数匹配
	name = reAtGroup.ReplaceAllString(name, " ")
	name = reTMDBTag.ReplaceAllString(name, " ")
	media.ReleaseGroup = extractBracketTag(&name)
	structuredTitle := extractStructuredTitleCandidate(name)

	media.AudioCodec = extractAudioCodec(&name)
	media.VideoCodec = extractVideoCodec(&name)

	working := normalizeSeparators(name)

	media.Season, media.Episode, media.Episodes = extractSeasonEpisode(&working)
	working = reResidualSeasonToken.ReplaceAllString(working, " ")
	media.Year = extractYear(&working)
	media.Resolution = extractPattern(&working, reResolution)
	media.Source = extractSource(&working)
	media.Quality = extractPattern(&working, reQuality)
	media.DynamicRange = extractDynamicRange(&working)
	media.HQ = extractBool(&working, reHQ)
	media.FPS = extractFPS(&working)
	working = reBitDepth.ReplaceAllString(working, " ")
	working = reFileSize.ReplaceAllString(working, " ")
	working = reMarketingWords.ReplaceAllString(working, " ")
	working = reEpisodeCountTag.ReplaceAllString(working, " ")
	working = reFrameRateMode.ReplaceAllString(working, " ")
	working = reEpisodeExtraTag.ReplaceAllString(working, " ")
	working = reLooseChannelsToken.ReplaceAllString(working, " ")
	media.Part = extractPart(&working)
	media.ReleaseGroup = extractReleaseGroup(&working)

	fallbackTitle := cleanTitle(working)
	if shouldPreferStructuredTitle(structuredTitle, fallbackTitle) {
		media.Title = structuredTitle
	} else {
		media.Title = fallbackTitle
	}
	media.IsMovie = media.Season == 0 && media.Episode == 0

	return media, nil
}

func normalizeSeparators(name string) string {
	result := reSeparator.ReplaceAllString(name, " ")
	return strings.TrimSpace(result)
}

func extractSeasonEpisode(working *string) (int, int, []int) {
	// 新增：中文季集优先检测
	if m := reChineseSeasonEp.FindStringSubmatch(*working); m != nil {
		*working = strings.Replace(*working, m[0], " ", 1)
		s := parseInt(m[1])
		e := parseInt(m[2])
		return s, e, []int{e}
	}
	if m := reChineseEpisode.FindStringSubmatch(*working); m != nil {
		*working = strings.Replace(*working, m[0], " ", 1)
		e := parseInt(m[1])
		// Chinese episode matched without Chinese season — check for S## or Season ## pattern
		if sm := reSeasonOnly.FindStringSubmatch(*working); sm != nil {
			*working = strings.Replace(*working, sm[0], " ", 1)
			return parseInt(sm[1]), e, []int{e}
		}
		if sm := reSeasonEp.FindStringSubmatch(*working); sm != nil {
			*working = strings.Replace(*working, sm[0], " ", 1)
			return parseInt(sm[1]), e, []int{e}
		}
		return 1, e, []int{e}
	}

	// 中文「第N季」无集号：仅季号，命中后 Season>0，避免被误判成电影
	if m := reChineseSeason.FindStringSubmatch(*working); m != nil {
		*working = strings.Replace(*working, m[0], " ", 1)
		return parseInt(m[1]), 0, nil
	}

	if m := reSeasonEp.FindStringSubmatch(*working); m != nil {
		*working = strings.Replace(*working, m[0], " ", 1)
		season := parseInt(m[1])
		episode := parseInt(m[2])
		episodes := []int{episode}
		if m[3] != "" {
			episodes = append(episodes, parseInt(m[3]))
		}
		return season, episode, episodes
	}

	if m := reSeasonWord.FindStringSubmatch(*working); m != nil {
		*working = strings.Replace(*working, m[0], " ", 1)
		return parseInt(m[1]), 0, nil
	}

	if m := reEpisodeWord.FindStringSubmatch(*working); m != nil {
		*working = strings.Replace(*working, m[0], " ", 1)
		return 0, parseInt(m[1]), []int{parseInt(m[1])}
	}

	if m := reSeasonOnly.FindStringSubmatch(*working); m != nil {
		*working = strings.Replace(*working, m[0], " ", 1)
		return parseInt(m[1]), 0, nil
	}

	if m := reEpisodeOnly.FindStringSubmatch(*working); m != nil {
		*working = strings.Replace(*working, m[0], " ", 1)
		ep := parseInt(m[1])
		episodes := []int{ep}
		if m[2] != "" {
			episodes = append(episodes, parseInt(m[2]))
		}
		return 0, ep, episodes
	}

	return 0, 0, nil
}

func extractYear(working *string) int {
	matches := reYear.FindAllStringSubmatch(*working, -1)
	for _, m := range matches {
		year := parseInt(m[1])
		if year >= 1900 && year <= 2099 {
			*working = strings.Replace(*working, m[0], " ", 1)
			return year
		}
	}
	return 0
}

func extractPattern(working *string, pattern *regexp.Regexp) string {
	if m := pattern.FindString(*working); m != "" {
		*working = strings.Replace(*working, m, " ", 1)
		return normalizeCodec(m)
	}
	return ""
}

func extractAudioCodec(working *string) string {
	if m := reAudioCodec.FindString(*working); m != "" {
		*working = strings.Replace(*working, m, " ", 1)
		return normalizeAudioCodec(m)
	}
	return ""
}

func extractVideoCodec(working *string) string {
	if m := reVideoCodec.FindString(*working); m != "" {
		*working = strings.Replace(*working, m, " ", 1)
		return normalizeCodec(m)
	}
	return ""
}

func extractDynamicRange(working *string) string {
	all := reDynamicRange.FindAllString(*working, -1)
	if len(all) == 0 {
		return ""
	}

	seen := map[string]bool{}
	var parts []string
	for _, m := range all {
		norm := normalizeDynamicRange(m)
		if norm != "" && !seen[norm] {
			seen[norm] = true
			parts = append(parts, norm)
		}
	}

	for _, m := range all {
		*working = strings.Replace(*working, m, " ", 1)
	}

	if len(parts) == 0 {
		return ""
	}

	dv := ""
	hdr := ""
	sdr := ""
	for _, p := range parts {
		switch p {
		case "DV":
			dv = "DV"
		case "SDR":
			sdr = "SDR"
		default:
			if hdr == "" {
				hdr = p
			}
		}
	}

	if dv != "" && hdr != "" {
		return dv + "." + hdr
	}
	if dv != "" {
		return dv
	}
	if hdr != "" {
		return hdr
	}
	return sdr
}

func normalizeDynamicRange(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))
	compact := strings.NewReplacer(".", "", " ", "", "_", "", "-", "").Replace(lower)
	switch {
	case strings.Contains(s, "杜比视界") || strings.Contains(s, "杜比视觉") ||
		strings.Contains(compact, "dolbyvision") || strings.Contains(compact, "dovi") || compact == "dv":
		return "DV"
	case strings.Contains(lower, "hdr10+"):
		return "HDR10+"
	case strings.Contains(lower, "hdr10"):
		return "HDR10"
	case strings.Contains(lower, "vivid"):
		return "HDR.Vivid"
	case strings.Contains(lower, "hdr"):
		return "HDR"
	case strings.Contains(lower, "hlg"):
		return "HLG"
	case strings.Contains(lower, "sdr"):
		return "SDR"
	default:
		return ""
	}
}

func extractSource(working *string) string {
	// 先检测流媒体来源
	streamingMatch := reStreaming.FindString(*working)
	streamingNorm := normalizeStreaming(streamingMatch)

	// 再检测传统来源
	sourceMatch := reSource.FindString(*working)
	sourceNorm := normalizeSource(sourceMatch)

	// 按优先级选择
	best := selectBestSource(streamingNorm, sourceNorm)

	if best != "" {
		if streamingMatch != "" {
			*working = strings.Replace(*working, streamingMatch, " ", 1)
		}
		if sourceMatch != "" {
			*working = strings.Replace(*working, sourceMatch, " ", 1)
		}
	}
	return best
}

func extractBool(working *string, pattern *regexp.Regexp) bool {
	if pattern.MatchString(*working) {
		m := pattern.FindString(*working)
		*working = strings.Replace(*working, m, " ", 1)
		return true
	}
	return false
}

func extractReleaseGroup(working *string) string {
	if m := reReleaseGrpExplicit.FindStringSubmatch(*working); m != nil && len(m) > 1 {
		group := strings.TrimSpace(m[1])
		if l := len([]rune(group)); l >= 2 && l <= 32 {
			*working = strings.Replace(*working, m[0], " ", 1)
			return group
		}
	}

	m := reReleaseGrp.FindStringSubmatch(*working)
	if m != nil && len(m) > 1 {
		group := m[1]
		if len(group) >= 2 && len(group) <= 20 {
			*working = strings.Replace(*working, m[0], " ", 1)
			return group
		}
	}
	return ""
}

func extractBracketTag(working *string) string {
	matches := reBracketTag.FindAllStringSubmatch(*working, -1)
	var group string
	for _, m := range matches {
		if len(m) > 1 {
			tag := m[1]
			if isMediaTag(tag) {
				group = tag
			}
			*working = strings.Replace(*working, m[0], " ", 1)
		}
	}
	return group
}

func isMediaTag(tag string) bool {
	mediaTags := []string{"CAS", "4K", "UHD", "HDR", "DV", "ATMOS", "REMUX", "PROPER", "REPACK"}
	upper := strings.ToUpper(tag)
	for _, t := range mediaTags {
		if upper == t {
			return true
		}
	}
	return false
}

func extractFPS(working *string) int {
	m := reFPS.FindStringSubmatch(*working)
	if m != nil {
		*working = strings.Replace(*working, m[0], " ", 1)
		return parseInt(m[1])
	}
	return 0
}

func extractPart(working *string) string {
	if m := rePart.FindString(*working); m != "" {
		*working = strings.Replace(*working, m, " ", 1)
		// 清理匹配结果中的分隔符，只保留实际的 part 标识
		return strings.Trim(m, "._- ")
	}
	return ""
}

func extractStructuredTitleCandidate(name string) string {
	normalized := normalizeSeparators(name)
	if normalized == "" {
		return ""
	}

	boundary, reason := findTitleBoundary(normalized)
	if boundary <= 0 || boundary >= len(normalized) {
		return ""
	}

	if reason == "year" && !containsTailMeta(normalized[boundary:]) {
		return ""
	}

	candidate := cleanTitle(normalized[:boundary])
	if candidate == "" {
		return ""
	}
	// Strip release year in parentheses — it's stored separately in media.Year.
	// Only strip when wrapped in parens: "挽救计划 (2026)" → "挽救计划"
	// but keep bare years that are part of the title: "你好1998", "Blade Runner 2049".
	candidate = reYearInParens.ReplaceAllString(candidate, " ")
	candidate = reMultiSpace.ReplaceAllString(candidate, " ")
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}
	return normalizeStructuredTitleCandidate(candidate)
}

func findTitleBoundary(normalized string) (int, string) {
	best := -1
	bestReason := ""
	for _, pattern := range titleBoundaryPatterns {
		if m := pattern.FindStringIndex(normalized); m != nil && m[0] > 0 {
			if best == -1 || m[0] < best {
				best = m[0]
				bestReason = "meta"
			}
		}
	}

	yearMatches := reYear.FindAllStringIndex(normalized, -1)
	if len(yearMatches) == 1 {
		yearStart := yearMatches[0][0]
		yearEnd := yearMatches[0][1]
		if yearStart > 0 && containsTailMeta(normalized[yearEnd:]) {
			// Include opening paren + preceding space so candidate isn't cut mid-bracket.
			// "挽救计划 (2026) 2160p…" → boundary before " (" instead of before "2026".
			adjustedStart := yearStart
			if adjustedStart > 0 {
				if normalized[adjustedStart-1] == '(' {
					adjustedStart--
					for adjustedStart > 0 && normalized[adjustedStart-1] == ' ' {
						adjustedStart--
					}
				} else if adjustedStart >= 3 {
					// Full-width （ is 3 bytes in UTF-8
					if r, _ := utf8.DecodeRuneInString(normalized[adjustedStart-3:]); r == '（' {
						adjustedStart -= 3
						for adjustedStart > 0 && normalized[adjustedStart-1] == ' ' {
							adjustedStart--
						}
					}
				}
			}
			if best == -1 || adjustedStart < best {
				best = adjustedStart
				bestReason = "year"
			}
		}
	}

	return best, bestReason
}

func containsTailMeta(tail string) bool {
	tail = strings.TrimSpace(tail)
	if tail == "" {
		return false
	}
	for _, pattern := range tailMetaPatterns {
		if pattern.MatchString(tail) {
			return true
		}
	}
	return false
}

func shouldPreferStructuredTitle(structuredTitle, fallbackTitle string) bool {
	structuredTitle = strings.TrimSpace(structuredTitle)
	fallbackTitle = strings.TrimSpace(fallbackTitle)
	if structuredTitle == "" {
		return false
	}
	if fallbackTitle == "" {
		return true
	}
	if mediaTitleKey(structuredTitle) == mediaTitleKey(fallbackTitle) {
		return false
	}
	if reTitleOnlyYear.MatchString(fallbackTitle) {
		return true
	}
	if titleLooksNoisy(fallbackTitle) {
		return true
	}
	if strings.Contains(strings.ToLower(fallbackTitle), strings.ToLower(structuredTitle)) && len([]rune(fallbackTitle))-len([]rune(structuredTitle)) >= 4 {
		return true
	}
	return false
}

func normalizeStructuredTitleCandidate(title string) string {
	parts := strings.Fields(strings.TrimSpace(title))
	if len(parts) == 2 && reTitleOnlyYear.MatchString(parts[0]) && reTitleOnlyYear.MatchString(parts[1]) {
		return parts[0]
	}
	return title
}

func titleLooksNoisy(title string) bool {
	t := strings.TrimSpace(title)
	if t == "" {
		return false
	}
	if reTitleResidue.MatchString(t) {
		return true
	}
	return reTrailingGroupLike.MatchString(t)
}

func cleanTitle(raw string) string {
	title := reJunkWords.ReplaceAllString(raw, " ")
	title = reMultiSpace.ReplaceAllString(title, " ")
	title = strings.TrimSpace(title)
	title = strings.Trim(title, "-–—")
	title = strings.TrimSpace(title)

	title = reEmptyParens.ReplaceAllString(title, " ")
	title = reEmptyBrackets.ReplaceAllString(title, " ")
	title = reMultiSpace.ReplaceAllString(title, " ")
	title = strings.TrimSpace(title)

	title = reInlineDash.ReplaceAllString(title, " ")
	title = reTrailingDash.ReplaceAllString(title, "")
	title = reMultiSpace.ReplaceAllString(title, " ")
	title = strings.TrimSpace(title)

	if title == "" {
		return ""
	}

	var cleaned []rune
	prevSpace := false
	for _, r := range title {
		if r == ' ' {
			if !prevSpace {
				cleaned = append(cleaned, r)
			}
			prevSpace = true
		} else {
			cleaned = append(cleaned, r)
			prevSpace = false
		}
	}
	title = string(cleaned)
	title = strings.TrimSpace(title)

	if len(title) > 0 {
		runes := []rune(title)
		runes[0] = unicode.ToUpper(runes[0])
		title = string(runes)
	}

	return title
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

func normalizeCodec(s string) string {
	upper := strings.ToUpper(s)
	switch upper {
	case "H265", "H.265", "H 265", "H-265", "H_265", "HEVC", "X265":
		return "HEVC"
	case "H264", "H.264", "H 264", "H-264", "H_264", "AVC", "X264":
		return "H264"
	case "AV1":
		return "AV1"
	case "MPEG2":
		return "MPEG2"
	case "MPEG4":
		return "MPEG4"
	default:
		return strings.ToUpper(s)
	}
}

func normalizeAudioCodec(s string) string {
	upper := strings.ToUpper(s)
	channels := extractChannels(upper)
	normalized := strings.NewReplacer(
		".", " ", "_", " ", "-", " ",
	).Replace(upper)

	switch {
	case strings.Contains(normalized, "TRUEHD") && strings.Contains(normalized, "ATMOS"):
		if channels != "" {
			return "TrueHD Atmos " + channels
		}
		return "TrueHD Atmos"
	case strings.Contains(normalized, "TRUEHD"):
		if channels != "" {
			return "TrueHD " + channels
		}
		return "TrueHD"
	case strings.Contains(normalized, "DDP") && strings.Contains(normalized, "ATMOS"):
		if channels != "" {
			return "DDP" + channels + " Atmos"
		}
		return "DDP Atmos"
	case strings.Contains(normalized, "ATMOS"):
		return "Atmos"
	case strings.Contains(normalized, "DTS HD") && strings.Contains(normalized, "MA"):
		if channels != "" {
			return "DTS-HD MA " + channels
		}
		return "DTS-HD MA"
	case strings.Contains(normalized, "DTS HD"):
		if channels != "" {
			return "DTS-HD " + channels
		}
		return "DTS-HD"
	case strings.Contains(normalized, "DTS"):
		if channels != "" {
			return "DTS " + channels
		}
		return "DTS"
	case strings.Contains(normalized, "DDP") || strings.Contains(normalized, "EAC3"):
		if channels != "" {
			return "DDP" + channels
		}
		return "DDP"
	case strings.Contains(normalized, "DD") || strings.Contains(normalized, "AC3"):
		if channels != "" {
			return "DD" + channels
		}
		return "DD"
	case strings.Contains(normalized, "AAC"):
		if channels != "" {
			return "AAC" + channels
		}
		return "AAC"
	case strings.Contains(normalized, "FLAC"):
		return "FLAC"
	case strings.Contains(normalized, "OPUS"):
		return "Opus"
	case strings.Contains(normalized, "LPCM") || strings.Contains(normalized, "PCM"):
		return "LPCM"
	default:
		return strings.ToUpper(s)
	}
}

var reChannels = regexp.MustCompile(`\d+\.\d+`)

func extractChannels(s string) string {
	if m := reChannels.FindString(s); m != "" {
		return m
	}
	return ""
}

func normalizeSource(s string) string {
	upper := strings.ToUpper(s)
	switch {
	case strings.Contains(upper, "BLURAY") && strings.Contains(upper, "REMUX"):
		return "BluRay Remux"
	case strings.Contains(upper, "REMUX"):
		return "Remux"
	case strings.Contains(upper, "BLURAY") || strings.Contains(upper, "BDMV"):
		return "BluRay"
	case strings.Contains(upper, "WEB") && strings.Contains(upper, "DL"):
		return "WEB-DL"
	case strings.Contains(upper, "WEBRIP"):
		return "WEBRip"
	case strings.Contains(upper, "HDTV"):
		return "HDTV"
	case strings.Contains(upper, "DVDRIP"):
		return "DVDRip"
	case strings.Contains(upper, "DVD"):
		return "DVD"
	case strings.Contains(upper, "HDRIP"):
		return "HDRip"
	case strings.Contains(upper, "BDRIP"):
		return "BDRip"
	default:
		return strings.ToUpper(s)
	}
}

func selectBestSource(streaming, source string) string {
	pri := func(s string) int {
		switch s {
		case "Remux", "BluRay Remux":
			return 0
		case "BluRay":
			return 1
		case "DSNP":
			return 2
		case "NF":
			return 3
		case "AMZN":
			return 4
		case "MAXPLUS":
			return 5
		case "CR":
			return 6
		case "HiveWeb":
			return 6
		case "MyTVSuper":
			return 7
		case "WEB-DL":
			return 7
		case "WEBRip":
			return 8
		case "HDTV":
			return 9
		default:
			return 99
		}
	}
	if streaming == "" && source == "" {
		return ""
	}
	if streaming == "" {
		return source
	}
	if source == "" {
		return streaming
	}
	if pri(streaming) < pri(source) {
		return streaming
	}
	return source
}

func normalizeStreaming(s string) string {
	upper := strings.ToUpper(s)
	switch {
	case strings.Contains(upper, "DSNP") || strings.Contains(upper, "DISNEY"):
		return "DSNP"
	case strings.Contains(upper, "NF") || strings.Contains(upper, "NETFLIX"):
		return "NF"
	case strings.Contains(upper, "AMZN") || strings.Contains(upper, "AMAZON"):
		return "AMZN"
	case strings.Contains(upper, "HIVEWEB"):
		return "HiveWeb"
	case strings.Contains(upper, "MAXPLUS"):
		return "MAXPLUS"
	case strings.Contains(upper, "MYTVSUPER"):
		return "MyTVSuper"
	case upper == "CR" || strings.Contains(upper, "CRUNCHYROLL"):
		return "CR"
	default:
		return ""
	}
}
