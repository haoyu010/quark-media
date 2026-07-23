//go:build ignore
// +build ignore

package organize

import "errors"

var (
	ErrEmptyFilename       = errors.New("filename is empty")
	ErrMissingAPIKey       = errors.New("tmdb api key is required")
	ErrNilParsedMedia      = errors.New("parsed media is nil")
	ErrNilRequest          = errors.New("request is nil")
	ErrInvalidMode         = errors.New("invalid organize mode")
	ErrWatchExists         = errors.New("directory watch already exists")
	ErrWatchNotFound       = errors.New("directory watch not found")
	ErrConflictDetected    = errors.New("target path conflict detected")
	ErrNoFilesProvided     = errors.New("no files provided")
	ErrScrapeNotConfigured = errors.New("scrape not configured: TMDBDetailProvider not set")
)
