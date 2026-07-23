package emby

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	BaseURL   string
	APIKey    string
	MediaRoot string // Emby library path for STRM root (host/NAS), not container path
	http      *http.Client
}

type Folder struct {
	Name           string   `json:"name"`
	ItemID         string   `json:"item_id"`
	Locations      []string `json:"locations"`
	CollectionType string   `json:"collection_type"`
}

type RefreshResult struct {
	OK      bool     `json:"ok"`
	Mode    string   `json:"mode"`
	Paths   []string `json:"paths,omitempty"`
	ItemIDs []string `json:"item_ids,omitempty"`
	Error   string   `json:"error,omitempty"`
	Detail  any      `json:"detail,omitempty"`
}

func New(base, key string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(strings.TrimSpace(base), "/"),
		APIKey:  strings.TrimSpace(key),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) WithMediaRoot(p string) *Client {
	c.MediaRoot = strings.TrimSpace(p)
	return c
}

func (c *Client) Configured() bool {
	return c.BaseURL != "" && c.APIKey != ""
}

func (c *Client) do(method, p string, body any) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.BaseURL+p, rdr)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("X-Emby-Token", c.APIKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b, resp.StatusCode, nil
}

func (c *Client) Ping() bool {
	if !c.Configured() {
		return false
	}
	_, code, err := c.do(http.MethodGet, "/System/Info/Public", nil)
	return err == nil && code < 400
}

func (c *Client) Folders() ([]Folder, error) {
	b, code, err := c.do(http.MethodGet, "/Library/VirtualFolders", nil)
	if err != nil {
		return nil, err
	}
	if code >= 400 {
		return nil, fmt.Errorf("emby folders %d: %s", code, truncate(string(b), 200))
	}
	var arr []map[string]any
	if err := json.Unmarshal(b, &arr); err != nil {
		return nil, err
	}
	out := make([]Folder, 0, len(arr))
	for _, f := range arr {
		fd := Folder{
			Name:           asStr(f["Name"]),
			ItemID:         firstStr(f, "ItemId", "Guid", "ItemID"),
			CollectionType: asStr(f["CollectionType"]),
		}
		if fd.Name == "" {
			fd.Name = asStr(f["name"])
		}
		if locs, ok := f["Locations"].([]any); ok {
			for _, x := range locs {
				fd.Locations = append(fd.Locations, asStr(x))
			}
		}
		out = append(out, fd)
	}
	return out, nil
}

func (c *Client) FoldersRaw() (any, error) {
	b, code, err := c.do(http.MethodGet, "/Library/VirtualFolders", nil)
	if err != nil {
		return nil, err
	}
	var v any
	_ = json.Unmarshal(b, &v)
	if code >= 400 {
		return v, fmt.Errorf("emby %d", code)
	}
	return v, nil
}

// MapToEmbyPath maps container strm path to Emby-visible path via MediaRoot.
func (c *Client) MapToEmbyPath(strmRoot, p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	strmRoot = strings.TrimRight(strings.ReplaceAll(strmRoot, "\\", "/"), "/")
	rel := p
	if strmRoot != "" && (p == strmRoot || strings.HasPrefix(p, strmRoot+"/")) {
		rel = strings.TrimPrefix(p, strmRoot)
		rel = strings.TrimPrefix(rel, "/")
	}
	if c.MediaRoot == "" {
		return p
	}
	root := strings.TrimRight(strings.ReplaceAll(c.MediaRoot, "\\", "/"), "/")
	if rel == "" {
		return root
	}
	return root + "/" + rel
}

