package replace

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"quark-media/internal/channel"
	"quark-media/internal/qas"
	"quark-media/internal/quark"
	"quark-media/internal/tginbox"
)

// Default exclude keywords (QAS DEFAULT_MEDIA_EXCLUDE_KEYWORDS).
const defaultMediaExcludeKeywords = "海报,poster,posters,封面,cover,covers,图片,image,images," +
	"备用,backup,bak,重复,duplicate,duplicates," +
	"sample,samples,样片,预览,nfo,txt,url,jpg,jpeg,png,webp"

var qualityOrder = map[string]int{
	"480p": 480, "720p": 720, "1080p": 1080, "2k": 1440, "1440p": 1440,
	"4k": 2160, "2160p": 2160, "8k": 4320, "4320p": 4320,
}

var (
	reQualityToken = regexp.MustCompile(`(?i)\b(480p|720p|1080p|1440p|2160p|4320p|2k|4k|8k|web-dl|webrip|bluray|hdr|dv)\b`)
	reSeasonSE     = regexp.MustCompile(`(?i)[Ss](\d{1,2})(?:[Ee]\d{1,3})?`)
	reSeasonCN     = regexp.MustCompile(`第\s*(\d{1,2})\s*季`)
	reURL          = regexp.MustCompile(`https?://\S+`)
	rePathJunk     = regexp.MustCompile(`[\\/:*?"<>|]+`)
	reSpaces       = regexp.MustCompile(`\s+`)
	reTokens       = regexp.MustCompile(`[\s._\-]+`)
)

// Settings mirrors QAS ResourceAutoReplacer._load_settings.
type Settings struct {
	Enabled       bool
	MinScore      int
	Sources       []string
	QualityPolicy string
	SearchTimeout int
}

// Result mirrors QAS try_replace result.
type Result struct {
	Attempted    bool           `json:"attempted"`
	Replaced     bool           `json:"replaced"`
	Message      string         `json:"message"`
	Reason       string         `json:"reason"`
	OldShareURL  string         `json:"old_shareurl,omitempty"`
	Best         map[string]any `json:"best,omitempty"`
	NewShareURL  string         `json:"new_shareurl,omitempty"`
}

// Replacer is QAS ResourceAutoReplacer.
type Replacer struct {
	Client   *quark.Client
	Channels []string
	Settings Settings
	Log      func(string)
	// optional: raw qas config path for reading task_settings
	QASPath string
	// extras for settings
	TaskSettings map[string]any
	// top-level auto_replace_invalid_shareurl override
	TopLevel any
}

func asStr(v any) string {
	if v == nil {
		return ""
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "<nil>" {
		return ""
	}
	return s
}

func asEnabled(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.TrimSpace(strings.ToLower(t))
		return s == "1" || s == "true" || s == "yes" || s == "enabled" || s == "on"
	case float64:
		return t != 0
	case int:
		return t != 0
	default:
		return false
	}
}

