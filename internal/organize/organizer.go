//go:build ignore
// +build ignore

package organize

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

var (
	reDirectoryNoise   = regexp.MustCompile(`(?i)(?:^|[\s._-])(4k|2160p|1080p|720p|uhd|web[\s._-]?dl|webrip|hdtv|bluray|remux|hevc|x265|x264|h264|h265|avc|aac|ddp|dts|atmos|dv|dovi|hdr|sdr|hifi|hiveweb|maxplus|nf|amzn|dsnp|cr|crunchyroll|hbomax|mytvsuper|hidive|hulu|atvp|aptv|pcok|pmtp|peacock|hmax|max|iview|kktv|linetv|viu|wetv|youku|iqiyi|baha|bglobal|bilibili|funimation|abema|abemax|edr|iq|p\d+)(?:$|[\s._-])`)
	reDirectoryCnNoise = regexp.MustCompile(`(?i)(臻彩|超分|豆瓣|合集|拼好剧|分辨率|内封|内嵌|字幕|音轨|仅秒传|双语|附剧场版|原盘)`)
	reDirTitleYear     = regexp.MustCompile(`^\s*(.+?)\s*[\(（]\s*(19\d{2}|20\d{2})\s*[\)）]\s*$`)
	reSeasonDirHint    = regexp.MustCompile(`(?i)^season[\s._-]*(\d{1,4})$|^s(\d{1,4})$|^第\s*(\d{1,4})\s*季$`)
	reBracketPrefix    = regexp.MustCompile(`^(?:\[[^\]]+\][\s._-]*)+`)
	reTrailingEpisode  = regexp.MustCompile(`(?:^|[._\s-])(\d{1,3})(?:$)`)
	rePureEpisodeStem  = regexp.MustCompile(`^\d{1,3}$`)
)

type organizer struct {
	config     Config
	matcher    *tmdbMatcher
	watchers   map[string]*fsWatcher
	classifier *CategoryClassifier
	scraper    *Scraper
	mu         sync.RWMutex
}

func New(cfg Config) (Organizer, error) {
	m, err := newTMDBMatcher(cfg.TMDBAPIKey, cfg.TMDBLanguage, cfg.TMDBBaseURL)
	if err != nil {
		return nil, fmt.Errorf("init tmdb matcher: %w", err)
	}

	org := &organizer{
		config:   cfg,
		matcher:  m,
		watchers: make(map[string]*fsWatcher),
	}

	if cfg.CategoryConfigPath != "" {
		classifier, err := NewCategoryClassifier(cfg.CategoryConfigPath)
		if err != nil {
			return nil, fmt.Errorf("load category config: %w", err)
		}
		org.classifier = classifier
	}

	if cfg.DetailProvider != nil {
		var httpClient *http.Client
		if cfg.HTTPClient != nil {
			if h, ok := cfg.HTTPClient.(*http.Client); ok {
				httpClient = h
			}
		}
		org.scraper = NewScraperWithOptions(cfg.DetailProvider, httpClient, ScraperOptions{
			ImageDownloadConcurrency: cfg.ImageDownloadConcurrency,
			ImageDownloadQueueSize:   cfg.ImageDownloadQueueSize,
			ImageDownloadTimeout:     time.Duration(cfg.ImageDownloadTimeoutSeconds) * time.Second,
		})
	} else if cfg.TMDBAPIKey != "" {
		defaultProvider := newDefaultDetailProvider(m.client, cfg.TMDBLanguage)
		org.scraper = NewScraperWithOptions(defaultProvider, nil, ScraperOptions{
			ImageDownloadConcurrency: cfg.ImageDownloadConcurrency,
			ImageDownloadQueueSize:   cfg.ImageDownloadQueueSize,
			ImageDownloadTimeout:     time.Duration(cfg.ImageDownloadTimeoutSeconds) * time.Second,
		})
	}

	return org, nil
}

func (o *organizer) ParseFilename(filename string) (*ParsedMedia, error) {
	return ParseFilename(filename)
}

func (o *organizer) MatchTMDB(ctx context.Context, parsed *ParsedMedia) (*TMDBMatchResult, error) {
	return o.matcher.match(ctx, parsed)
}

func (o *organizer) GeneratePath(match *TMDBMatchResult, parsed *ParsedMedia, tmpl *NamingTemplate) (*OrganizedPath, string, error) {
	if tmpl == nil && o.config.NamingTemplate != nil {
		tmpl = cloneNamingTemplate(o.config.NamingTemplate)
	}
	if tmpl == nil {
		tmpl = defaultNamingTemplate()
	}
	if o.config.NamingTemplate != nil && o.config.NamingTemplate.EnableCategory && !tmpl.EnableCategory {
		tmpl = cloneNamingTemplate(tmpl)
		tmpl.EnableCategory = true
	}

	var category string
	if o.classifier != nil && match != nil && match.Matched {
		mediaType := "movie"
		if !parsed.IsMovie {
			mediaType = "tv"
		}
		category = o.classifier.Classify(mediaType, match)
	}

	path, err := GeneratePathWithCategory(match, parsed, tmpl, category)
	return path, category, err
}

