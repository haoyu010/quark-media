//go:build ignore
// +build ignore

package organize

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// tmdbImageBaseURL TMDB 图片基础 URL（API 返回的 PosterPath 等是相对路径，需拼接此前缀）
const tmdbImageBaseURL = "https://image.tmdb.org/t/p"

const (
	tmdbPosterSize   = "w342"
	tmdbBackdropSize = "w780"
	tmdbStillSize    = "w300"
	tmdbLogoSize     = "w300"

	defaultImageDownloadConcurrency = 4
	defaultImageDownloadTimeout     = 15 * time.Second
	defaultImageQueueSizeMultiplier = 8
)

type ScraperOptions struct {
	ImageDownloadConcurrency int
	ImageDownloadQueueSize   int
	ImageDownloadTimeout     time.Duration
}

type imageDownloadTask struct {
	url  string
	dst  string
	size string
}

type Scraper struct {
	provider TMDBDetailProvider
	client   *http.Client

	mu             sync.Mutex
	movieCache     map[int]*MovieScrapeInfo
	tvCache        map[int]*TVScrapeInfo
	imageMu        sync.Mutex
	imageQueue     chan imageDownloadTask
	imagePending   map[string]struct{}
	imageQueueDone bool
	imageCtx       context.Context
	imageCancel    context.CancelFunc
}

func NewScraper(provider TMDBDetailProvider, httpClient *http.Client) *Scraper {
	return NewScraperWithOptions(provider, httpClient, ScraperOptions{})
}

func NewScraperWithOptions(provider TMDBDetailProvider, httpClient *http.Client, opts ScraperOptions) *Scraper {
	concurrency := opts.ImageDownloadConcurrency
	if concurrency <= 0 {
		concurrency = defaultImageDownloadConcurrency
	}
	queueSize := opts.ImageDownloadQueueSize
	if queueSize <= 0 {
		queueSize = concurrency * defaultImageQueueSizeMultiplier
	}
	if queueSize < concurrency {
		queueSize = concurrency
	}
	if httpClient == nil {
		timeout := opts.ImageDownloadTimeout
		if timeout <= 0 {
			timeout = defaultImageDownloadTimeout
		}
		httpClient = &http.Client{Timeout: timeout}
	}
	bgCtx, cancel := context.WithCancel(context.Background())

	s := &Scraper{
		provider:       provider,
		client:         httpClient,
		movieCache:     make(map[int]*MovieScrapeInfo),
		tvCache:        make(map[int]*TVScrapeInfo),
		imageQueue:     make(chan imageDownloadTask, queueSize),
		imagePending:   make(map[string]struct{}),
		imageCtx:       bgCtx,
		imageCancel:    cancel,
		imageQueueDone: false,
	}

	for i := 0; i < concurrency; i++ {
		go s.imageWorker()
	}

	return s
}

func (s *Scraper) Close() {
	if s == nil {
		return
	}
	var cancel context.CancelFunc
	s.imageMu.Lock()
	if s.imageQueueDone {
		s.imageMu.Unlock()
		return
	}
	s.imageQueueDone = true
	close(s.imageQueue)
	cancel = s.imageCancel
	s.imageMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Scraper) imageWorker() {
	for task := range s.imageQueue {
		_ = s.downloadImage(s.imageCtx, task.url, task.dst, task.size)
		s.imageMu.Lock()
		delete(s.imagePending, task.dst)
		s.imageMu.Unlock()
	}
}

func (s *Scraper) enqueueImage(url, dst, size string) {
	if s == nil || url == "" || dst == "" {
		return
	}
	if _, err := os.Stat(dst); err == nil {
		return
	}

	s.imageMu.Lock()
	defer s.imageMu.Unlock()
	if s.imageQueueDone {
		return
	}
	if _, exists := s.imagePending[dst]; exists {
		return
	}
	task := imageDownloadTask{url: url, dst: dst, size: size}
	select {
	case s.imageQueue <- task:
		s.imagePending[dst] = struct{}{}
	default:
		// queue full: best effort only, drop task immediately to avoid blocking main flow
	}
}

