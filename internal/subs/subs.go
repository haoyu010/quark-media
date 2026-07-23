package subs

import (
	"fmt"
	"strings"

	"quark-media/internal/config"
)

type View struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	TMDBID      string   `json:"tmdb_id"`
	ContentType string   `json:"content_type"`
	MediaType   string   `json:"media_type"`
	Keywords    []string `json:"keywords"`
	SavePath    string   `json:"save_path"`
	ShareURL    string   `json:"share_url"`
	Sources     []string `json:"sources"`
	Enabled     bool     `json:"enabled"`
	StrmSubdir  string   `json:"strm_subdir"`
	PosterURL   string   `json:"poster_url"`
}

func List(cfg *config.Config) []View {
	out := make([]View, 0, len(cfg.Subscriptions))
	for i, s := range cfg.Subscriptions {
		en := true
		if s.Enabled != nil {
			en = *s.Enabled
		}
		out = append(out, View{
			ID: fmt.Sprintf("sub:%d", i), Name: s.Name, Title: s.Name,
			TMDBID: s.TMDBID, ContentType: s.ContentType, MediaType: s.ContentType,
			Keywords: s.Keywords, SavePath: s.SavePath, ShareURL: s.ShareURL,
			Sources: s.Sources, Enabled: en, StrmSubdir: s.StrmSubdir, PosterURL: s.PosterURL,
		})
	}
	return out
}

func Create(cfg *config.Config, body map[string]any) (View, error) {
	s := mapToSub(body, config.Subscription{})
	if s.Name == "" && s.SavePath == "" && s.ShareURL == "" {
		return View{}, fmt.Errorf("need name/path/share")
	}
	if s.Name == "" {
		s.Name = s.SavePath
		if s.Name == "" {
			s.Name = s.ShareURL
		}
	}
	cfg.Subscriptions = append(cfg.Subscriptions, s)
	if err := cfg.Save(); err != nil {
		return View{}, err
	}
	return List(cfg)[len(cfg.Subscriptions)-1], nil
}

func Update(cfg *config.Config, id string, body map[string]any) (View, error) {
	var idx int
	if _, err := fmt.Sscanf(id, "sub:%d", &idx); err != nil || idx < 0 || idx >= len(cfg.Subscriptions) {
		return View{}, fmt.Errorf("subscription not found")
	}
	cfg.Subscriptions[idx] = mapToSub(body, cfg.Subscriptions[idx])
	if err := cfg.Save(); err != nil {
		return View{}, err
	}
	return List(cfg)[idx], nil
}

func Delete(cfg *config.Config, id string) error {
	var idx int
	if _, err := fmt.Sscanf(id, "sub:%d", &idx); err != nil || idx < 0 || idx >= len(cfg.Subscriptions) {
		return fmt.Errorf("subscription not found")
	}
	cfg.Subscriptions = append(cfg.Subscriptions[:idx], cfg.Subscriptions[idx+1:]...)
	return cfg.Save()
}

func mapToSub(body map[string]any, base config.Subscription) config.Subscription {
	s := base
	if v, ok := body["name"].(string); ok {
		s.Name = v
	}
	if v, ok := body["title"].(string); ok && s.Name == "" {
		s.Name = v
	}
	if v, ok := body["tmdb_id"].(string); ok {
		s.TMDBID = v
	} else if v, ok := body["tmdb_id"].(float64); ok {
		s.TMDBID = fmt.Sprintf("%.0f", v)
	}
	if v, ok := body["content_type"].(string); ok {
		s.ContentType = v
	}
	if s.ContentType == "" {
		if v, ok := body["media_type"].(string); ok {
			s.ContentType = v
		}
	}
	if s.ContentType == "" {
		s.ContentType = "tv"
	}
	if v, ok := body["save_path"].(string); ok {
		s.SavePath = v
	}
	if v, ok := body["share_url"].(string); ok {
		s.ShareURL = v
	}
	if v, ok := body["strm_subdir"].(string); ok {
		s.StrmSubdir = v
	}
	if v, ok := body["poster_url"].(string); ok {
		s.PosterURL = v
	}
	if v, ok := body["keywords"].(string); ok && v != "" {
		s.Keywords = splitCSV(v)
	}
	if v, ok := body["keywords"].([]any); ok {
		s.Keywords = anyToStrs(v)
	}
	if v, ok := body["sources"].(string); ok && v != "" {
		s.Sources = splitCSV(v)
	}
	if v, ok := body["sources"].([]any); ok {
		s.Sources = anyToStrs(v)
	}
	if len(s.Sources) == 0 {
		s.Sources = []string{"telegram", "qas"}
	}
	if v, ok := body["enabled"].(bool); ok {
		s.Enabled = &v
	} else if s.Enabled == nil {
		t := true
		s.Enabled = &t
	}
	return s
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '\n'
	}) {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func anyToStrs(v []any) []string {
	out := make([]string, 0, len(v))
	for _, x := range v {
		out = append(out, fmt.Sprint(x))
	}
	return out
}