func cloneNamingTemplate(t *NamingTemplate) *NamingTemplate {
	if t == nil {
		return nil
	}
	cp := *t
	return &cp
}

func (o *organizer) applyDirectoryHints(sourcePath string, parsed *ParsedMedia) {
	if parsed == nil || sourcePath == "" {
		return
	}

	applyPathSeasonEpisodeHints(sourcePath, parsed)
	if parsed.IsMovie {
		return
	}

	hint := extractParentDirTitle(sourcePath)
	if hint == "" {
		return
	}
	parentParsed := parseDirectoryTitleHint(hint)
	if parentParsed == nil || parentParsed.Title == "" {
		return
	}

	// 监控目录通常按「剧名/Season/文件」投放；剧集文件名可能是英文发行名或带错误年份。
	// 如果目录标题与文件名标题只是标点/引号形态不同（例如 Journey's vs Journey‘s），
	// 保留更标准的文件名标题，只从目录补年份，避免目录噪声反向污染标题。
	if shouldKeepFilenameTitle(parsed.Title, parentParsed.Title, hint) {
		if parsed.Year == 0 && parentParsed.Year > 0 {
			parsed.Year = parentParsed.Year
		}
		return
	}

	// 对 TV 文件优先使用上级剧名目录，可避免同一季内匹配到不同 TMDB 条目。
	if parentParsed.Year > 0 || containsHan(parentParsed.Title) || !containsHan(parsed.Title) {
		parsed.Title = parentParsed.Title
		if parentParsed.Year > 0 {
			parsed.Year = parentParsed.Year
		}
	}
}

func parseDirectoryTitleHint(hint string) *ParsedMedia {
	if m := reDirTitleYear.FindStringSubmatch(strings.TrimSpace(hint)); len(m) == 3 {
		year, _ := strconv.Atoi(m[2])
		title := strings.TrimSpace(m[1])
		if title != "" {
			return &ParsedMedia{
				Title:   title,
				Year:    year,
				IsMovie: true,
			}
		}
	}
	parsed, err := ParseFilename(hint)
	if err != nil {
		return nil
	}
	return parsed
}

func applyPathSeasonEpisodeHints(sourcePath string, parsed *ParsedMedia) {
	if parsed == nil {
		return
	}

	season, episode := inferSeasonEpisodeFromPath(sourcePath)
	if parsed.Season == 0 && season > 0 {
		parsed.Season = season
	}
	if parsed.Episode == 0 && episode > 0 {
		parsed.Episode = episode
		parsed.Episodes = []int{episode}
	}
	if parsed.Episode > 0 {
		if parsed.Season == 0 {
			parsed.Season = 1
		}
		parsed.IsMovie = false
	}
}

func inferSeasonEpisodeFromPath(sourcePath string) (int, int) {
	season := inferSeasonFromDirs(sourcePath)
	episode := inferEpisodeFromFilename(sourcePath, season > 0)
	if season == 0 && episode > 0 {
		season = 1
	}
	return season, episode
}

