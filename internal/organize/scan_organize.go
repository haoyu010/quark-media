//go:build ignore
// +build ignore

package organize

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ScanOrganizeOptions BatchScanOrganize 的输入参数
type ScanOrganizeOptions struct {
	JobID          string            `json:"jobId,omitempty"`
	Directory      string            `json:"directory"`
	TargetDir      string            `json:"targetDir"`
	Recursive      bool              `json:"recursive"`
	SkipPaths      []string          `json:"skipPaths,omitempty"`
	FileOperation  FileOperationMode `json:"fileOperation,omitempty"`
	SyncDelete     *bool             `json:"syncDelete,omitempty"`
	EnableCategory *bool             `json:"enableCategory,omitempty"`
	EnableScrape   *bool             `json:"enableScrape,omitempty"`
	Overwrite      bool              `json:"overwrite,omitempty"`
	NamingTemplate *NamingTemplate   `json:"namingTemplate,omitempty"`
	CallbackURL    string            `json:"callbackUrl,omitempty"`
	ProgressURL    string            `json:"progressUrl,omitempty"`
	Parallelism    int               `json:"parallelism,omitempty"`
}

// ScanProgress 进度事件 payload
type ScanProgress struct {
	JobID   string `json:"jobId"`
	Seq     int64  `json:"seq,omitempty"`
	Total   int    `json:"total"`
	Done    int    `json:"done"`
	Success int    `json:"success"`
	Failed  int    `json:"failed"`
	Skipped int    `json:"skipped,omitempty"`
	Current string `json:"current,omitempty"`
	Phase   string `json:"phase"`
	Error   string `json:"error,omitempty"`
}

// BatchScanOrganize 异步批量扫描 + 整理
// 与 watcher.processFile 共享 Organize + applyDirectoryHints + 局部 cache 模型
func (o *organizer) BatchScanOrganize(ctx context.Context, jobID string, opts ScanOrganizeOptions) error {
	if opts.Directory == "" {
		return fmt.Errorf("directory is required")
	}
	if opts.TargetDir == "" {
		return fmt.Errorf("targetDir is required")
	}
	if opts.Parallelism <= 0 {
		opts.Parallelism = 5
	}
	log.Printf("[scan-plan] job %s start: directory=%s target=%s recursive=%t parallelism=%d overwrite=%t",
		jobID, opts.Directory, opts.TargetDir, opts.Recursive, opts.Parallelism, opts.Overwrite)

	skipSet := make(map[string]struct{}, len(opts.SkipPaths))
	for _, p := range opts.SkipPaths {
		skipSet[filepath.Clean(p)] = struct{}{}
	}

	o.publishProgress(opts.ProgressURL, ScanProgress{JobID: jobID, Seq: 1, Phase: "scanning"})

	files, err := o.collectMediaFiles(opts.Directory, opts.TargetDir, opts.Recursive, skipSet)
	if err != nil {
		o.publishProgress(opts.ProgressURL, ScanProgress{
			JobID: jobID, Phase: "done", Error: err.Error(),
		})
		return err
	}

	total := len(files)
	log.Printf("[scan-plan] job %s collected %d media files", jobID, total)
	progress := &batchProgress{
		jobID: jobID,
		seq:   1,
		total: int32(total),
		emit:  o.publishProgress,
	}
	progress.emitState(opts.ProgressURL, "organizing")

	if total == 0 {
		progress.emitState(opts.ProgressURL, "done")
		return nil
	}

	cache := newBatchCache()
	sem := make(chan struct{}, opts.Parallelism)
	var wg sync.WaitGroup
	groups := GroupFiles(opts.Directory, fileInputsFromPaths(files))
	for _, group := range groups {
		group := group
		select {
		case <-ctx.Done():
			progress.emitState(opts.ProgressURL, "done")
			return ctx.Err()
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			o.processBatchGroup(ctx, group, opts, cache, progress)
		}()
	}
	wg.Wait()
	progress.emitState(opts.ProgressURL, "done")
	log.Printf("[scan-plan] job %s done: total=%d success=%d failed=%d skipped=%d",
		jobID,
		total,
		atomic.LoadInt32(&progress.success),
		atomic.LoadInt32(&progress.failed),
		atomic.LoadInt32(&progress.skipped),
	)
	return nil
}