func (s *Scraper) ScrapeMovie(ctx context.Context, targetPath string, tmdbID int) error {
	if s == nil || s.provider == nil || tmdbID <= 0 {
		return nil
	}

	details, err := s.getMovieDetails(ctx, tmdbID)
	if err != nil || details == nil {
		return err
	}

	showDir := targetPath
	if err := os.MkdirAll(showDir, 0o755); err != nil {
		return err
	}
	base := strings.TrimSuffix(filepath.Base(targetPath), filepath.Ext(targetPath))

	if err := writeIfMissing(filepath.Join(showDir, base+".nfo"), func() ([]byte, error) {
		return []byte(renderMovieNFO(details)), nil
	}); err != nil {
		return err
	}
	s.enqueueImage(details.PosterPath, filepath.Join(showDir, "poster.jpg"), tmdbPosterSize)
	s.enqueueImage(details.BackdropPath, filepath.Join(showDir, "fanart.jpg"), tmdbBackdropSize)
	s.enqueueImage(details.LogoPath, filepath.Join(showDir, "clearlogo.png"), tmdbLogoSize)
	return nil
}

func (s *Scraper) ScrapeTV(ctx context.Context, targetPath string, tmdbID int, season, episode int, episodeFileName string) error {
	if s == nil || s.provider == nil || tmdbID <= 0 {
		return nil
	}

	seasonDir := targetPath
	showDir := filepath.Dir(seasonDir)
	if err := s.ScrapeTVShow(ctx, showDir, tmdbID); err != nil {
		return err
	}
	return s.ScrapeTVEpisode(ctx, seasonDir, tmdbID, season, episode, episodeFileName)
}

func (s *Scraper) ScrapeTVShow(ctx context.Context, showDir string, tmdbID int) error {
	if s == nil || s.provider == nil || tmdbID <= 0 {
		return nil
	}

	details, err := s.getTVDetails(ctx, tmdbID)
	if err != nil || details == nil {
		return err
	}

	if err := os.MkdirAll(showDir, 0o755); err != nil {
		return err
	}

	if err := writeIfMissing(filepath.Join(showDir, "tvshow.nfo"), func() ([]byte, error) {
		return []byte(renderTVShowNFO(details)), nil
	}); err != nil {
		return err
	}
	s.enqueueImage(details.PosterPath, filepath.Join(showDir, "poster.jpg"), tmdbPosterSize)
	s.enqueueImage(details.BackdropPath, filepath.Join(showDir, "fanart.jpg"), tmdbBackdropSize)
	s.enqueueImage(details.LogoPath, filepath.Join(showDir, "clearlogo.png"), tmdbLogoSize)
	return nil
}

func (s *Scraper) ScrapeTVEpisode(ctx context.Context, seasonDir string, tmdbID int, season, episode int, episodeFileName string) error {
	if s == nil || s.provider == nil || tmdbID <= 0 {
		return nil
	}
	if err := os.MkdirAll(seasonDir, 0o755); err != nil {
		return err
	}

	if season <= 0 || episode <= 0 {
		return nil
	}

	if details, err := s.getTVDetails(ctx, tmdbID); err == nil && details != nil {
		s.enqueueImage(details.PosterPath, filepath.Join(seasonDir, "seasonposter.jpg"), tmdbPosterSize)
	} else if err != nil {
		return err
	}

	epInfo, err := s.provider.GetTVEpisodeDetails(ctx, tmdbID, season, episode)
	if err != nil {
		return fmt.Errorf("get episode details: %w", err)
	}
	if epInfo == nil {
		return nil
	}

	epBase := episodeFileName
	if epBase == "" {
		epBase = fmt.Sprintf("S%02dE%02d", season, episode)
	}

	if err := writeIfMissing(filepath.Join(seasonDir, epBase+".nfo"), func() ([]byte, error) {
		return []byte(renderEpisodeNFO(epInfo)), nil
	}); err != nil {
		return err
	}
	s.enqueueImage(epInfo.StillPath, filepath.Join(seasonDir, epBase+"-thumb.jpg"), tmdbStillSize)

	return nil
}

