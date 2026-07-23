package emby

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	APIKey  string
	http    *http.Client
}

func New(base, key string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(strings.TrimSpace(base), "/"),
		APIKey:  strings.TrimSpace(key),
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) Configured() bool {
	return c.BaseURL != "" && c.APIKey != ""
}

func (c *Client) Ping() bool {
	if !c.Configured() {
		return false
	}
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/System/Info/Public", nil)
	if err != nil {
		return false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 400
}

func (c *Client) FoldersRaw() (any, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/Library/VirtualFolders", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Emby-Token", c.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return v, fmt.Errorf("emby %d", resp.StatusCode)
	}
	return v, nil
}

func (c *Client) Refresh(itemID string) error {
	path := "/Library/Refresh"
	if itemID != "" {
		path = "/Items/" + itemID + "/Refresh?Recursive=true&MetadataRefreshMode=Default&ImageRefreshMode=Default"
	}
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Emby-Token", c.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		msg := string(b)
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return fmt.Errorf("refresh %d: %s", resp.StatusCode, msg)
	}
	return nil
}