func fileInputsFromPaths(paths []string) []FileInput {
	files := make([]FileInput, 0, len(paths))
	for _, path := range paths {
		files = append(files, FileInput{
			ID:   path,
			Name: filepath.Base(path),
			Path: path,
		})
	}
	return files
}

func (o *organizer) processBatchGroup(ctx context.Context, group ScanGroup, opts ScanOrganizeOptions, cache *batchCache, progress *batchProgress) {
	for _, file := range group.Files {
		select {
		case <-ctx.Done():
			progress.recordDone(opts.ProgressURL, file.Name, false)
			return
		default:
			o.processBatchFile(ctx, file.Path, opts, cache, progress)
		}
	}
}

// processBatchFile 处理单个文件 - 基本复用 watcher.processFile 路径
func (o *organizer) processBatchFile(ctx context.Context, path string, opts ScanOrganizeOptions, cache *batchCache, progress *batchProgress) {
	filename := filepath.Base(path)

	if isSubtitleFile(filename, o.config.MediaExtensions) {
		progress.recordDone(opts.ProgressURL, filename, true)
		return
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Printf("[scan-plan] file not found: %s", path)
		progress.recordDone(opts.ProgressURL, filename, false)
		return
	}

	log.Printf("[scan-plan] processing file: %s", path)
	progress.setCurrent(opts.ProgressURL, filename)
	fileCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	req := &Request{
		SourcePath:     path,
		TargetDir:      opts.TargetDir,
		Mode:           ModeExecute,
		FileOperation:  opts.FileOperation,
		SyncDelete:     opts.SyncDelete,
		Overwrite:      opts.Overwrite,
		NamingTemplate: cloneNamingTemplate(opts.NamingTemplate),
	}
	if req.NamingTemplate != nil && opts.EnableCategory != nil {
		req.NamingTemplate.EnableCategory = *opts.EnableCategory
	}
	if opts.EnableScrape != nil && *opts.EnableScrape {
		req.ScrapeMetadata = true
	}

	parsed, err := o.ParseFilename(filename)
	if err == nil && parsed != nil {
		o.applyDirectoryHints(path, parsed)
		if cached := cache.lookupResource(path, opts.Directory, parsed); cached != nil {
			tmdbID := cached.TMDBID
			mediaType := cached.MediaType
			req.TMDBID = &tmdbID
			req.MediaType = &mediaType
		} else if parsed.Title != "" {
			if cached := cache.lookupTitle(parsed.Title); cached != nil {
				tmdbID := cached.TMDBID
				mediaType := cached.MediaType
				req.TMDBID = &tmdbID
				req.MediaType = &mediaType
			}
		}
	}

	result, err := o.Organize(fileCtx, req)
	if err != nil {
		log.Printf("[scan-plan] organize error: %s -> %v", path, err)
		progress.recordDone(opts.ProgressURL, filename, false)
		o.sendBatchCallback(opts.CallbackURL, progress.jobID, nil, req, err.Error())
		return
	}

	success := false
	if result != nil && result.Execution != nil {
		success = result.Execution.Success
	}
	if isNonOverwriteTargetExists(result) {
		log.Printf("[scan-plan] target exists, skip callback: %s", path)
		progress.recordSkipped(opts.ProgressURL, filename)
		return
	}

	if result != nil {
		if match := result.TMDBMatch; match != nil && match.Matched && match.TMDBID > 0 {
			cache.store(path, opts.Directory, match)
		}
	}

	progress.recordDone(opts.ProgressURL, filename, success)
	if success && result != nil && result.NewPath != nil {
		log.Printf("[scan-plan] organize success: %s -> %s", path, result.NewPath.FullPath)
	} else if result != nil && result.Execution != nil && result.Execution.Error != "" {
		log.Printf("[scan-plan] organize skipped: %s -> %s", path, result.Execution.Error)
	}
	o.sendBatchCallback(opts.CallbackURL, progress.jobID, result, req, "")
}

