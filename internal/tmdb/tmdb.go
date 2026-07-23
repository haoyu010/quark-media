package tmdb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const base = "https://api.themoviedb.org/3"
const img = "https://image.tmdb.org/t/p/w500"

type Client struct {
	Key  string
	Lang string
	http *http.Client
}

func New(key string) *Client {
	return &Client{Key: strings.TrimSpace(key), Lang: "zh-CN", http: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) OK() bool { return c.Key != "" }

func (c *Client) get(path string, q url.Values) (map[string]any, error) {
	if c.Key == "" {
		return nil, fmt.Errorf("missing tmdb api key")
	}
	if q == nil {
		q = url.Values{}
	}
	q.Set("api_key", c.Key)
	if q.Get("language") == "" {
		q.Set("language", c.Lang)
	}
	u := base + path + "?" + q.Encode()
	resp, err := c.http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var js map[string]any
	if err := json.Unmarshal(b, &js); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		msg := string(b)
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return js, fmt.Errorf("tmdb %d: %s", resp.StatusCode, msg)
	}
	return js, nil
}

func posterURL(path any) string {
	s := fmt.Sprint(path)
	if s == "" || s == "<nil>" {
		return ""
	}
	if strings.HasPrefix(s, "http") {
		return s
	}
	return img + s
}

func mapItem(it map[string]any, mediaType string) map[string]any {
	mt := mediaType
	if mt == "" {
		mt = fmt.Sprint(it["media_type"])
	}
	if mt == "" || mt == "<nil>" {
		if _, ok := it["first_air_date"]; ok {
			mt = "tv"
		} else {
			mt = "movie"
		}
	}
	title := fmt.Sprint(it["title"])
	if title == "" || title == "<nil>" {
		title = fmt.Sprint(it["name"])
	}
	date := fmt.Sprint(it["release_date"])
	if date == "" || date == "<nil>" {
		date = fmt.Sprint(it["first_air_date"])
	}
	year := ""
	if len(date) >= 4 {
		year = date[:4]
	}
	return map[string]any{
		"id":           it["id"],
		"tmdb_id":      fmt.Sprint(it["id"]),
		"media_type":   mt,
		"content_type": mt,
		"title":        title,
		"name":         title,
		"overview":     it["overview"],
		"poster_path":  it["poster_path"],
		"poster_url":   posterURL(it["poster_path"]),
		"backdrop_url": posterURL(it["backdrop_path"]),
		"year":         year,
		"vote_average": it["vote_average"],
		"popularity":   it["popularity"],
	}
}

func (c *Client) Discover(tab string, page int) (map[string]any, error) {
	if page <= 0 {
		page = 1
	}
	q := url.Values{}
	q.Set("page", fmt.Sprint(page))
	var path string
	switch tab {
	case "movie", "movies":
		path = "/discover/movie"
		q.Set("sort_by", "popularity.desc")
	case "tv", "shows", "series":
		path = "/discover/tv"
		q.Set("sort_by", "popularity.desc")
	case "trending":
		path = "/trending/all/week"
	default:
		path = "/trending/all/day"
	}
	js, err := c.get(path, q)
	if err != nil {
		return nil, err
	}
	results, _ := js["results"].([]any)
	items := make([]map[string]any, 0, len(results))
	for _, r := range results {
		m, _ := r.(map[string]any)
		if m == nil {
			continue
		}
		mt := ""
		if path == "/discover/movie" {
			mt = "movie"
		} else if path == "/discover/tv" {
			mt = "tv"
		}
		items = append(items, mapItem(m, mt))
	}
	return map[string]any{
		"ok": true, "page": js["page"], "total_pages": js["total_pages"],
		"total_results": js["total_results"], "items": items, "results": items,
	}, nil
}

func (c *Client) Search(query string, mediaType string, page int) (map[string]any, error) {
	if page <= 0 {
		page = 1
	}
	q := url.Values{}
	q.Set("query", query)
	q.Set("page", fmt.Sprint(page))
	q.Set("include_adult", "false")
	path := "/search/multi"
	switch mediaType {
	case "movie":
		path = "/search/movie"
	case "tv":
		path = "/search/tv"
	}
	js, err := c.get(path, q)
	if err != nil {
		return nil, err
	}
	results, _ := js["results"].([]any)
	items := make([]map[string]any, 0, len(results))
	for _, r := range results {
		m, _ := r.(map[string]any)
		if m == nil {
			continue
		}
		items = append(items, mapItem(m, mediaType))
	}
	return map[string]any{
		"ok": true, "page": js["page"], "total_pages": js["total_pages"],
		"items": items, "results": items, "query": query,
	}, nil
}
