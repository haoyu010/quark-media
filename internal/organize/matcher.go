//go:build ignore
// +build ignore

package organize

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	tmdb "github.com/cyruzin/golang-tmdb"
)

const (
	// TMDB 当前实测 100 req/s 内无 429，但 120+ 延迟明显劣化、130+ 开始出现超时/重置。
	// 这里取 60 req/s：比官方旧限速和原 4 req/s 高很多，同时给刮削/网络抖动留余量。
	rateLimit       = 60
	rateInterval    = time.Second
	minConfidence   = 0.6
	confidentEnough = 0.85

	// TMDB SDK 默认超时较短，网络抖动时容易触发 context deadline exceeded。
	// 这里放宽超时并配合重试，降低偶发失败。
	tmdbHTTPTimeout    = 20 * time.Second
	tmdbRetryAttempts  = 3
	tmdbRetryBaseDelay = 600 * time.Millisecond
)

type tmdbMatcher struct {
	client   *tmdb.Client
	language string
	limiter  chan struct{}
	done     chan struct{}
}

func newTMDBMatcher(apiKey, language, baseURL string) (*tmdbMatcher, error) {
	if apiKey == "" {
		return nil, ErrMissingAPIKey
	}
	if language == "" {
		language = "zh-CN"
	}
	c, err := tmdb.Init(apiKey)
	if err != nil {
		return nil, fmt.Errorf("init tmdb client: %w", err)
	}
	if baseURL != "" {
		c.SetCustomBaseURL(baseURL)
	}
	c.SetClientAutoRetry()
	c.SetClientConfig(http.Client{
		Timeout: tmdbHTTPTimeout,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	})

	m := &tmdbMatcher{
		client:   c,
		language: language,
		limiter:  make(chan struct{}, rateLimit),
		done:     make(chan struct{}),
	}
	for i := 0; i < rateLimit; i++ {
		m.limiter <- struct{}{}
	}
	go m.refillLimiter()
	return m, nil
}

func (m *tmdbMatcher) Close() {
	close(m.done)
}

func (m *tmdbMatcher) refillLimiter() {
	ticker := time.NewTicker(rateInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
		refill:
			for i := 0; i < rateLimit; i++ {
				select {
				case m.limiter <- struct{}{}:
				default:
					break refill
				}
			}
		}
	}
}