func inferSeasonFromDirs(sourcePath string) int {
	dir := filepath.Dir(sourcePath)
	for i := 0; i < 6; i++ {
		name := strings.TrimSpace(filepath.Base(dir))
		if m := reSeasonDirHint.FindStringSubmatch(name); m != nil {
			for _, g := range m[1:] {
				if g == "" {
					continue
				}
				if v, _ := strconv.Atoi(g); v > 0 {
					return v
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return 0
}

func inferEpisodeFromFilename(sourcePath string, hasSeasonHint bool) int {
	stem := filepath.Base(sourcePath)
	if strings.HasSuffix(strings.ToLower(stem), ".strm") {
		stem = strings.TrimSuffix(stem, filepath.Ext(stem))
		innerExt := strings.ToLower(filepath.Ext(stem))
		switch innerExt {
		case ".mkv", ".mp4", ".avi", ".mov", ".wmv", ".flv", ".m4v", ".ts", ".rmvb", ".rm", ".3gp", ".mpg", ".mpeg", ".webm", ".m2ts":
			stem = strings.TrimSuffix(stem, filepath.Ext(stem))
		}
	} else {
		stem = strings.TrimSuffix(stem, filepath.Ext(stem))
	}

	stem = reBracketPrefix.ReplaceAllString(stem, "")
	stem = strings.TrimSpace(stem)
	if stem == "" {
		return 0
	}

	normalized := normalizeSeparators(stem)
	if normalized == "" {
		return 0
	}

	working := normalized
	if _, ep, _ := extractSeasonEpisode(&working); ep > 0 {
		return ep
	}

	if !hasSeasonHint && !rePureEpisodeStem.MatchString(strings.TrimSpace(normalized)) {
		return 0
	}

	if m := reTrailingEpisode.FindStringSubmatch(normalized); len(m) > 1 {
		if ep, _ := strconv.Atoi(m[1]); ep > 0 && ep <= 999 {
			return ep
		}
	}
	return 0
}

func (o *organizer) preferDetailedMatch(ctx context.Context, match *TMDBMatchResult) *TMDBMatchResult {
	if match == nil || !match.Matched || match.TMDBID <= 0 || match.MediaType == "" {
		return match
	}
	detailed, err := o.matcher.getByID(ctx, match.TMDBID, match.MediaType)
	if err != nil || detailed == nil || !detailed.Matched {
		return match
	}
	detailed.Confidence = match.Confidence
	return detailed
}

func (o *organizer) Organize(ctx context.Context, req *Request) (*Result, error) {
	if req == nil {
		return nil, ErrNilRequest
	}

	filename := filepath.Base(req.SourcePath)
	parsed, err := o.ParseFilename(filename)
	if err != nil {
		return nil, fmt.Errorf("parse filename: %w", err)
	}
	o.applyDirectoryHints(req.SourcePath, parsed)
	var parsedForFallback *ParsedMedia
	if parsed != nil {
		cp := *parsed
		parsedForFallback = &cp
	}

	result := &Result{
		Original: parsed,
	}

	var match *TMDBMatchResult
	if req.TMDBID != nil {
		mediaType := "movie"
		if req.MediaType != nil {
			mediaType = *req.MediaType
		}
		match, err = o.matcher.getByID(ctx, *req.TMDBID, mediaType)
		if err != nil {
			return nil, fmt.Errorf("get tmdb by id: %w", err)
		}
	} else {
		if tmdbID := extractTMDBIDFromPath(req.SourcePath); tmdbID > 0 {
			mediaType := "movie"
			if !parsed.IsMovie {
				mediaType = "tv"
			}
			match, err = o.matcher.getByID(ctx, tmdbID, mediaType)
			if err != nil {
				// 路径里显式 tmdbId 是强提示，但线上网络偶发超时时，不应直接整条失败；
				// 允许回退到标题搜索，尽量完成整理。
				if parsed != nil && parsed.Title != "" && isRetryableTMDBError(err) {
					match, err = o.MatchTMDB(ctx, parsed)
					if err != nil {
						return nil, fmt.Errorf("fallback match tmdb after id lookup failed: %w", err)
					}
				} else {
					return nil, fmt.Errorf("get tmdb by id from path: %w", err)
				}
			}
		} else {
			preferredMediaType := "movie"
			if parsed != nil && !parsed.IsMovie {
				preferredMediaType = "tv"
			}
			match, err = o.matchOrganizeTMDBWithFilenameFallback(ctx, req.SourcePath, parsedForFallback, preferredMediaType)
			if err != nil {
				return nil, fmt.Errorf("match tmdb: %w", err)
			}
		}
	}
	result.TMDBMatch = match

	if match == nil || !match.Matched {
		result.Execution = &ExecutionResult{
			OldPath: req.SourcePath,
			Error:   "TMDB match failed: no matching result found",
		}
		return result, nil
	}

	tmpl := req.NamingTemplate
	if tmpl == nil {
		tmpl = o.config.NamingTemplate
	}

	orgPath, category, err := o.GeneratePath(match, parsed, tmpl)
	if err != nil {
		return nil, fmt.Errorf("generate path: %w", err)
	}
	result.Category = category

	orgPath.FullPath = filepath.Join(req.TargetDir, orgPath.FullPath)
	orgPath.DirPath = filepath.Join(req.TargetDir, orgPath.DirPath)
	if orgPath.STRMPath != "" {
		orgPath.STRMPath = filepath.Join(req.TargetDir, orgPath.STRMPath)
	}
	result.NewPath = orgPath

	if req.GenerateSTRM {
		strmPath := strings.TrimSuffix(orgPath.FullPath, filepath.Ext(orgPath.FullPath)) + ".strm"
		orgPath.STRMPath = strmPath
	}

	if req.Mode == ModeExecute {
		exec := o.executeOrganize(req, orgPath)
		result.Execution = exec

		if exec.Success && req.ScrapeMetadata && match != nil && match.Matched {
			mediaType := "movie"
			if match.MediaType == "tv" {
				mediaType = "tv"
			}
			epBase := strings.TrimSuffix(filepath.Base(orgPath.FullPath), filepath.Ext(orgPath.FullPath))
			if err := o.Scrape(ctx, orgPath.DirPath, match.TMDBID, mediaType, parsed.Season, parsed.Episode, epBase); err != nil {
				exec.Error = fmt.Sprintf("scrape: %v", err)
			}
		}
	}

	return result, nil
}

func (o *organizer) OrganizeBatch(ctx context.Context, reqs []*Request) ([]*Result, error) {
	results := make([]*Result, len(reqs))
	for i, req := range reqs {
		result, err := o.Organize(ctx, req)
		if err != nil {
			results[i] = &Result{
				Execution: &ExecutionResult{
					Success: false,
					Error:   err.Error(),
				},
			}
			continue
		}
		results[i] = result
	}
	return results, nil
}

func (o *organizer) WatchDirectory(ctx context.Context, opts WatchOptions) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if _, exists := o.watchers[opts.Directory]; exists {
		return ErrWatchExists
	}

	w, err := newFSWatcher(o, opts)
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}

	o.watchers[opts.Directory] = w

	go func() {
		<-ctx.Done()
		o.mu.Lock()
		delete(o.watchers, opts.Directory)
		o.mu.Unlock()
		w.stop()
	}()

	return w.start()
}

func (o *organizer) StopWatch(directory string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	w, exists := o.watchers[directory]
	if !exists {
		return ErrWatchNotFound
	}

	delete(o.watchers, directory)
	return w.stop()
}

func (o *organizer) ReloadCategory(data []byte) error {
	classifier, err := NewCategoryClassifierFromBytes(data)
	if err != nil {
		return fmt.Errorf("reload category: %w", err)
	}
	o.mu.Lock()
	o.classifier = classifier
	o.mu.Unlock()
	return nil
}

func (o *organizer) ReloadCategoryFromFile(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read category config: %w", err)
	}
	return o.ReloadCategory(data)
}

func (o *organizer) Close() {
	o.mu.Lock()
	defer o.mu.Unlock()
	for dir, w := range o.watchers {
		w.stop()
		delete(o.watchers, dir)
	}
	if o.matcher != nil {
		o.matcher.Close()
	}
	if o.scraper != nil {
		o.scraper.Close()
	}
}

func (o *organizer) Plan(ctx context.Context, req *PlanRequest) (*PlanResult, error) {
	if len(req.Files) == 0 {
		return nil, ErrNoFilesProvided
	}

	resourceName := req.ResourceName
	if resourceName == "" {
		resourceName = inferResourceName(req.Files)
	}

	parsed, parseErr, parsedFiles := o.parsePlanInputs(resourceName, req.Files)
	match, err := o.matchPlanTMDB(ctx, req, resourceName, parsed, parseErr, parsedFiles)
	if err != nil {
		return nil, fmt.Errorf("match tmdb: %w", err)
	}
	match = o.preferDetailedMatch(ctx, match)
	items := o.buildPlanItems(req, match, parsedFiles)

	conflictCount := MarkConflicts(items)

	if req.Mode == ModeExecute {
		for i := range items {
			if items[i].Conflict || items[i].TargetPath == "" {
				continue
			}
			exec := o.executePlanItem(ctx, &items[i], req)
			items[i].Execution = exec
		}
		o.scrapePlanItems(ctx, items, req)
	}

	errMsg := ""
	if conflictCount > 0 {
		errMsg = fmt.Sprintf("存在目标路径冲突: %d 个文件", conflictCount)
	}

	return &PlanResult{Success: true, Error: errMsg, Items: items}, nil
}

func (o *organizer) parsePlanInputs(resourceName string, files []FileInput) (*ParsedMedia, error, []*ParsedMedia) {
	parsed, parseErr := ParseFilename(resourceName)
	parsedFiles := make([]*ParsedMedia, 0, len(files))
	for _, f := range files {
		fp, fpErr := ParseFilename(f.Name)
		if fpErr != nil || fp == nil {
			parsedFiles = append(parsedFiles, nil)
			continue
		}
		if strings.TrimSpace(f.Path) != "" {
			o.applyDirectoryHints(f.Path, fp)
		}
		parsedFiles = append(parsedFiles, fp)
	}
	return parsed, parseErr, parsedFiles
}

func (o *organizer) matchPlanTMDB(ctx context.Context, req *PlanRequest, resourceName string, parsed *ParsedMedia, parseErr error, parsedFiles []*ParsedMedia) (*TMDBMatchResult, error) {
	if req.TMDBMatch != nil && req.TMDBMatch.Matched {
		return req.TMDBMatch, nil
	}
	if req.TMDBID != nil {
		mediaType := "movie"
		if req.MediaType != nil {
			mediaType = *req.MediaType
		}
		return o.matcher.getByID(ctx, *req.TMDBID, mediaType)
	}
	if tmdbID := extractTMDBIDFromPlan(resourceName, req.Files); tmdbID > 0 {
		match, err := o.matcher.getByID(ctx, tmdbID, inferPlanMediaType(parsed, parsedFiles))
		if err != nil && parsed != nil && parsed.Title != "" && isRetryableTMDBError(err) {
			match, err = o.MatchTMDB(ctx, parsed)
		}
		return match, err
	}
	preferredMediaType := inferPlanMediaType(parsed, parsedFiles)
	return o.matchPlanTMDBWithFilenameFallback(ctx, parsed, parseErr, req.Files, parsedFiles, preferredMediaType)
}

func (o *organizer) buildPlanItems(req *PlanRequest, match *TMDBMatchResult, parsedFiles []*ParsedMedia) []PlanItem {
	items := make([]PlanItem, 0, len(req.Files))
	for idx, f := range req.Files {
		items = append(items, o.buildPlanItem(req, match, f, parsedFiles[idx], idx))
	}
	return items
}

func (o *organizer) buildPlanItem(req *PlanRequest, match *TMDBMatchResult, f FileInput, fp *ParsedMedia, idx int) PlanItem {
	mediaType := "movie"
	if match != nil && match.MediaType == "tv" {
		mediaType = "tv"
	}
	if mediaType == "tv" && fp != nil {
		fp.IsMovie = false
		if fp.Episode == 0 {
			fp.Episode = idx + 1
		}
		if fp.Season == 0 {
			fp.Season = 1
		}
	}

	category := ""
	if o.classifier != nil && match != nil && match.Matched {
		category = o.classifier.Classify(mediaType, match)
	}

	item := PlanItem{
		FileInput: f,
		Parsed:    fp,
		TMDBMatch: match,
		Category:  category,
	}
	orgPath, genErr := GeneratePathWithCategory(match, fp, req.NamingTemplate, category)
	if genErr != nil {
		item.Conflict = true
		item.ConflictMsg = fmt.Sprintf("路径生成失败: %v", genErr)
		return item
	}
	if orgPath != nil {
		item.TargetPath = filepath.Join(req.TargetDir, orgPath.FullPath)
	}
	return item
}

func (o *organizer) matchOrganizeTMDBWithFilenameFallback(ctx context.Context, sourcePath string, parsedFromFilename *ParsedMedia, preferredMediaType string) (*TMDBMatchResult, error) {
	var primary *TMDBMatchResult

	// 首选资源目录名进行匹配（与 Plan 的 resourceName 优先策略保持一致）。
	dirTitle := extractParentDirTitle(sourcePath)
	if dirParsed, parseErr := ParseFilename(dirTitle); parseErr == nil && dirParsed != nil && strings.TrimSpace(dirParsed.Title) != "" {
		match, err := o.MatchTMDB(ctx, dirParsed)
		if err != nil {
			return nil, err
		}
		if match != nil && match.Matched {
			// 当文件侧明显是剧集时，不接受目录名先命中的 movie，继续走文件名 TV 匹配。
			if !(preferredMediaType == "tv" && match.MediaType == "movie") {
				return match, nil
			}
			primary = match
		}
	}

	// 目录名未命中，或命中类型与文件侧推断冲突时，降级到文件名提取标题再匹配。
	if parsedFromFilename == nil || strings.TrimSpace(parsedFromFilename.Title) == "" {
		return primary, nil
	}
	fallback := *parsedFromFilename
	if preferredMediaType == "tv" {
		fallback.IsMovie = false
	}

	fallbackMatch, err := o.MatchTMDB(ctx, &fallback)
	if err != nil {
		// 如果主匹配已有结果，回退匹配失败不阻断整理流程。
		if primary != nil && primary.Matched {
			return primary, nil
		}
		return nil, err
	}
	if fallbackMatch != nil && fallbackMatch.Matched {
		return fallbackMatch, nil
	}
	return primary, nil
}

func (o *organizer) matchPlanTMDBWithFilenameFallback(ctx context.Context, parsed *ParsedMedia, parseErr error, files []FileInput, parsedFiles []*ParsedMedia, preferredMediaType string) (*TMDBMatchResult, error) {
	var primary *TMDBMatchResult

	// 首选 resourceName（通常来自目录标题）进行匹配。
	if parseErr == nil && parsed != nil && strings.TrimSpace(parsed.Title) != "" {
		match, err := o.MatchTMDB(ctx, parsed)
		if err != nil {
			return nil, err
		}
		if match != nil && match.Matched {
			// 当文件侧明显是剧集时，不接受目录名先命中的 movie，继续走文件侧 TV 匹配。
			if !(preferredMediaType == "tv" && match.MediaType == "movie") {
				return match, nil
			}
			primary = match
		}
	}

	// resourceName 未命中，或命中类型与文件侧推断冲突时，降级到文件名提取标题再匹配。
	fallback := inferPlanParsedFromFiles(files, parsedFiles)
	if fallback == nil || strings.TrimSpace(fallback.Title) == "" {
		return primary, nil
	}
	if preferredMediaType == "tv" {
		fallback.IsMovie = false
	}

	fallbackMatch, err := o.MatchTMDB(ctx, fallback)
	if err != nil {
		// 如果主匹配已有结果，回退匹配失败不阻断计划生成。
		if primary != nil && primary.Matched {
			return primary, nil
		}
		return nil, err
	}
	if fallbackMatch != nil && fallbackMatch.Matched {
		return fallbackMatch, nil
	}
	return primary, nil
}

func (o *organizer) Scrape(ctx context.Context, targetPath string, tmdbID int, mediaType string, season, episode int, episodeFileName string) error {
	if o.scraper == nil {
		return ErrScrapeNotConfigured
	}
	switch mediaType {
	case "tv":
		return o.scraper.ScrapeTV(ctx, targetPath, tmdbID, season, episode, episodeFileName)
	default:
		return o.scraper.ScrapeMovie(ctx, targetPath, tmdbID)
	}
}

func (o *organizer) executeOrganize(req *Request, orgPath *OrganizedPath) *ExecutionResult {
	exec := &ExecutionResult{
		OldPath: req.SourcePath,
		NewPath: orgPath.FullPath,
	}

	if err := os.MkdirAll(orgPath.DirPath, 0o755); err != nil {
		exec.Error = fmt.Sprintf("create directory: %v", err)
		return exec
	}

	if !req.Overwrite {
		if _, err := os.Stat(orgPath.FullPath); err == nil {
			exec.Error = "target file already exists"
			return exec
		}
	}

	fileOp := o.resolveFileOperationMode(req.FileOperation)
	if err := performFileOperation(req.SourcePath, orgPath.FullPath, fileOp, req.Overwrite); err != nil {
		exec.Error = fmt.Sprintf("file operation (%s): %v", fileOp, err)
		return exec
	}

	exec.Success = true

	if req.GenerateSTRM && orgPath.STRMPath != "" {
		if err := generateSTRMFile(orgPath.STRMPath, req.SourcePath, req.STRMURLPrefix); err != nil {
			exec.Error = fmt.Sprintf("generate strm: %v", err)
		} else {
			exec.STRMGenerated = true
			exec.STRMPath = orgPath.STRMPath
		}
	}

	return exec
}

// resolveFileOperationMode 解析文件操作模式（优先使用请求级配置，否则使用全局配置）
func (o *organizer) resolveFileOperationMode(reqMode FileOperationMode) FileOperationMode {
	if reqMode != FileOpMove {
		return reqMode
	}
	return o.config.FileOperation
}

func generateSTRMFile(strmPath, sourcePath, urlPrefix string) error {
	content := sourcePath
	if urlPrefix != "" {
		content = urlPrefix + "/" + filepath.Base(sourcePath)
	}

	if err := os.MkdirAll(filepath.Dir(strmPath), 0o755); err != nil {
		return err
	}

	return os.WriteFile(strmPath, []byte(content), 0o644)
}

func inferResourceName(files []FileInput) string {
	if len(files) == 0 {
		return ""
	}
	if len(files) == 1 {
		return files[0].Name
	}

	titles := make([]string, 0, len(files))
	for _, f := range files {
		p, err := ParseFilename(f.Name)
		if err != nil || p.Title == "" {
			titles = append(titles, files[0].Name)
			continue
		}
		titles = append(titles, p.Title)
	}

	common := longestCommonPrefix(titles)
	if len(strings.TrimSpace(common)) >= 2 {
		return strings.TrimSpace(common)
	}

	for _, f := range files {
		if strings.TrimSpace(f.Path) != "" {
			dir := filepath.Dir(f.Path)
			if dir != "." && dir != "" {
				return filepath.Base(dir)
			}
		}
	}

	return files[0].Name
}

func inferPlanParsedFromFiles(files []FileInput, parsedFiles []*ParsedMedia) *ParsedMedia {
	if len(parsedFiles) > 0 && parsedFiles[0] != nil && strings.TrimSpace(parsedFiles[0].Title) != "" {
		cp := *parsedFiles[0]
		return &cp
	}
	if len(files) == 0 {
		return nil
	}
	parsed, err := ParseFilename(files[0].Name)
	if err == nil && parsed != nil && strings.TrimSpace(parsed.Title) != "" {
		return parsed
	}
	return nil
}

func longestCommonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	prefix := strs[0]
	for _, s := range strs[1:] {
		for !strings.HasPrefix(strings.ToLower(s), strings.ToLower(prefix)) {
			if len(prefix) == 0 {
				return ""
			}
			prefix = prefix[:len(prefix)-1]
		}
	}
	return strings.TrimSpace(prefix)
}