func (s *Scraper) getMovieDetails(ctx context.Context, tmdbID int) (*MovieScrapeInfo, error) {
	s.mu.Lock()
	cached := s.movieCache[tmdbID]
	s.mu.Unlock()
	if cached != nil {
		return cached, nil
	}
	details, err := s.provider.GetMovieDetails(ctx, tmdbID)
	if err != nil || details == nil {
		return details, err
	}
	s.mu.Lock()
	s.movieCache[tmdbID] = details
	s.mu.Unlock()
	return details, nil
}

func (s *Scraper) getTVDetails(ctx context.Context, tmdbID int) (*TVScrapeInfo, error) {
	s.mu.Lock()
	cached := s.tvCache[tmdbID]
	s.mu.Unlock()
	if cached != nil {
		return cached, nil
	}
	details, err := s.provider.GetTVDetails(ctx, tmdbID)
	if err != nil || details == nil {
		return details, err
	}
	s.mu.Lock()
	s.tvCache[tmdbID] = details
	s.mu.Unlock()
	return details, nil
}

func (s *Scraper) downloadImage(ctx context.Context, url, dst, size string) error {
	if s == nil || s.client == nil {
		return nil
	}
	if url == "" || dst == "" {
		return nil
	}
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	if !strings.HasPrefix(strings.ToLower(url), "http") {
		url = tmdbImageURL(url, size)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(ct, "image/") && !strings.HasPrefix(ct, "application/octet-stream") {
		return fmt.Errorf("unexpected content type: %s", ct)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".organizer-img-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("copy: %w", err)
	}
	tmp.Close()
	if err := os.Rename(tmpPath, dst); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

func tmdbImageURL(path, size string) string {
	if path == "" {
		return ""
	}
	if size == "" {
		size = "original"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return tmdbImageBaseURL + "/" + size + path
}

func writeIfMissing(path string, gen func() ([]byte, error)) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := gen()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func renderMovieNFO(d *MovieScrapeInfo) string {
	if d == nil {
		return ""
	}
	year := extractYearFromString(d.ReleaseDate)
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes" ?>` + "\n")
	b.WriteString("<movie>\n")
	fmt.Fprintf(&b, "  <title>%s</title>\n", xmlEscape(d.Title))
	if d.OriginalTitle != "" {
		fmt.Fprintf(&b, "  <originaltitle>%s</originaltitle>\n", xmlEscape(d.OriginalTitle))
	}
	if year != "" {
		fmt.Fprintf(&b, "  <year>%s</year>\n", xmlEscape(year))
	}
	if d.ReleaseDate != "" {
		fmt.Fprintf(&b, "  <premiered>%s</premiered>\n", xmlEscape(d.ReleaseDate))
	}
	if d.Overview != "" {
		fmt.Fprintf(&b, "  <plot><![CDATA[%s]]></plot>\n", xmlCDATA(d.Overview))
	}
	fmt.Fprintf(&b, "  <rating>%.1f</rating>\n", d.VoteAverage)
	fmt.Fprintf(&b, "  <tmdbid>%d</tmdbid>\n", d.TMDBID)
	fmt.Fprintf(&b, "  <uniqueid type=\"tmdb\" default=\"true\">%d</uniqueid>\n", d.TMDBID)
	for _, c := range d.Cast {
		b.WriteString("  <actor>\n")
		fmt.Fprintf(&b, "    <name>%s</name>\n", xmlEscape(c.Name))
		fmt.Fprintf(&b, "    <role>%s</role>\n", xmlEscape(c.Character))
		fmt.Fprintf(&b, "    <order>%d</order>\n", c.Order)
		if c.ProfilePath != "" {
			fmt.Fprintf(&b, "    <thumb>%s</thumb>\n", xmlEscape(c.ProfilePath))
		}
		b.WriteString("  </actor>\n")
	}
	b.WriteString("</movie>\n")
	return b.String()
}

func renderTVShowNFO(d *TVScrapeInfo) string {
	if d == nil {
		return ""
	}
	year := extractYearFromString(d.ReleaseDate)
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes" ?>` + "\n")
	b.WriteString("<tvshow>\n")
	fmt.Fprintf(&b, "  <title>%s</title>\n", xmlEscape(d.Title))
	if d.OriginalTitle != "" {
		fmt.Fprintf(&b, "  <originaltitle>%s</originaltitle>\n", xmlEscape(d.OriginalTitle))
	}
	if year != "" {
		fmt.Fprintf(&b, "  <year>%s</year>\n", xmlEscape(year))
	}
	if d.ReleaseDate != "" {
		fmt.Fprintf(&b, "  <premiered>%s</premiered>\n", xmlEscape(d.ReleaseDate))
	}
	if d.Overview != "" {
		fmt.Fprintf(&b, "  <plot><![CDATA[%s]]></plot>\n", xmlCDATA(d.Overview))
	}
	fmt.Fprintf(&b, "  <rating>%.1f</rating>\n", d.VoteAverage)
	if d.Status != "" {
		fmt.Fprintf(&b, "  <status>%s</status>\n", xmlEscape(d.Status))
	}
	if d.TotalSeasons > 0 {
		fmt.Fprintf(&b, "  <season>%d</season>\n", d.TotalSeasons)
	}
	fmt.Fprintf(&b, "  <tmdbid>%d</tmdbid>\n", d.TMDBID)
	fmt.Fprintf(&b, "  <uniqueid type=\"tmdb\" default=\"true\">%d</uniqueid>\n", d.TMDBID)
	for _, c := range d.Cast {
		b.WriteString("  <actor>\n")
		fmt.Fprintf(&b, "    <name>%s</name>\n", xmlEscape(c.Name))
		fmt.Fprintf(&b, "    <role>%s</role>\n", xmlEscape(c.Character))
		fmt.Fprintf(&b, "    <order>%d</order>\n", c.Order)
		if c.ProfilePath != "" {
			fmt.Fprintf(&b, "    <thumb>%s</thumb>\n", xmlEscape(c.ProfilePath))
		}
		b.WriteString("  </actor>\n")
	}
	b.WriteString("</tvshow>\n")
	return b.String()
}

var xmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;")

func xmlEscape(s string) string { return xmlEscaper.Replace(s) }

func xmlCDATA(s string) string {
	return strings.ReplaceAll(s, "]]>", "]]]]><![CDATA[>")
}

func renderEpisodeNFO(d *EpisodeScrapeInfo) string {
	if d == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes" ?>` + "\n")
	b.WriteString("<episodedetails>\n")
	fmt.Fprintf(&b, "  <title>%s</title>\n", xmlEscape(d.Title))
	fmt.Fprintf(&b, "  <season>%d</season>\n", d.Season)
	fmt.Fprintf(&b, "  <episode>%d</episode>\n", d.Episode)
	if d.Overview != "" {
		fmt.Fprintf(&b, "  <plot><![CDATA[%s]]></plot>\n", xmlCDATA(d.Overview))
	}
	if d.AirDate != "" {
		fmt.Fprintf(&b, "  <aired>%s</aired>\n", xmlEscape(d.AirDate))
	}
	fmt.Fprintf(&b, "  <rating>%.1f</rating>\n", d.VoteAverage)
	b.WriteString("</episodedetails>\n")
	return b.String()
}