// LoadSettings from QAS config_data style maps.
func LoadSettings(topLevel any, taskSettings map[string]any) Settings {
	if taskSettings == nil {
		taskSettings = map[string]any{}
	}
	raw := topLevel
	if raw == nil {
		raw = taskSettings["auto_replace_invalid_shareurl"]
		if raw == nil {
			raw = "disabled"
		}
	}
	s := Settings{
		MinScore:      85,
		Sources:       []string{"telegram"},
		QualityPolicy: "no_downgrade",
		SearchTimeout: 8,
	}
	if m, ok := raw.(map[string]any); ok {
		s.Enabled = asEnabled(m["enabled"])
		if v := asStr(m["min_score"]); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				s.MinScore = n
			}
		} else if n, ok := m["min_score"].(float64); ok {
			s.MinScore = int(n)
		} else if n, ok := m["min_score"].(int); ok {
			s.MinScore = n
		}
		if v, ok := m["sources"].([]any); ok {
			s.Sources = nil
			for _, x := range v {
				s.Sources = append(s.Sources, strings.ToLower(asStr(x)))
			}
		} else if v := asStr(m["sources"]); v != "" {
			s.Sources = splitCSV(v)
		}
		if v := asStr(m["quality_policy"]); v != "" {
			s.QualityPolicy = v
		}
		if n, ok := m["timeout_seconds"].(float64); ok {
			s.SearchTimeout = int(n)
		}
	} else {
		s.Enabled = asEnabled(raw)
	}
	if v := asStr(taskSettings["auto_replace_min_score"]); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			s.MinScore = n
		}
	} else if n, ok := taskSettings["auto_replace_min_score"].(float64); ok {
		s.MinScore = int(n)
	}
	if v, ok := taskSettings["auto_replace_sources"].([]any); ok && len(v) > 0 {
		s.Sources = nil
		for _, x := range v {
			s.Sources = append(s.Sources, strings.ToLower(asStr(x)))
		}
	} else if v := asStr(taskSettings["auto_replace_sources"]); v != "" {
		s.Sources = splitCSV(v)
	}
	if v := asStr(taskSettings["auto_replace_quality_policy"]); v != "" {
		s.QualityPolicy = v
	}
	if n, ok := taskSettings["auto_replace_timeout_seconds"].(float64); ok {
		s.SearchTimeout = int(n)
	}
	if s.MinScore < 0 {
		s.MinScore = 0
	}
	if s.MinScore > 100 {
		s.MinScore = 100
	}
	if s.SearchTimeout < 2 {
		s.SearchTimeout = 2
	}
	if s.SearchTimeout > 60 {
		s.SearchTimeout = 60
	}
	allowed := map[string]bool{"telegram": true}
	var src []string
	for _, x := range s.Sources {
		x = strings.TrimSpace(strings.ToLower(x))
		if allowed[x] {
			src = append(src, x)
		}
	}
	if len(src) == 0 {
		src = []string{"telegram"}
	}
	s.Sources = src
	return s
}

func splitCSV(v string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' || r == ';' || r == '\n' }) {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// NewFromQAS builds replacer from qas config file + quark client + channel list.
func NewFromQAS(qasPath string, client *quark.Client, channels []string, logfn func(string)) *Replacer {
	ex := qas.LoadExtras(qasPath)
	// top-level key may exist in raw file
	top := loadTopLevelReplace(qasPath)
	if logfn == nil {
		logfn = func(string) {}
	}
	return &Replacer{
		Client:       client,
		Channels:     channels,
		Settings:     LoadSettings(top, ex.TaskSettings),
		Log:          logfn,
		QASPath:      qasPath,
		TaskSettings: ex.TaskSettings,
		TopLevel:     top,
	}
}

func loadTopLevelReplace(path string) any {
	// reuse ListTasks file read via raw
	b, err := readRaw(path)
	if err != nil || b == nil {
		return nil
	}
	return b["auto_replace_invalid_shareurl"]
}

func readRaw(path string) (map[string]any, error) {
	return qas.LoadRaw(path)
}

// TaskDisabled: movies or per-task disabled (QAS task_auto_replace_disabled).
func TaskDisabled(task map[string]any) bool {
	ct := strings.ToLower(asStr(task["content_type"]))
	if ct == "movie" || ct == "movies" || ct == "film" {
		return true
	}
	v := strings.ToLower(asStr(task["auto_replace_invalid_shareurl"]))
	return v == "disabled"
}

// IsInvalidShareErr detects share dead errors for triggering replace.
func IsInvalidShareErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	keys := []string{
		"失效", "不存在", "取消", "已删", "分享为空", "无效分享", "stoken",
		"forbidden", "not found", "deleted", "invalid", "share not", "获取 stoken",
		"提取链接", "分享链接",
	}
	for _, k := range keys {
		if strings.Contains(s, strings.ToLower(k)) {
			return true
		}
	}
	return false
}