func (o *organizer) executePlanItem(ctx context.Context, item *PlanItem, req *PlanRequest) *ExecutionResult {
	exec := &ExecutionResult{
		OldPath: item.FileInput.Path,
		NewPath: item.TargetPath,
	}

	if item.TargetPath == "" {
		exec.Error = "target path is empty"
		return exec
	}

	fileOp := o.resolvePlanFileOperationMode(req.FileOperation)
	if err := performFileOperation(item.FileInput.Path, item.TargetPath, fileOp, req.Overwrite); err != nil {
		exec.Error = fmt.Sprintf("file operation (%s): %v", fileOp, err)
		return exec
	}

	exec.Success = true
	return exec
}

func (o *organizer) scrapePlanItems(ctx context.Context, items []PlanItem, req *PlanRequest) {
	if !req.Scrape || o.scraper == nil {
		return
	}

	tvShows := make(map[string][]int)
	for i := range items {
		item := &items[i]
		if item.Execution == nil || !item.Execution.Success || item.TMDBMatch == nil || !item.TMDBMatch.Matched || item.TargetPath == "" {
			continue
		}
		mediaType := item.TMDBMatch.MediaType
		if mediaType != "tv" {
			if err := o.Scrape(ctx, filepath.Dir(item.TargetPath), item.TMDBMatch.TMDBID, "movie", 0, 0, ""); err != nil {
				appendExecutionError(item.Execution, fmt.Sprintf("scrape: %v", err))
			}
			continue
		}

		seasonDir := filepath.Dir(item.TargetPath)
		showDir := filepath.Dir(seasonDir)
		key := fmt.Sprintf("%d|%s", item.TMDBMatch.TMDBID, showDir)
		tvShows[key] = append(tvShows[key], i)
	}

	for _, indexes := range tvShows {
		if len(indexes) == 0 {
			continue
		}
		first := &items[indexes[0]]
		showDir := filepath.Dir(filepath.Dir(first.TargetPath))
		if err := o.scraper.ScrapeTVShow(ctx, showDir, first.TMDBMatch.TMDBID); err != nil {
			for _, idx := range indexes {
				appendExecutionError(items[idx].Execution, fmt.Sprintf("scrape show: %v", err))
			}
			continue
		}

		for _, idx := range indexes {
			item := &items[idx]
			var season, episode int
			if item.Parsed != nil {
				season = item.Parsed.Season
				episode = item.Parsed.Episode
			}
			epBase := strings.TrimSuffix(filepath.Base(item.TargetPath), filepath.Ext(item.TargetPath))
			seasonDir := filepath.Dir(item.TargetPath)
			if err := o.scraper.ScrapeTVEpisode(ctx, seasonDir, item.TMDBMatch.TMDBID, season, episode, epBase); err != nil {
				appendExecutionError(item.Execution, fmt.Sprintf("scrape episode: %v", err))
			}
		}
	}
}