func isNonOverwriteTargetExists(result *Result) bool {
	if result == nil || result.Execution == nil || result.Execution.Success {
		return false
	}
	errText := result.Execution.Error
	return strings.Contains(errText, "target file already exists") ||
		strings.Contains(errText, "目标文件已存在")
}

// collectMediaFiles 递归扫描媒体文件，跳过 SkipPaths/targetDir
func (o *organizer) collectMediaFiles(root, targetDir string, recursive bool, skipSet map[string]struct{}) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if !recursive && path != root {
				return filepath.SkipDir
			}
			if ShouldSkipDir(path, root, targetDir) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isMediaFile(info.Name(), o.config.MediaExtensions) {
			return nil
		}
		clean := filepath.Clean(path)
		if _, skip := skipSet[clean]; skip {
			return nil
		}
		files = append(files, clean)
		return nil
	})
	return files, err
}

// batchCache 模拟 fsWatcher 的 resourceCache + matchCache
type batchCache struct {
	mu              sync.RWMutex
	matchByID       map[int]*TMDBMatchResult
	matchByResource map[string]*TMDBMatchResult
}

func newBatchCache() *batchCache {
	return &batchCache{
		matchByID:       make(map[int]*TMDBMatchResult),
		matchByResource: make(map[string]*TMDBMatchResult),
	}
}

func (c *batchCache) lookupResource(path, watchDir string, parsed *ParsedMedia) *TMDBMatchResult {
	key := batchResourceKey(path, watchDir)
	if key == "" {
		return nil
	}
	c.mu.RLock()
	cached := c.matchByResource[key]
	c.mu.RUnlock()
	if cached == nil || !cached.Matched || cached.TMDBID <= 0 {
		return nil
	}
	if parsed != nil {
		expected := "movie"
		if !parsed.IsMovie {
			expected = "tv"
		}
		if cached.MediaType != "" && cached.MediaType != expected {
			return nil
		}
	}
	return cached
}

func (c *batchCache) lookupTitle(title string) *TMDBMatchResult {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, cached := range c.matchByID {
		if cached.Matched && titlesMatch(title, cached.Title, cached.OriginalTitle) {
			return cached
		}
	}
	return nil
}

func (c *batchCache) store(path, watchDir string, match *TMDBMatchResult) {
	if match == nil || !match.Matched || match.TMDBID <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.matchByID[match.TMDBID] = match
	if key := batchResourceKey(path, watchDir); key != "" {
		c.matchByResource[key] = match
	}
}

func batchResourceKey(path, watchDir string) string {
	resourceDir := extractResourceDir(path, watchDir)
	if resourceDir == "" {
		return ""
	}
	if abs, err := filepath.Abs(resourceDir); err == nil {
		resourceDir = abs
	}
	return filepath.Clean(resourceDir)
}

// batchProgress 进度跟踪
type batchProgress struct {
	jobID     string
	seq       int64
	total     int32
	done      int32
	success   int32
	failed    int32
	skipped   int32
	currentMu sync.Mutex
	current   string
	emit      func(string, ScanProgress)
}

func (p *batchProgress) setCurrent(url, name string) {
	p.currentMu.Lock()
	p.current = name
	p.currentMu.Unlock()
	p.emit(url, p.snapshot("organizing"))
}

func (p *batchProgress) recordDone(url, _ string, ok bool) {
	atomic.AddInt32(&p.done, 1)
	if ok {
		atomic.AddInt32(&p.success, 1)
	} else {
		atomic.AddInt32(&p.failed, 1)
	}
	p.emit(url, p.snapshot("organizing"))
}