// TryReplace implements QAS ResourceAutoReplacer.try_replace.
// task keys accept both QAS (taskname/shareurl/savepath) and Go (name/share_url/save_path).
func (r *Replacer) TryReplace(task map[string]any, reason string) Result {
	res := Result{Reason: reason}
	if task == nil {
		res.Message = "任务为空"
		return res
	}
	if !r.Settings.Enabled {
		res.Message = "自动换源未启用"
		return res
	}
	if TaskDisabled(task) {
		res.Message = "任务已禁用自动换源"
		return res
	}
	hasTG := false
	for _, s := range r.Settings.Sources {
		if s == "telegram" {
			hasTG = true
		}
	}
	if !hasTG || len(r.Channels) == 0 {
		res.Message = "未配置可用搜索来源"
		return res
	}
	queries := r.BuildSearchQueries(task)
	if len(queries) == 0 {
		res.Message = "无法生成搜索关键词"
		return res
	}
	res.Attempted = true
	baseline := r.buildQualityBaseline(task)
	candidates := r.collectCandidates(queries)
	type scoredItem struct {
		cand  map[string]any
		score int
		why   string
		snap  map[string]any
		ts    float64
	}
	var scored []scoredItem
	oldShare := firstShare(task)
	for _, cand := range candidates {
		share := asStr(cand["shareurl"])
		if share == "" {
			share = asStr(cand["url"])
		}
		if share == "" || sameShare(share, oldShare) {
			continue
		}
		snap := r.ValidateShare(share)
		ok, _ := snap["success"].(bool)
		if !ok {
			continue
		}
		files, _ := snap["files"].([]map[string]any)
		filtered := r.FilterFilesByTask(files, task)
		if len(filtered) == 0 {
			continue
		}
		snap["files"] = filtered
		score, why := r.ScoreCandidate(task, cand, snap, baseline)
		if score >= r.Settings.MinScore {
			scored = append(scored, scoredItem{
				cand: cand, score: score, why: why, snap: snap,
				ts: toTS(cand["publish_date"], cand["datetime"]),
			})
		}
	}
	if len(scored) == 0 {
		res.Message = "未找到达到质量阈值的有效替换链接"
		return res
	}
	// sort by score desc, ts desc
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score || (scored[j].score == scored[i].score && scored[j].ts > scored[i].ts) {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}
	best := scored[0]
	bestURL := asStr(best.cand["shareurl"])
	if bestURL == "" {
		bestURL = asStr(best.cand["url"])
	}
	// mutate task like QAS
	task["shareurl"] = bestURL
	task["share_url"] = bestURL
	task["shareurl_ban"] = nil
	res.Replaced = true
	res.OldShareURL = oldShare
	res.NewShareURL = bestURL
	res.Message = fmt.Sprintf("已替换失效链接：%s -> %s", oldShare, bestURL)
	filesOut, _ := best.snap["files"].([]map[string]any)
	// keep raw files for startfid selection; floor filter applied at save time
	res.Best = map[string]any{
		"shareurl": bestURL,
		"taskname": asStr(best.cand["taskname"]),
		"source":   asStr(best.cand["source"]),
		"score":    best.score,
		"reason":   best.why,
		"files":    filesOut,
	}
	return res
}

func firstShare(task map[string]any) string {
	if v := asStr(task["shareurl"]); v != "" {
		return v
	}
	return asStr(task["share_url"])
}

func firstName(task map[string]any) string {
	if v := asStr(task["taskname"]); v != "" {
		return v
	}
	return asStr(task["name"])
}

func firstSave(task map[string]any) string {
	if v := asStr(task["savepath"]); v != "" {
		return v
	}
	return asStr(task["save_path"])
}

// BuildSearchQueries QAS build_search_queries.
func (r *Replacer) BuildSearchQueries(task map[string]any) []string {
	seeds := []string{
		firstName(task),
		asStr(task["episode_naming"]),
		asStr(task["sequence_naming"]),
		asStr(task["pattern"]),
		path.Base(strings.ReplaceAll(firstSave(task), "\\", "/")),
	}
	var queries []string
	seen := map[string]bool{}
	add := func(q string) {
		q = strings.TrimSpace(q)
		if q == "" || seen[q] {
			return
		}
		seen[q] = true
		queries = append(queries, q)
	}
	for _, seed := range seeds {
		clean := cleanQuery(seed)
		add(clean)
		add(removeQualityTerms(clean))
		if len(queries) >= 5 {
			break
		}
	}
	if len(queries) > 5 {
		queries = queries[:5]
	}
	return queries
}