func appendExecutionError(exec *ExecutionResult, msg string) {
	if exec == nil || msg == "" {
		return
	}
	if exec.Error == "" {
		exec.Error = msg
		return
	}
	exec.Error += "; " + msg
}

// resolvePlanFileOperationMode 解析 Plan 请求的文件操作模式
func (o *organizer) resolvePlanFileOperationMode(reqMode FileOperationMode) FileOperationMode {
	if reqMode != FileOpMove {
		return reqMode
	}
	return o.config.FileOperation
}

func extractTMDBIDFromPlan(resourceName string, files []FileInput) int {
	if id := extractTMDBIDFromText(resourceName); id > 0 {
		return id
	}
	for _, f := range files {
		if id := extractTMDBIDFromText(f.Name); id > 0 {
			return id
		}
		if id := extractTMDBIDFromPath(f.Path); id > 0 {
			return id
		}
	}
	return 0
}

func inferPlanMediaType(parsed *ParsedMedia, parsedFiles []*ParsedMedia) string {
	if parsed != nil && !parsed.IsMovie {
		return "tv"
	}
	for _, fp := range parsedFiles {
		if fp != nil && !fp.IsMovie {
			return "tv"
		}
	}
	return "movie"
}

func extractTMDBIDFromText(text string) int {
	re := regexp.MustCompile(`(?i)[\{\[]?tmdb(?:id)?[-_ .]?(\d+)[\}\]]?`)
	if m := re.FindStringSubmatch(text); len(m) > 1 {
		id, _ := strconv.Atoi(m[1])
		return id
	}
	return 0
}

