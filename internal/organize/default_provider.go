//go:build ignore
// +build ignore

package organize

import (
	"context"
	"fmt"

	tmdb "github.com/cyruzin/golang-tmdb"
)

type defaultDetailProvider struct {
	client   *tmdb.Client
	language string
}

func newDefaultDetailProvider(client *tmdb.Client, language string) *defaultDetailProvider {
	if language == "" {
		language = "zh-CN"
	}
	return &defaultDetailProvider{client: client, language: language}
}

func (p *defaultDetailProvider) GetMovieDetails(ctx context.Context, tmdbID int) (*MovieScrapeInfo, error) {
	opts := map[string]string{"language": p.language, "append_to_response": "credits"}
	resp, err := p.client.GetMovieDetails(tmdbID, opts)
	if err != nil {
		return nil, fmt.Errorf("get movie details: %w", err)
	}

	info := &MovieScrapeInfo{
		Title:         resp.Title,
		OriginalTitle: resp.OriginalTitle,
		Overview:      resp.Overview,
		ReleaseDate:   resp.ReleaseDate,
		PosterPath:    resp.PosterPath,
		BackdropPath:  resp.BackdropPath,
		VoteAverage:   float64(resp.VoteAverage),
		TMDBID:        int(resp.ID),
	}

	if resp.Credits.Cast != nil {
		for _, c := range resp.Credits.Cast {
			info.Cast = append(info.Cast, CastInfo{
				Name:        c.Name,
				Character:   c.Character,
				Order:       c.Order,
				ProfilePath: c.ProfilePath,
			})
		}
	}

	return info, nil
}

func (p *defaultDetailProvider) GetTVDetails(ctx context.Context, tmdbID int) (*TVScrapeInfo, error) {
	opts := map[string]string{"language": p.language, "append_to_response": "credits"}
	resp, err := p.client.GetTVDetails(tmdbID, opts)
	if err != nil {
		return nil, fmt.Errorf("get tv details: %w", err)
	}

	info := &TVScrapeInfo{
		Title:         resp.Name,
		OriginalTitle: resp.OriginalName,
		Overview:      resp.Overview,
		ReleaseDate:   resp.FirstAirDate,
		PosterPath:    resp.PosterPath,
		BackdropPath:  resp.BackdropPath,
		VoteAverage:   float64(resp.VoteAverage),
		TMDBID:        int(resp.ID),
		TotalSeasons:  resp.NumberOfSeasons,
		Status:        resp.Status,
	}

	if resp.Credits.Cast != nil {
		for _, c := range resp.Credits.Cast {
			info.Cast = append(info.Cast, CastInfo{
				Name:        c.Name,
				Character:   c.Character,
				Order:       c.Order,
				ProfilePath: c.ProfilePath,
			})
		}
	}

	return info, nil
}

func (p *defaultDetailProvider) GetTVEpisodeDetails(ctx context.Context, tmdbID, seasonNum, episodeNum int) (*EpisodeScrapeInfo, error) {
	opts := map[string]string{"language": p.language}
	resp, err := p.client.GetTVEpisodeDetails(tmdbID, seasonNum, episodeNum, opts)
	if err != nil {
		return nil, fmt.Errorf("get tv episode details: %w", err)
	}
	return &EpisodeScrapeInfo{
		Title:       resp.Name,
		Overview:    resp.Overview,
		AirDate:     resp.AirDate,
		StillPath:   resp.StillPath,
		VoteAverage: float64(resp.VoteAverage),
		Season:      resp.SeasonNumber,
		Episode:     resp.EpisodeNumber,
	}, nil
}