func (r *Replacer) collectCandidates(queries []string) []map[string]any {
	seen := map[string]bool{}
	var out []map[string]any
	limit := 20
	for _, q := range queries {
		hits, err := channel.SearchPublic(r.Channels, []string{q}, limit)
		if err != nil {
			r.Log(fmt.Sprintf("自动换源搜索失败 [%s]: %v", q, err))
			continue
		}
		for _, h := range hits {
			key := tginbox.NormalizeShareID(h.URL)
			if key == "" {
				key = h.URL
			}
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, map[string]any{
				"shareurl": h.URL,
				"url":      h.URL,
				"taskname": h.Text,
				"content":  h.Text,
				"source":   "telegram:" + h.Channel,
			})
		}
	}
	return out
}

// ValidateShare QAS validate_share.
func (r *Replacer) ValidateShare(shareURL string) map[string]any {
	if r.Client == nil {
		return map[string]any{"success": false, "files": []map[string]any{}, "error": "no client"}
	}
	pwdID, pc := r.Client.ParseShare(shareURL)
	if pwdID == "" {
		return map[string]any{"success": false, "files": []map[string]any{}, "error": "提取链接参数失败"}
	}
	stoken, err := r.Client.GetSToken(pwdID, pc)
	if err != nil {
		return map[string]any{"success": false, "files": []map[string]any{}, "error": err.Error()}
	}
	files, err := r.Client.ShareList(pwdID, stoken, "0")
	if err != nil {
		return map[string]any{"success": false, "files": []map[string]any{}, "error": err.Error()}
	}
	// if single dir, expand one level
	if len(files) == 1 {
		if isDir(files[0]) {
			fid := asStr(files[0]["fid"])
			sub, err := r.Client.ShareList(pwdID, stoken, fid)
			if err == nil && len(sub) > 0 {
				files = sub
			}
		}
	}
	if len(files) == 0 {
		return map[string]any{"success": false, "files": []map[string]any{}, "error": "分享为空"}
	}
	return map[string]any{"success": true, "files": files, "error": ""}
}

func isDir(m map[string]any) bool {
	if m == nil {
		return false
	}
	if b, ok := m["dir"].(bool); ok {
		return b
	}
	// quark: file_type 0 dir sometimes
	ft := asStr(m["file_type"])
	return ft == "0" || ft == "dir"
}

// FilterFilesByTask QAS filter_files_by_task.
func (r *Replacer) FilterFilesByTask(files []map[string]any, task map[string]any) []map[string]any {
	var usable []map[string]any
	for _, item := range files {
		if item == nil || isDir(item) {
			continue
		}
		usable = append(usable, item)
	}
	filterwords := r.effectiveFilterwords(task)
	if filterwords == "" {
		return usable
	}
	normalized := strings.ReplaceAll(filterwords, "，", ",")
	if !strings.Contains(normalized, "|") {
		blocked := splitFilterWords(normalized)
		var out []map[string]any
		for _, item := range usable {
			if !matchesAnyFilter(item, blocked) {
				out = append(out, item)
			}
		}
		return out
	}
	parts := strings.Split(normalized, "|")
	filtered := usable
	for _, keepPart := range parts[:len(parts)-1] {
		keepPart = strings.TrimSpace(keepPart)
		words := splitFilterWords(keepPart)
		if len(words) == 0 {
			continue
		}
		var next []map[string]any
		for _, item := range filtered {
			name := strings.ToLower(fileName(item))
			ok := false
			for _, w := range words {
				if strings.Contains(name, w) {
					ok = true
					break
				}
			}
			if ok {
				next = append(next, item)
			}
		}
		filtered = next
	}
	blocked := splitFilterWords(parts[len(parts)-1])
	if len(blocked) > 0 {
		var out []map[string]any
		for _, item := range filtered {
			if !matchesAnyFilter(item, blocked) {
				out = append(out, item)
			}
		}
		return out
	}
	return filtered
}

func (r *Replacer) effectiveFilterwords(task map[string]any) string {
	taskFW := strings.TrimSpace(asStr(task["filterwords"]))
	defaultFW := defaultMediaExcludeKeywords
	if r.TaskSettings != nil {
		if _, ok := r.TaskSettings["media_exclude_keywords"]; ok {
			defaultFW = strings.TrimSpace(asStr(r.TaskSettings["media_exclude_keywords"]))
		}
	}
	if defaultFW == "" {
		return taskFW
	}
	if taskFW == "" {
		return defaultFW
	}
	if !strings.Contains(taskFW, "|") {
		return mergeFilterTerms(taskFW, defaultFW)
	}
	parts := strings.Split(taskFW, "|")
	block := strings.TrimSpace(parts[len(parts)-1])
	merged := mergeFilterTerms(block, defaultFW)
	return strings.Join(append(parts[:len(parts)-1], merged), "|")
}