func (m *tmdbMatcher) waitRateLimit(ctx context.Context) error {
	select {
	case <-m.limiter:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *tmdbMatcher) options() map[string]string {
	return map[string]string{"language": m.language}
}

func (m *tmdbMatcher) optionsWithLanguage(lang string) map[string]string {
	return map[string]string{"language": lang}
}

func (m *tmdbMatcher) shouldFallbackEnglish() bool {
	return !strings.EqualFold(m.language, "en-US")
}

func (m *tmdbMatcher) match(ctx context.Context, parsed *ParsedMedia) (*TMDBMatchResult, error) {
	if parsed == nil || parsed.Title == "" {
		return &TMDBMatchResult{Matched: false}, nil
	}

	query := parsed.Title

	var result *TMDBMatchResult
	var err error

	if parsed.IsMovie {
		result, err = m.matchMovie(ctx, query, parsed.Year)
	} else {
		result, err = m.matchTV(ctx, query, parsed.Year, parsed.Season)
	}
	if err != nil {
		return nil, err
	}

	if result != nil && result.Matched && len(result.OriginCountries) == 0 {
		m.enrichOriginCountries(ctx, result)
	}

	localizeChineseTitle(result)

	return result, nil
}

// localizeChineseTitle 让中文原产作品落地仍以中文命名，避免英文发行名（如 "Sword of Coming"）
// 在 zh-CN 命中失败、回退 en-US 命中时把 Title 写成英文发行名。
func localizeChineseTitle(r *TMDBMatchResult) {
	if r == nil || !r.Matched || r.OriginalTitle == "" {
		return
	}
	if !strings.HasPrefix(strings.ToLower(r.OriginalLanguage), "zh") {
		return
	}
	r.Title = r.OriginalTitle
}

func (m *tmdbMatcher) matchMovie(ctx context.Context, query string, year int) (*TMDBMatchResult, error) {
	// 先搜当前语言；置信度足够时直接返回。只有低置信度/无结果时再补英文，兼顾中英文命名和速度。
	primaryResults, firstErr := m.searchMovies(ctx, query, m.options(), "search movies")
	primaryBest := bestMovieMatch(query, year, primaryResults)
	if primaryBest != nil && primaryBest.Matched && primaryBest.Confidence >= confidentEnough {
		return primaryBest, nil
	}
	if !m.shouldFallbackEnglish() {
		if primaryBest != nil && primaryBest.Matched {
			return primaryBest, nil
		}
		if firstErr != nil && len(primaryResults) == 0 {
			return nil, firstErr
		}
		return &TMDBMatchResult{Matched: false}, nil
	}

	englishResults, englishErr := m.searchMovies(ctx, query, m.optionsWithLanguage("en-US"), "search movies (en)")
	// 同一 TMDB ID 下，zh-CN 与 en-US 返回的 Title/OriginalTitle 字段可能差异巨大
	// （例如纯英文片名的中文版返回中文标题），合并去重会丢掉相对查询更匹配的那一份。
	// 改为各自独立打分，取置信度较高的一份。
	englishBest := bestMovieMatch(query, year, englishResults)
	best := pickBetterMatch(primaryBest, englishBest)
	if best != nil && best.Matched {
		return best, nil
	}
	if firstErr != nil && len(primaryResults) == 0 && len(englishResults) == 0 {
		return nil, firstErr
	}
	if englishErr != nil && len(primaryResults) == 0 && len(englishResults) == 0 {
		return nil, englishErr
	}
	return &TMDBMatchResult{Matched: false}, nil
}

func (m *tmdbMatcher) searchMovies(ctx context.Context, query string, opts map[string]string, label string) (map[int64]tmdb.MovieResult, error) {
	results := make(map[int64]tmdb.MovieResult)
	var lastErr error
	for attempt := 1; attempt <= tmdbRetryAttempts; attempt++ {
		if err := m.waitRateLimit(ctx); err != nil {
			return results, err
		}
		resp, err := m.client.GetSearchMovies(query, opts)
		if err == nil {
			for _, r := range resp.Results {
				results[r.ID] = r
			}
			return results, nil
		}
		lastErr = err
		if !isRetryableTMDBError(err) || attempt == tmdbRetryAttempts {
			break
		}
		if err := sleepRetryBackoff(ctx, attempt); err != nil {
			return results, err
		}
	}
	return results, fmt.Errorf("%s: %w", label, lastErr)
}

func pickBetterMatch(a, b *TMDBMatchResult) *TMDBMatchResult {
	switch {
	case a == nil || !a.Matched:
		return b
	case b == nil || !b.Matched:
		return a
	case b.Confidence > a.Confidence:
		return b
	default:
		return a
	}
}

func bestMovieMatch(query string, year int, results map[int64]tmdb.MovieResult) *TMDBMatchResult {
	if len(results) == 0 {
		return &TMDBMatchResult{Matched: false}
	}
	var bestResult *TMDBMatchResult
	bestConfidence := 0.0
	for _, r := range results {
		resultYear := extractYearFromDate(r.ReleaseDate)
		confidence := calculateConfidence(query, r.Title, r.OriginalTitle, year, resultYear, 0, 0)
		if confidence > bestConfidence {
			bestConfidence = confidence
			bestResult = movieResultToMatch(r, confidence)
		}
	}
	if bestResult != nil && bestResult.Confidence >= minConfidence {
		return bestResult
	}
	return &TMDBMatchResult{Matched: false}
}

func movieResultToMatch(r tmdb.MovieResult, confidence float64) *TMDBMatchResult {
	return &TMDBMatchResult{
		Matched:          true,
		TMDBID:           int(r.ID),
		MediaType:        "movie",
		Title:            r.Title,
		OriginalTitle:    r.OriginalTitle,
		Year:             extractYearFromDate(r.ReleaseDate),
		Overview:         r.Overview,
		PosterPath:       r.PosterPath,
		BackdropPath:     r.BackdropPath,
		Confidence:       confidence,
		VoteAverage:      float64(r.VoteAverage),
		GenreIDs:         intSliceToStringSlice(r.GenreIDs),
		OriginalLanguage: r.OriginalLanguage,
	}
}

func (m *tmdbMatcher) matchTV(ctx context.Context, query string, year, season int) (*TMDBMatchResult, error) {
	primaryResults, firstErr := m.searchTV(ctx, query, m.options(), "search tv")
	primaryBest := bestTVMatch(query, year, season, primaryResults)
	if primaryBest != nil && primaryBest.Matched && primaryBest.Confidence >= confidentEnough {
		return primaryBest, nil
	}
	if !m.shouldFallbackEnglish() {
		if primaryBest != nil && primaryBest.Matched {
			if season > 0 && len(primaryResults) > 1 {
				candidates := m.collectTVCandidates(query, year, season, primaryResults)
				if len(candidates) > 1 {
					return m.disambiguateTVBySeason(ctx, candidates, season), nil
				}
			}
			return primaryBest, nil
		}
		if firstErr != nil && len(primaryResults) == 0 {
			return nil, firstErr
		}
		return &TMDBMatchResult{Matched: false}, nil
	}

	englishResults, englishErr := m.searchTV(ctx, query, m.optionsWithLanguage("en-US"), "search tv (en)")
	englishBest := bestTVMatch(query, year, season, englishResults)
	best := pickBetterMatch(primaryBest, englishBest)

	if best != nil && best.Matched && season > 0 {
		allResults := mergeTVResults(primaryResults, englishResults)
		if len(allResults) > 1 {
			candidates := m.collectTVCandidates(query, year, season, allResults)
			if len(candidates) > 1 {
				return m.disambiguateTVBySeason(ctx, candidates, season), nil
			}
		}
		return best, nil
	}

	if best != nil && best.Matched {
		return best, nil
	}
	if firstErr != nil && len(primaryResults) == 0 && len(englishResults) == 0 {
		return nil, firstErr
	}
	if englishErr != nil && len(primaryResults) == 0 && len(englishResults) == 0 {
		return nil, englishErr
	}
	return &TMDBMatchResult{Matched: false}, nil
}

func mergeTVResults(a, b map[int64]tmdb.TVShowResult) map[int64]tmdb.TVShowResult {
	merged := make(map[int64]tmdb.TVShowResult, len(a)+len(b))
	for id, r := range a {
		merged[id] = r
	}
	for id, r := range b {
		if _, exists := merged[id]; !exists {
			merged[id] = r
		}
	}
	return merged
}

func (*tmdbMatcher) collectTVCandidates(query string, year, season int, results map[int64]tmdb.TVShowResult) []*TMDBMatchResult {
	var candidates []*TMDBMatchResult
	for _, r := range results {
		resultYear := extractYearFromDate(r.FirstAirDate)
		confidence := calculateConfidence(query, r.Name, r.OriginalName, year, resultYear, season, 0)
		if confidence >= minConfidence {
			candidates = append(candidates, tvResultToMatch(r, confidence))
		}
	}
	return candidates
}

func (m *tmdbMatcher) searchTV(ctx context.Context, query string, opts map[string]string, label string) (map[int64]tmdb.TVShowResult, error) {
	results := make(map[int64]tmdb.TVShowResult)
	var lastErr error
	for attempt := 1; attempt <= tmdbRetryAttempts; attempt++ {
		if err := m.waitRateLimit(ctx); err != nil {
			return results, err
		}
		resp, err := m.client.GetSearchTVShow(query, opts)
		if err == nil {
			for _, r := range resp.Results {
				results[r.ID] = r
			}
			return results, nil
		}
		lastErr = err
		if !isRetryableTMDBError(err) || attempt == tmdbRetryAttempts {
			break
		}
		if err := sleepRetryBackoff(ctx, attempt); err != nil {
			return results, err
		}
	}
	return results, fmt.Errorf("%s: %w", label, lastErr)
}

func bestTVMatch(query string, year, season int, results map[int64]tmdb.TVShowResult) *TMDBMatchResult {
	if len(results) == 0 {
		return &TMDBMatchResult{Matched: false}
	}
	var bestResult *TMDBMatchResult
	bestConfidence := 0.0
	for _, r := range results {
		resultYear := extractYearFromDate(r.FirstAirDate)
		confidence := calculateConfidence(query, r.Name, r.OriginalName, year, resultYear, season, 0)
		if confidence > bestConfidence {
			bestConfidence = confidence
			bestResult = tvResultToMatch(r, confidence)
		}
	}
	if bestResult != nil && bestResult.Confidence >= minConfidence {
		return bestResult
	}
	return &TMDBMatchResult{Matched: false}
}

// disambiguateTVBySeason resolves ambiguous TV matches by querying TMDB detail API for season count.
// If season > 0, results whose total seasons < season are penalized.
func (m *tmdbMatcher) disambiguateTVBySeason(ctx context.Context, candidates []*TMDBMatchResult, season int) *TMDBMatchResult {
	if season <= 0 || len(candidates) <= 1 {
		return candidates[0]
	}

	var best *TMDBMatchResult
	bestScore := -1.0
	for _, c := range candidates {
		totalSeasons := 0
		if m.client != nil {
			if err := m.waitRateLimit(ctx); err != nil {
				return candidates[0]
			}
			details, err := m.client.GetTVDetails(c.TMDBID, m.options())
			if err == nil && details != nil {
				totalSeasons = details.NumberOfSeasons
			}
		}

		score := c.Confidence
		if totalSeasons > 0 {
			if totalSeasons < season {
				score -= 0.3
			} else {
				score += 0.05
			}
		}

		if score > bestScore {
			bestScore = score
			best = c
		}
	}
	return best
}

func tvResultToMatch(r tmdb.TVShowResult, confidence float64) *TMDBMatchResult {
	return &TMDBMatchResult{
		Matched:          true,
		TMDBID:           int(r.ID),
		MediaType:        "tv",
		Title:            r.Name,
		OriginalTitle:    r.OriginalName,
		Year:             extractYearFromDate(r.FirstAirDate),
		Overview:         r.Overview,
		PosterPath:       r.PosterPath,
		BackdropPath:     r.BackdropPath,
		Confidence:       confidence,
		VoteAverage:      float64(r.VoteAverage),
		GenreIDs:         intSliceToStringSlice(r.GenreIDs),
		OriginalLanguage: r.OriginalLanguage,
		OriginCountries:  r.OriginCountry,
	}
}

func (m *tmdbMatcher) getByID(ctx context.Context, tmdbID int, mediaType string) (*TMDBMatchResult, error) {
	opts := m.options()

	if mediaType == "movie" {
		var lastErr error
		for attempt := 1; attempt <= tmdbRetryAttempts; attempt++ {
			if err := m.waitRateLimit(ctx); err != nil {
				return nil, err
			}
			resp, err := m.client.GetMovieDetails(tmdbID, opts)
			if err == nil {
				genreIDs := make([]string, len(resp.Genres))
				for i, g := range resp.Genres {
					genreIDs[i] = fmt.Sprintf("%d", g.ID)
				}
				return &TMDBMatchResult{
					Matched:          true,
					TMDBID:           int(resp.ID),
					MediaType:        "movie",
					Title:            resp.Title,
					OriginalTitle:    resp.OriginalTitle,
					Year:             extractYearFromDate(resp.ReleaseDate),
					Overview:         resp.Overview,
					PosterPath:       resp.PosterPath,
					BackdropPath:     resp.BackdropPath,
					Confidence:       1.0,
					VoteAverage:      float64(resp.VoteAverage),
					GenreIDs:         genreIDs,
					OriginalLanguage: resp.OriginalLanguage,
				}, nil
			}
			lastErr = err
			if !isRetryableTMDBError(err) || attempt == tmdbRetryAttempts {
				break
			}
			if err := sleepRetryBackoff(ctx, attempt); err != nil {
				return nil, err
			}
		}
		return nil, fmt.Errorf("get movie details: %w", lastErr)
	}

	var lastErr error
	for attempt := 1; attempt <= tmdbRetryAttempts; attempt++ {
		if err := m.waitRateLimit(ctx); err != nil {
			return nil, err
		}
		resp, err := m.client.GetTVDetails(tmdbID, opts)
		if err == nil {
			genreIDs := make([]string, len(resp.Genres))
			for i, g := range resp.Genres {
				genreIDs[i] = fmt.Sprintf("%d", g.ID)
			}
			return &TMDBMatchResult{
				Matched:          true,
				TMDBID:           int(resp.ID),
				MediaType:        "tv",
				Title:            resp.Name,
				OriginalTitle:    resp.OriginalName,
				Year:             extractYearFromDate(resp.FirstAirDate),
				Overview:         resp.Overview,
				PosterPath:       resp.PosterPath,
				BackdropPath:     resp.BackdropPath,
				Confidence:       1.0,
				VoteAverage:      float64(resp.VoteAverage),
				TotalEpisodes:    resp.NumberOfEpisodes,
				GenreIDs:         genreIDs,
				OriginalLanguage: resp.OriginalLanguage,
				OriginCountries:  resp.OriginCountry,
			}, nil
		}
		lastErr = err
		if !isRetryableTMDBError(err) || attempt == tmdbRetryAttempts {
			break
		}
		if err := sleepRetryBackoff(ctx, attempt); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get tv details: %w", lastErr)
}

func extractYearFromDate(date string) int {
	if len(date) >= 4 {
		y, err := strconv.Atoi(date[:4])
		if err == nil {
			return y
		}
	}
	return 0
}

func (m *tmdbMatcher) enrichOriginCountries(ctx context.Context, result *TMDBMatchResult) {
	if result.TMDBID <= 0 {
		return
	}
	if err := m.waitRateLimit(ctx); err != nil {
		return
	}
	if result.MediaType == "tv" {
		resp, err := m.client.GetTVDetails(result.TMDBID, m.options())
		if err != nil || resp == nil {
			return
		}
		result.OriginCountries = resp.OriginCountry
		if len(result.GenreIDs) == 0 && len(resp.Genres) > 0 {
			genreIDs := make([]string, len(resp.Genres))
			for i, g := range resp.Genres {
				genreIDs[i] = fmt.Sprintf("%d", g.ID)
			}
			result.GenreIDs = genreIDs
		}
	} else {
		resp, err := m.client.GetMovieDetails(result.TMDBID, m.options())
		if err != nil || resp == nil {
			return
		}
		if len(resp.ProductionCountries) > 0 {
			countries := make([]string, len(resp.ProductionCountries))
			for i, c := range resp.ProductionCountries {
				countries[i] = c.Iso3166_1
			}
			result.OriginCountries = countries
		}
		if len(result.GenreIDs) == 0 && len(resp.Genres) > 0 {
			genreIDs := make([]string, len(resp.Genres))
			for i, g := range resp.Genres {
				genreIDs[i] = fmt.Sprintf("%d", g.ID)
			}
			result.GenreIDs = genreIDs
		}
	}
}

func calculateConfidence(query, title, originalTitle string, queryYear, resultYear, querySeason, _ int) float64 {
	queryLower := strings.ToLower(query)
	titleLower := strings.ToLower(title)
	origTitleLower := strings.ToLower(originalTitle)

	titleScore := 0.0
	if queryLower == titleLower || queryLower == origTitleLower {
		titleScore = 100
	} else if strings.Contains(titleLower, queryLower) || strings.Contains(origTitleLower, queryLower) {
		titleScore = 80
	} else if strings.Contains(queryLower, titleLower) || strings.Contains(queryLower, origTitleLower) {
		titleScore = 70
	} else {
		titleScore = stringSimilarityLevenshtein(query, title)
		if origTitleLower != titleLower {
			altScore := stringSimilarityLevenshtein(query, originalTitle)
			if altScore > titleScore {
				titleScore = altScore
			}
		}
	}

	yearScore := 70.0
	if queryYear > 0 && resultYear > 0 {
		diff := resultYear - queryYear
		if diff < 0 {
			diff = -diff
		}
		switch {
		case diff == 0:
			yearScore = 100
		case diff == 1:
			yearScore = 80
		case diff == 2:
			yearScore = 50
		default:
			yearScore = 20
		}
	} else if queryYear == 0 && resultYear > 0 {
		// No year from filename → prefer newer: 1973≈40, 2020≈88, 2025≈95
		yearScore = 40 + float64(min(resultYear, 2025)-1970)*0.0714
		if yearScore > 95 {
			yearScore = 95
		}
	}

	seasonScore := 100.0
	if querySeason > 0 {
		seasonScore = 90
	}

	return (titleScore*0.6 + yearScore*0.3 + seasonScore*0.1) / 100
}

func intSliceToStringSlice(ints []int64) []string {
	result := make([]string, len(ints))
	for i, v := range ints {
		result[i] = fmt.Sprintf("%d", v)
	}
	return result
}

func sleepRetryBackoff(ctx context.Context, attempt int) error {
	delay := tmdbRetryBaseDelay * time.Duration(attempt)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isRetryableTMDBError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "context deadline exceeded"),
		strings.Contains(msg, "timeout"),
		strings.Contains(msg, "temporarily unavailable"),
		strings.Contains(msg, "connection reset by peer"),
		strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "broken pipe"),
		strings.Contains(msg, "eof"),
		strings.Contains(msg, "too many requests"),
		strings.Contains(msg, "status 429"),
		strings.Contains(msg, "status 500"),
		strings.Contains(msg, "status 502"),
		strings.Contains(msg, "status 503"),
		strings.Contains(msg, "status 504"):
		return true
	default:
		return false
	}
}