// RefreshPaths only notifies given paths. Never full-library refresh.
func (c *Client) RefreshPaths(paths []string) RefreshResult {
	res := RefreshResult{Mode: "path"}
	if !c.Configured() {
		res.Error = "emby not configured"
		return res
	}
	seen := map[string]bool{}
	var uniq []string
	for _, p := range paths {
		p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		uniq = append(uniq, p)
	}
	if len(uniq) == 0 {
		res.Error = "no paths"
		return res
	}
	res.Paths = uniq

	updates := make([]map[string]any, 0, len(uniq))
	for _, p := range uniq {
		updates = append(updates, map[string]any{"Path": p, "UpdateType": "Created"})
	}
	body := map[string]any{"Updates": updates}
	b, code, err := c.do(http.MethodPost, "/Library/Media/Updated", body)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if code < 400 {
		res.OK = true
		res.Mode = "media_updated"
		res.Detail = map[string]any{"status": code, "text": truncate(string(b), 120)}
		ids := c.matchFolderIDs(uniq)
		if len(ids) > 0 {
			res.ItemIDs = ids
			for _, id := range ids {
				_ = c.RefreshItem(id)
			}
			res.Mode = "media_updated+folder_item"
		}
		return res
	}

	// fallback: only matching virtual folder ItemId (still not full library)
	ids := c.matchFolderIDs(uniq)
	if len(ids) == 0 {
		res.Error = fmt.Sprintf("Media/Updated %d, no matching library: %s", code, truncate(string(b), 120))
		return res
	}
	res.ItemIDs = ids
	var lastErr error
	for _, id := range ids {
		if err := c.RefreshItem(id); err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		res.Error = lastErr.Error()
		return res
	}
	res.OK = true
	res.Mode = "folder_item"
	return res
}

func (c *Client) matchFolderIDs(paths []string) []string {
	folders, err := c.Folders()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var ids []string
	for _, p := range paths {
		pl := strings.ToLower(strings.ReplaceAll(p, "\\", "/"))
		for _, f := range folders {
			if f.ItemID == "" || seen[f.ItemID] {
				continue
			}
			for _, loc := range f.Locations {
				ll := strings.ToLower(strings.TrimRight(strings.ReplaceAll(loc, "\\", "/"), "/"))
				if pl == ll || strings.HasPrefix(pl, ll+"/") || strings.HasPrefix(ll, pl+"/") {
					seen[f.ItemID] = true
					ids = append(ids, f.ItemID)
					break
				}
				if c.MediaRoot != "" {
					mr := strings.ToLower(strings.TrimRight(strings.ReplaceAll(c.MediaRoot, "\\", "/"), "/"))
					if strings.HasPrefix(pl, mr) && (ll == mr || strings.HasPrefix(ll, mr+"/") || strings.HasPrefix(mr, ll+"/")) {
						seen[f.ItemID] = true
						ids = append(ids, f.ItemID)
						break
					}
				}
			}
		}
	}
	return ids
}

func (c *Client) RefreshItem(itemID string) error {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return fmt.Errorf("empty item id")
	}
	q := url.Values{}
	q.Set("Recursive", "true")
	q.Set("MetadataRefreshMode", "Default")
	q.Set("ImageRefreshMode", "Default")
	q.Set("ReplaceAllMetadata", "false")
	q.Set("ReplaceAllImages", "false")
	_, code, err := c.do(http.MethodPost, "/Items/"+url.PathEscape(itemID)+"/Refresh?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	if code >= 400 {
		return fmt.Errorf("item refresh %d", code)
	}
	return nil
}

// Refresh rejects full-library; require item id.
func (c *Client) Refresh(itemID string) error {
	if strings.TrimSpace(itemID) == "" {
		return fmt.Errorf("refuse full-library refresh: pass item_id or use RefreshPaths")
	}
	return c.RefreshItem(itemID)
}

func JoinMedia(root, rel string) string {
	root = strings.TrimRight(strings.ReplaceAll(root, "\\", "/"), "/")
	rel = strings.Trim(strings.ReplaceAll(rel, "\\", "/"), "/")
	if root == "" {
		return rel
	}
	if rel == "" {
		return root
	}
	return root + "/" + rel
}

func asStr(v any) string {
	if v == nil {
		return ""
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "<nil>" {
		return ""
	}
	return s
}

func firstStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := asStr(m[k]); s != "" {
			return s
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