func mergeFilterTerms(groups ...string) string {
	var merged []string
	seen := map[string]bool{}
	for _, g := range groups {
		g = strings.ReplaceAll(g, "，", ",")
		for _, term := range strings.Split(g, ",") {
			term = strings.TrimSpace(term)
			if term == "" {
				continue
			}
			key := strings.ToLower(term)
			if seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, term)
		}
	}
	return strings.Join(merged, ",")
}

func splitFilterWords(s string) []string {
	var out []string
	for _, w := range strings.Split(strings.ReplaceAll(s, "，", ","), ",") {
		w = strings.TrimSpace(strings.ToLower(w))
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

func fileName(item map[string]any) string {
	if v := asStr(item["file_name"]); v != "" {
		return v
	}
	return asStr(item["name"])
}

func matchesAnyFilter(item map[string]any, words []string) bool {
	name := strings.ToLower(fileName(item))
	text := strings.ToLower(strings.Join([]string{
		fileName(item),
		asStr(item["relative_path"]),
		asStr(item["path"]),
	}, " "))
	ext := ""
	if i := strings.LastIndex(name, "."); i >= 0 {
		ext = name[i+1:]
	}
	for _, w := range words {
		if strings.Contains(text, w) || w == ext {
			return true
		}
	}
	return false
}

func (r *Replacer) buildQualityBaseline(task map[string]any) map[string]any {
	hintText := strings.Join([]string{
		firstName(task),
		firstSave(task),
		asStr(task["pattern"]),
		asStr(task["replace"]),
		asStr(task["filterwords"]),
		asStr(task["episode_naming"]),
		asStr(task["sequence_naming"]),
	}, " ")
	files := loadSavedFilesFromTask(task)
	var names []string
	for _, f := range files {
		names = append(names, fileName(f))
	}
	savedText := strings.Join(names, " ")
	req := extractMaxResolution(hintText)
	if s := extractMaxResolution(savedText); s > req {
		req = s
	}
	return map[string]any{
		"required_resolution": req,
		"avg_size":            avgSize(files),
	}
}

func loadSavedFilesFromTask(task map[string]any) []map[string]any {
	if task == nil {
		return nil
	}
	switch v := task["_saved_files_for_baseline"].(type) {
	case []map[string]any:
		return v
	case []any:
		var out []map[string]any
		for _, x := range v {
			if m, ok := x.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// ScoreCandidate QAS score_candidate.
func (r *Replacer) ScoreCandidate(task, candidate, snapshot, baseline map[string]any) (int, string) {
	files, _ := snapshot["files"].([]map[string]any)
	var names []string
	for _, f := range files {
		names = append(names, fileName(f))
	}
	candidateText := strings.Join([]string{
		asStr(candidate["taskname"]),
		asStr(candidate["content"]),
		strings.Join(names, " "),
	}, " ")
	reqRes, _ := baseline["required_resolution"].(int)
	candRes := extractMaxResolution(candidateText)
	if reqRes > 0 && candRes < reqRes {
		return 0, fmt.Sprintf("清晰度降低: 需要 %dp，候选 %v", reqRes, orUnknown(candRes))
	}
	if reqRes == 0 && candRes > 0 && candRes < 1080 {
		return 0, fmt.Sprintf("候选清晰度低于自动换源底线: %dp", candRes)
	}
	reqSeason := extractSeason(strings.Join([]string{
		firstName(task), asStr(task["episode_naming"]), asStr(task["sequence_naming"]), firstSave(task),
	}, " "))
	candSeason := extractSeason(candidateText)
	if reqSeason > 0 && candSeason > 0 && candSeason != reqSeason {
		return 0, fmt.Sprintf("季号不匹配: 需要 S%02d，候选 S%02d", reqSeason, candSeason)
	}
	baseAvg, _ := baseline["avg_size"].(float64)
	candAvg := avgSize(files)
	if baseAvg > 0 && candAvg > 0 && candAvg < baseAvg*0.65 {
		return 0, "文件体积明显下降"
	}
	score := 30
	score += titleRelevanceScore(task, candidateText)
	score += 15
	if reqRes > 0 {
		score += 20
	} else if candRes >= 2160 {
		score += 18
	} else if candRes >= 1080 {
		score += 14
	} else if candRes > 0 {
		score += 8
	}
	if reqSeason > 0 {
		score += 10
	} else if candSeason > 0 {
		score += 6
	}
	if baseAvg > 0 {
		if candAvg >= baseAvg*0.85 {
			score += 10
		} else {
			score += 5
		}
	} else if candAvg > 0 {
		score += 5
	}
	if asStr(candidate["source"]) != "" {
		score += 3
	}
	if asStr(candidate["publish_date"]) != "" || asStr(candidate["datetime"]) != "" {
		score += 2
	}
	if score > 100 {
		score = 100
	}
	return score, "有效链接且质量达标"
}

func orUnknown(n int) any {
	if n == 0 {
		return "未知"
	}
	return n
}

func extractMaxResolution(text string) int {
	if text == "" {
		return 0
	}
	lower := strings.ToLower(text)
	max := 0
	for token, val := range qualityOrder {
		// simple contains with boundaries soft
		if strings.Contains(lower, token) {
			if val > max {
				max = val
			}
		}
	}
	return max
}

func extractSeason(text string) int {
	if text == "" {
		return 0
	}
	if m := reSeasonSE.FindStringSubmatch(text); len(m) > 1 {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	if m := reSeasonCN.FindStringSubmatch(text); len(m) > 1 {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

func titleRelevanceScore(task map[string]any, candidateText string) int {
	query := strings.ToLower(removeQualityTerms(cleanQuery(firstName(task))))
	cand := strings.ToLower(candidateText)
	parts := reTokens.Split(query, -1)
	var tokens []string
	for _, t := range parts {
		t = strings.TrimSpace(t)
		if len(t) >= 2 && !strings.HasPrefix(t, "s0") {
			tokens = append(tokens, t)
		}
	}
	if len(tokens) == 0 {
		return 15
	}
	matched := 0
	for _, t := range tokens {
		if strings.Contains(cand, t) {
			matched++
		}
	}
	ratio := float64(matched) / float64(len(tokens))
	if ratio >= 0.8 {
		return 25
	}
	if ratio >= 0.5 {
		return 18
	}
	if ratio > 0 {
		return 10
	}
	return 0
}

func avgSize(files []map[string]any) float64 {
	var sum float64
	var n int
	for _, f := range files {
		if isDir(f) {
			continue
		}
		sz := float64(0)
		switch v := f["size"].(type) {
		case float64:
			sz = v
		case int:
			sz = float64(v)
		case int64:
			sz = float64(v)
		case string:
			sz, _ = strconv.ParseFloat(v, 64)
		}
		if sz > 0 {
			sum += sz
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func cleanQuery(value string) string {
	if value == "" {
		return ""
	}
	text := reURL.ReplaceAllString(value, " ")
	text = strings.ReplaceAll(text, "{}", " ")
	text = strings.ReplaceAll(text, "[]", " ")
	text = rePathJunk.ReplaceAllString(text, " ")
	text = reSpaces.ReplaceAllString(text, " ")
	return strings.Trim(text, " -_·")
}

func removeQualityTerms(value string) string {
	if value == "" {
		return ""
	}
	text := reQualityToken.ReplaceAllString(value, " ")
	text = reSpaces.ReplaceAllString(text, " ")
	return strings.Trim(text, " -_·")
}

func sameShare(left, right string) bool {
	a := tginbox.NormalizeShareID(left)
	b := tginbox.NormalizeShareID(right)
	return a != "" && b != "" && a == b
}

func toTS(vals ...any) float64 {
	for _, v := range vals {
		text := asStr(v)
		if text == "" {
			continue
		}
		for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02", time.RFC3339} {
			if tm, err := time.Parse(layout, text); err == nil {
				return float64(tm.Unix())
			}
		}
	}
	return 0
}