func extractTMDBIDFromPath(sourcePath string) int {
	if id := extractTMDBIDFromText(filepath.Base(sourcePath)); id > 0 {
		return id
	}
	// walk from file dir upward: fileDir → parentDir → ...
	dir := filepath.Dir(sourcePath)
	for i := 0; i < 6; i++ {
		if id := extractTMDBIDFromText(filepath.Base(dir)); id > 0 {
			return id
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return 0
}

func extractParentDirTitle(sourcePath string) string {
	resourceDir := extractResourceDir(sourcePath, "")
	if resourceDir == "" {
		return ""
	}
	return filepath.Base(resourceDir)
}

func extractResourceDir(sourcePath, stopDir string) string {
	dir := filepath.Dir(sourcePath)
	var stopAbs string
	if stopDir != "" {
		if absStop, err := filepath.Abs(stopDir); err == nil {
			stopAbs = filepath.Clean(absStop)
		} else {
			stopAbs = filepath.Clean(stopDir)
		}
	}

	var candidates []string
	for i := 0; i < 8; i++ {
		current := filepath.Clean(dir)
		if stopAbs != "" {
			if absCurrent, err := filepath.Abs(current); err == nil {
				current = filepath.Clean(absCurrent)
			}
			if current == stopAbs {
				break
			}
		}

		dirName := filepath.Base(dir)
		parsed, err := ParseFilename(dirName)
		if err == nil && parsed != nil && parsed.Title != "" && !isSeasonDirectoryName(dirName, parsed.Title) {
			candidates = append(candidates, dir)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if len(candidates) == 0 {
		return ""
	}
	// 资源目录取离文件最近的非季目录。这样对“监控根/批次目录/片名/Season/文件”有效，
	// 不需要硬编码跳过“临时目录”等环境相关名称。
	return candidates[0]
}

func isSeasonDirectoryName(name, parsedTitle string) bool {
	lowerTitle := strings.ToLower(strings.TrimSpace(parsedTitle))
	if lowerTitle == "season" {
		return true
	}
	seasonDirPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)^season[\s._-]*\d{1,4}$`),
		regexp.MustCompile(`(?i)^s\d{1,4}$`),
		regexp.MustCompile(`^第\s*\d{1,4}\s*季$`),
	}
	for _, re := range seasonDirPatterns {
		if re.MatchString(strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

func directoryTitleLooksNoisy(title string) bool {
	t := strings.TrimSpace(title)
	if t == "" {
		return false
	}
	return reDirectoryNoise.MatchString(t) || reDirectoryCnNoise.MatchString(t)
}

func mostlyASCIIAlphaNumSpace(s string) bool {
	if s == "" {
		return false
	}
	total := 0
	ascii := 0
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			total++
			if r <= unicode.MaxASCII {
				ascii++
			}
		}
	}
	if total == 0 {
		return false
	}
	return ascii*100/total >= 75
}

func shouldKeepFilenameTitle(filenameTitle, parentTitle, parentRawHint string) bool {
	fileKey := mediaTitleKey(filenameTitle)
	parentKey := mediaTitleKey(parentTitle)
	if fileKey == "" || parentKey == "" {
		return false
	}
	if fileKey == parentKey {
		return true
	}

	// 目录含明显发行噪声且文件名标题更干净时，优先保留文件名标题。
	if directoryTitleLooksNoisy(parentRawHint) || directoryTitleLooksNoisy(parentTitle) {
		if mostlyASCIIAlphaNumSpace(filenameTitle) || containsHan(filenameTitle) {
			return true
		}
	}

	// 发布目录常见形态：标准标题 + 集数/音轨/字幕/大小等标签。
	// 文件名标题是父目录标题的核心前缀/子串时，保留文件名标题，只从目录补年份。
	return len(fileKey) >= 5 && len(parentKey) > len(fileKey) && strings.Contains(parentKey, fileKey)
}

func mediaTitleKey(s string) string {
	s = strings.NewReplacer(
		"’", "'",
		"‘", "'",
		"＇", "'",
		"`", "'",
		"：", ":",
	).Replace(strings.ToLower(strings.TrimSpace(s)))

	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func containsHan(s string) bool {
	for _, r := range s {
		if r >= '一' && r <= '鿿' {
			return true
		}
	}
	return false
}
