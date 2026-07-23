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
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	defaultStableWait  = 5 * time.Second
	defaultStableCheck = 1 * time.Second
	mediaExtDefault    = ".mkv,.mp4,.avi,.mov,.wmv,.flv,.m4v,.ts,.rmvb,.rm,.3gp,.mpg,.mpeg,.webm,.strm"
	subtitleExtDefault = ".srt,.ass,.ssa,.sub,.idx"
)

type fsWatcher struct {
	org           *organizer
	opts          WatchOptions
	watcher       *fsnotify.Watcher
	stableMap     map[string]time.Time
	processing    map[string]struct{}
	processed     map[string]time.Time
	matchCache    map[int]*TMDBMatchResult
	resourceCache map[string]*TMDBMatchResult
	mu            sync.Mutex
	done          chan struct{}
	closeOnce     sync.Once
}

func newFSWatcher(org *organizer, opts WatchOptions) (*fsWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if opts.StableWait == 0 {
		opts.StableWait = defaultStableWait
	}
	if opts.StableCheckInterval == 0 {
		opts.StableCheckInterval = defaultStableCheck
	}

	return &fsWatcher{
		org:           org,
		opts:          opts,
		watcher:       watcher,
		stableMap:     make(map[string]time.Time),
		processing:    make(map[string]struct{}),
		processed:     make(map[string]time.Time),
		matchCache:    make(map[int]*TMDBMatchResult),
		resourceCache: make(map[string]*TMDBMatchResult),
		done:          make(chan struct{}),
	}, nil
}

func (w *fsWatcher) start() error {
	if err := w.addWatchPaths(w.opts.Directory); err != nil {
		return err
	}

	go w.processEvents()
	go w.checkStability()

	return nil
}

func (w *fsWatcher) stop() error {
	w.closeOnce.Do(func() {
		close(w.done)
	})
	return w.watcher.Close()
}

func (w *fsWatcher) addWatchPaths(dir string) error {
	if err := w.watcher.Add(dir); err != nil {
		return err
	}

	if !w.opts.Recursive {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			subdir := filepath.Join(dir, entry.Name())
			if err := w.addWatchPaths(subdir); err != nil {
				log.Printf("warn: failed to watch %s: %v", subdir, err)
			}
		}
	}

	return nil
}

func (w *fsWatcher) processEvents() {
	for {
		select {
		case <-w.done:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			if w.opts.OnError != nil {
				w.opts.OnError(err)
			}
		}
	}
}

func (w *fsWatcher) handleEvent(event fsnotify.Event) {
	if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) == 0 {
		return
	}

	filename := filepath.Base(event.Name)
	log.Printf("[watcher] detected event: %s %s", event.Op, filename)

	if event.Op&fsnotify.Create != 0 {
		info, err := os.Stat(event.Name)
		if err == nil && info.IsDir() {
			if w.opts.Recursive {
				w.watcher.Add(event.Name)
			}
			go w.scanExistingFiles(event.Name)
			return
		}
	}

	if w.opts.FileFilter != nil && !w.opts.FileFilter(filename) {
		log.Printf("[watcher] filtered by FileFilter: %s", filename)
		return
	}
	if !isMediaFile(filename, w.org.config.MediaExtensions) {
		log.Printf("[watcher] not media file: %s (ext: %s)", filename, filepath.Ext(filename))
		return
	}

	w.enqueueFile(event.Name, true)
}

func (w *fsWatcher) checkStability() {
	ticker := time.NewTicker(w.opts.StableCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			w.processStableFiles()
		}
	}
}

func (w *fsWatcher) processStableFiles() {
	w.mu.Lock()
	now := time.Now()
	var stableFiles []string

	for path, lastMod := range w.stableMap {
		if now.Sub(lastMod) >= w.opts.StableWait {
			stableFiles = append(stableFiles, path)
			delete(w.stableMap, path)
		}
	}
	w.mu.Unlock()

	for _, path := range stableFiles {
		w.processFile(path)
	}
}