func (p *batchProgress) recordSkipped(url, _ string) {
	atomic.AddInt32(&p.done, 1)
	atomic.AddInt32(&p.skipped, 1)
	p.emit(url, p.snapshot("organizing"))
}

func (p *batchProgress) emitState(url, phase string) {
	p.emit(url, p.snapshot(phase))
}

func (p *batchProgress) snapshot(phase string) ScanProgress {
	p.currentMu.Lock()
	current := p.current
	p.currentMu.Unlock()
	return ScanProgress{
		JobID:   p.jobID,
		Seq:     atomic.AddInt64(&p.seq, 1),
		Total:   int(atomic.LoadInt32(&p.total)),
		Done:    int(atomic.LoadInt32(&p.done)),
		Success: int(atomic.LoadInt32(&p.success)),
		Failed:  int(atomic.LoadInt32(&p.failed)),
		Skipped: int(atomic.LoadInt32(&p.skipped)),
		Current: current,
		Phase:   phase,
	}
}

// publishProgress fire-and-forget HTTP POST，不阻塞 worker
func (o *organizer) publishProgress(url string, p ScanProgress) {
	if url == "" {
		return
	}
	go func() {
		body, _ := json.Marshal(p)
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		resp.Body.Close()
	}()
}

// sendBatchCallback 单条结果回调，与 watcher.sendCallback 同 payload schema
func (o *organizer) sendBatchCallback(url, jobID string, result *Result, req *Request, fallbackErr string) {
	if url == "" {
		return
	}

	var exec *ExecutionResult
	var match *TMDBMatchResult
	var parsed *ParsedMedia
	if result != nil {
		exec = result.Execution
		match = result.TMDBMatch
		parsed = result.Original
	}

	success := false
	if exec != nil {
		success = exec.Success
	}

	sourceName := filepath.Base(req.SourcePath)
	fallbackTitle := strings.TrimSuffix(sourceName, filepath.Ext(sourceName))
	if parsed != nil && parsed.Title != "" {
		fallbackTitle = parsed.Title
	}

	payload := map[string]any{
		"sourcePath":    req.SourcePath,
		"targetPath":    "",
		"sourceName":    sourceName,
		"targetName":    "",
		"success":       success,
		"mediaType":     "",
		"category":      "",
		"title":         fallbackTitle,
		"originalTitle": "",
		"year":          "",
		"tmdbId":        0,
		"season":        0,
		"episode":       0,
		"scraped":       false,
		"strmGenerated": false,
	}

	if result != nil {
		payload["category"] = result.Category
		if result.NewPath != nil {
			payload["targetPath"] = result.NewPath.FullPath
			payload["targetName"] = filepath.Base(result.NewPath.FullPath)
		}
	}
	if match != nil {
		payload["mediaType"] = match.MediaType
		if match.Title != "" {
			payload["title"] = match.Title
		}
		if match.OriginalTitle != "" {
			payload["originalTitle"] = match.OriginalTitle
		}
		payload["tmdbId"] = match.TMDBID
		payload["matchScore"] = match.Confidence * 100
		if match.Year > 0 {
			payload["year"] = fmt.Sprintf("%d", match.Year)
		}
	}
	if parsed != nil {
		payload["season"] = parsed.Season
		payload["episode"] = parsed.Episode
	}
	if exec != nil {
		payload["strmGenerated"] = exec.STRMGenerated
		if !exec.Success && exec.Error != "" {
			payload["error"] = exec.Error
		}
	}
	if !success && fallbackErr != "" {
		payload["error"] = fallbackErr
	}

	body, _ := json.Marshal(map[string]any{
		"results":    []any{payload},
		"sourceType": "scan",
		"jobId":      jobID,
	})
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("[scan-plan] callback failed: %v", err)
		return
	}
	resp.Body.Close()
}