func (w *fsWatcher) enqueueFile(path string, logAdded bool) bool {
	now := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()

	w.pruneProcessedLocked(now)
	if _, ok := w.processing[path]; ok {
		return false
	}
	if _, ok := w.processed[path]; ok {
		return false
	}
	if _, ok := w.stableMap[path]; ok {
		w.stableMap[path] = now
		return false
	}
	w.stableMap[path] = now
	if logAdded {
		log.Printf("[watcher] adding to stableMap: %s", path)
	}
	return true
}

func (w *fsWatcher) markProcessing(path string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.processing[path]; ok {
		return false
	}
	if _, ok := w.processed[path]; ok {
		return false
	}
	w.processing[path] = struct{}{}
	return true
}

func (w *fsWatcher) markProcessed(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.processing, path)
	w.processed[path] = time.Now()
	w.pruneProcessedLocked(time.Now())
}

func (w *fsWatcher) pruneProcessedLocked(now time.Time) {
	ttl := w.opts.StableWait * 12
	if ttl < time.Minute {
		ttl = time.Minute
	}
	for path, t := range w.processed {
		if now.Sub(t) > ttl {
			delete(w.processed, path)
		}
	}
}

func (w *fsWatcher) processFile(path string) {
	if !w.markProcessing(path) {
		log.Printf("[watcher] skipping duplicate/in-flight file: %s", path)
		return
	}
	defer w.markProcessed(path)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Printf("[watcher] file not found: %s", path)
		return
	}

	filename := filepath.Base(path)
	if isSubtitleFile(filename, w.org.config.MediaExtensions) {
		log.Printf("[watcher] skipping subtitle: %s", filename)
		return
	}

	log.Printf("[watcher] processing file: %s", path)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	req := &Request{
		SourcePath:     path,
		TargetDir:      w.opts.TargetDir,
		Mode:           w.opts.Mode,
		FileOperation:  w.opts.FileOperation,
		SyncDelete:     w.opts.SyncDelete,
		Overwrite:      w.opts.Overwrite,
		NamingTemplate: cloneNamingTemplate(w.opts.NamingTemplate),
	}
	if req.NamingTemplate != nil && w.opts.EnableCategory != nil {
		req.NamingTemplate.EnableCategory = *w.opts.EnableCategory
	}

	if w.opts.EnableScrape != nil && *w.opts.EnableScrape {
		req.ScrapeMetadata = true
	}

	parsed, err := w.org.ParseFilename(filename)
	if err == nil && parsed != nil {
		w.org.applyDirectoryHints(path, parsed)
		if cached := w.lookupResourceMatch(path, parsed); cached != nil {
			tmdbID := cached.TMDBID
			mediaType := cached.MediaType
			req.TMDBID = &tmdbID
			req.MediaType = &mediaType
			goto organize
		}
	}
	if err == nil && parsed != nil && parsed.Title != "" {
		w.mu.Lock()
		for _, cached := range w.matchCache {
			if cached.Matched && cached.MediaType == "tv" && len(cached.OriginCountries) > 0 {
				if titlesMatch(parsed.Title, cached.Title, cached.OriginalTitle) {
					tmdbID := cached.TMDBID
					mediaType := cached.MediaType
					w.mu.Unlock()
					req.TMDBID = &tmdbID
					req.MediaType = &mediaType
					goto organize
				}
			}
		}
		w.mu.Unlock()
	}

organize:
	result, err := w.org.Organize(ctx, req)
	if err != nil {
		log.Printf("[watcher] organize error: %v", err)
		if w.opts.OnError != nil {
			w.opts.OnError(err)
		}
		return
	}

	if result.Execution != nil && result.Execution.Error != "" {
		log.Printf("[watcher] organize skipped: %s -> %s", path, result.Execution.Error)
		if isNonOverwriteTargetExists(result) {
			return
		}
		if w.opts.CallbackURL != "" {
			w.sendCallback(result, req)
		}
		return
	}

	if match := result.TMDBMatch; match != nil && match.Matched && len(match.OriginCountries) > 0 {
		w.mu.Lock()
		w.matchCache[match.TMDBID] = match
		if key := w.resourceKey(path); key != "" {
			w.resourceCache[key] = match
		}
		w.mu.Unlock()
	}

	log.Printf("[watcher] organize success: %s -> %s", path, result.NewPath.FullPath)
	if w.opts.OnComplete != nil {
		w.opts.OnComplete(result)
	}

	if w.opts.CallbackURL != "" {
		w.sendCallback(result, req)
	}
}

func (w *fsWatcher) lookupResourceMatch(path string, parsed *ParsedMedia) *TMDBMatchResult {
	key := w.resourceKey(path)
	if key == "" {
		return nil
	}
	w.mu.Lock()
	cached := w.resourceCache[key]
	w.mu.Unlock()
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
	log.Printf("[watcher] resource cache hit: %s -> tmdb-%d (%s)", key, cached.TMDBID, cached.MediaType)
	return cached
}

func (w *fsWatcher) resourceKey(path string) string {
	resourceDir := extractResourceDir(path, w.opts.Directory)
	if resourceDir == "" {
		return ""
	}
	abs, err := filepath.Abs(resourceDir)
	if err == nil {
		resourceDir = abs
	}
	return filepath.Clean(resourceDir)
}

func titlesMatch(query, title, originalTitle string) bool {
	q := strings.ToLower(query)
	return strings.ToLower(title) == q || strings.ToLower(originalTitle) == q
}

func (w *fsWatcher) sendCallback(result *Result, req *Request) {
	exec := result.Execution
	match := result.TMDBMatch
	parsed := result.Original

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
		"category":      result.Category,
		"title":         fallbackTitle,
		"originalTitle": "",
		"year":          "",
		"tmdbId":        0,
		"season":        0,
		"episode":       0,
		"scraped":       false,
		"strmGenerated": false,
	}

	if result.NewPath != nil {
		payload["targetPath"] = result.NewPath.FullPath
		payload["targetName"] = filepath.Base(result.NewPath.FullPath)
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
		if !exec.Success {
			payload["error"] = exec.Error
		}
	}

	body, _ := json.Marshal(map[string]any{"results": []map[string]any{payload}, "sourceType": "watch"})
	resp, err := http.Post(w.opts.CallbackURL, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[watcher] callback error: %v", err)
		return
	}
	defer resp.Body.Close()
	log.Printf("[watcher] callback sent: %d", resp.StatusCode)
}

func (w *fsWatcher) scanExistingFiles(dir string) {
	w.scanDir(dir, true)
}

func (w *fsWatcher) scanDir(dir string, initialWait bool) {
	if initialWait {
		time.Sleep(3 * time.Second)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		full := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			if w.opts.Recursive {
				w.watcher.Add(full)
				w.scanDir(full, false)
			}
			continue
		}
		if !isMediaFile(entry.Name(), w.org.config.MediaExtensions) {
			continue
		}
		if w.enqueueFile(full, false) {
			log.Printf("[watcher] scan found: %s", full)
		}
	}
}

func isMediaFile(filename string, extensions []string) bool {
	if len(extensions) == 0 {
		extensions = strings.Split(mediaExtDefault, ",")
	}

	ext := strings.ToLower(filepath.Ext(filename))
	for _, e := range extensions {
		if ext == strings.ToLower(strings.TrimSpace(e)) {
			return true
		}
	}
	return false
}

func isSubtitleFile(filename string, extensions []string) bool {
	if len(extensions) == 0 {
		extensions = strings.Split(subtitleExtDefault, ",")
	}

	ext := strings.ToLower(filepath.Ext(filename))
	for _, e := range extensions {
		if ext == strings.ToLower(strings.TrimSpace(e)) {
			return true
		}
	}
	return false
}
