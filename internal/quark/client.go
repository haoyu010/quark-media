package quark

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	baseM  = "https://drive-m.quark.cn"
	basePC = "https://drive-pc.quark.cn"
	uaM    = "Mozilla/5.0 (Linux; Android 13) AppleWebKit/537.36 (KHTML, like Gecko) Quark/7.4.5 Mobile Safari/537.36"
	uaPC   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) quark-cloud-drive/3.19.0 Chrome/112.0.5615.165 Safari/537.36"
)

type Client struct {
	Cookie string
	MParam map[string]string
	http   *http.Client
	cache  map[string]cacheItem
	pathC  map[string]string
	mu     sync.Mutex
}

type cacheItem struct {
	URL string
	Exp time.Time
}

type FileItem struct {
	FID      string
	Name     string
	Path     string
	Size     int64
	IsDir    bool
	Raw      map[string]any
}

type Video struct {
	FID  string `json:"fid"`
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func New(cookie, mURL string) *Client {
	c := &Client{
		Cookie: strings.TrimSpace(cookie),
		MParam: map[string]string{},
		http:   &http.Client{Timeout: 45 * time.Second},
		cache:  map[string]cacheItem{},
		pathC:  map[string]string{},
	}
	for k, v := range mparamFromCookie(c.Cookie) {
		c.MParam[k] = v
	}
	for k, v := range mparamFromURL(mURL) {
		c.MParam[k] = v
	}
	if _, ok := c.MParam["pr"]; !ok {
		c.MParam["pr"] = "ucpro"
	}
	return c
}

func (c *Client) SetCookie(cookie string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Cookie = strings.TrimSpace(cookie)
	for k, v := range mparamFromCookie(c.Cookie) {
		c.MParam[k] = v
	}
	c.pathC = map[string]string{}
}

func (c *Client) SetMURL(mURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, v := range mparamFromURL(mURL) {
		c.MParam[k] = v
	}
}

func (c *Client) Ready() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Cookie != "" && c.MParam["kps"] != "" && c.MParam["sign"] != "" && c.MParam["vcode"] != ""
}

func (c *Client) CookieOK() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.Cookie) > 40
}

func (c *Client) MParamOK() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.MParam["kps"] != "" && c.MParam["sign"] != "" && c.MParam["vcode"] != ""
}

func (c *Client) reqPC(method, apiPath string, params url.Values, body any) (map[string]any, error) {
	if params == nil {
		params = url.Values{}
	}
	if params.Get("pr") == "" {
		params.Set("pr", "ucpro")
	}
	if params.Get("fr") == "" {
		params.Set("fr", "pc")
	}
	if !params.Has("uc_param_str") {
		params.Set("uc_param_str", "")
	}
	var rdr io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	}
	u := basePC + apiPath + "?" + params.Encode()
	req, err := http.NewRequest(method, u, rdr)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	cookie := c.Cookie
	c.mu.Unlock()
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", uaPC)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", "https://pan.quark.cn")
	req.Header.Set("Referer", "https://pan.quark.cn/")
	if body != nil {
		req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var js map[string]any
	if err := json.Unmarshal(b, &js); err != nil {
		return nil, fmt.Errorf("json: %v raw=%s", err, truncate(string(b), 200))
	}
	if resp.StatusCode >= 400 {
		return js, fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(b), 200))
	}
	return js, nil
}

func isDir(it map[string]any) bool {
	if v, ok := it["dir"]; ok {
		switch t := v.(type) {
		case bool:
			return t
		case float64:
			return t != 0
		case string:
			return t == "true" || t == "1"
		}
	}
	switch t := it["file_type"].(type) {
	case float64:
		return int(t) == 0
	case string:
		return t == "0"
	}
	return false
}

func (c *Client) LS(pdirFID string, pageSize int) ([]map[string]any, error) {
	if pdirFID == "" {
		pdirFID = "0"
	}
	if pageSize <= 0 {
		pageSize = 100
	}
	var out []map[string]any
	for page := 1; page <= 200; page++ {
		params := url.Values{}
		params.Set("pdir_fid", pdirFID)
		params.Set("_page", fmt.Sprint(page))
		params.Set("_size", fmt.Sprint(pageSize))
		params.Set("_fetch_total", "1")
		params.Set("_fetch_sub_dirs", "0")
		params.Set("_sort", "file_type:asc,updated_at:desc")
		js, err := c.reqPC(http.MethodGet, "/1/clouddrive/file/sort", params, nil)
		if err != nil {
			return out, err
		}
		data, _ := js["data"].(map[string]any)
		list, _ := data["list"].([]any)
		if len(list) == 0 {
			break
		}
		for _, it := range list {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		if len(list) < pageSize {
			break
		}
	}
	return out, nil
}


// ListDirs lists only directories under pdirFID.
func (c *Client) ListDirs(pdirFID string) ([]map[string]string, error) {
	items, err := c.LS(pdirFID, 100)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]string, 0)
	for _, it := range items {
		if !isDir(it) {
			continue
		}
		fid := fmt.Sprint(it["fid"])
		name := fmt.Sprint(it["file_name"])
		if fid == "" || name == "" || name == "<nil>" {
			continue
		}
		out = append(out, map[string]string{"fid": fid, "name": name})
	}
	return out, nil
}

func (c *Client) PathToFID(p string) (string, error) {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" {
		return "0", nil
	}
	c.mu.Lock()
	if fid, ok := c.pathC[p]; ok {
		c.mu.Unlock()
		return fid, nil
	}
	c.mu.Unlock()

	cur := "0"
	built := []string{}
	for _, part := range strings.Split(p, "/") {
		if part == "" {
			continue
		}
		built = append(built, part)
		key := strings.Join(built, "/")
		c.mu.Lock()
		if fid, ok := c.pathC[key]; ok {
			cur = fid
			c.mu.Unlock()
			continue
		}
		c.mu.Unlock()
		items, err := c.LS(cur, 100)
		if err != nil {
			return "", err
		}
		found := ""
		for _, it := range items {
			if fmt.Sprint(it["file_name"]) == part {
				found = fmt.Sprint(it["fid"])
				break
			}
		}
		if found == "" {
			return "", fmt.Errorf("路径不存在: %s", key)
		}
		c.mu.Lock()
		c.pathC[key] = found
		c.mu.Unlock()
		cur = found
	}
	return cur, nil
}

func (c *Client) WalkVideos(rootPath string, videoExts []string, maxDepth int) ([]Video, error) {
	rootPath = strings.Trim(strings.TrimSpace(rootPath), "/")
	if maxDepth <= 0 {
		maxDepth = 12
	}
	exts := map[string]bool{}
	for _, e := range videoExts {
		e = strings.ToLower(e)
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		exts[e] = true
	}
	// single file
	if rootPath != "" {
		low := strings.ToLower(rootPath)
		for e := range exts {
			if strings.HasSuffix(low, e) {
				parent := path.Dir(rootPath)
				if parent == "." {
					parent = ""
				}
				name := path.Base(rootPath)
				parentFID := "0"
				var err error
				if parent != "" {
					parentFID, err = c.PathToFID(parent)
					if err != nil {
						return nil, err
					}
				}
				items, err := c.LS(parentFID, 100)
				if err != nil {
					return nil, err
				}
				for _, it := range items {
					if fmt.Sprint(it["file_name"]) == name && !isDir(it) {
						sz, _ := it["size"].(float64)
						return []Video{{FID: fmt.Sprint(it["fid"]), Name: name, Path: rootPath, Size: int64(sz)}}, nil
					}
				}
				return nil, fmt.Errorf("文件不存在: %s", rootPath)
			}
		}
	}
	rootFID, err := c.PathToFID(rootPath)
	if err != nil {
		return nil, err
	}
	var out []Video
	var rec func(fid, prefix string, depth int) error
	rec = func(fid, prefix string, depth int) error {
		if depth > maxDepth {
			return nil
		}
		items, err := c.LS(fid, 100)
		if err != nil {
			return err
		}
		for _, it := range items {
			name := fmt.Sprint(it["file_name"])
			rel := name
			if prefix != "" {
				rel = prefix + "/" + name
			}
			if isDir(it) {
				if err := rec(fmt.Sprint(it["fid"]), rel, depth+1); err != nil {
					return err
				}
				continue
			}
			low := strings.ToLower(name)
			ok := false
			for e := range exts {
				if strings.HasSuffix(low, e) {
					ok = true
					break
				}
			}
			if ok {
				sz, _ := it["size"].(float64)
				out = append(out, Video{FID: fmt.Sprint(it["fid"]), Name: name, Path: rel, Size: int64(sz)})
			}
		}
		return nil
	}
	if err := rec(rootFID, rootPath, 0); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) GetPlayURL(fid string) (string, error) {
	c.mu.Lock()
	if c.Cookie == "" || c.MParam["kps"] == "" || c.MParam["sign"] == "" || c.MParam["vcode"] == "" {
		c.mu.Unlock()
		return "", fmt.Errorf("缺少 cookie 或 m_url 签名(kps/sign/vcode)")
	}
	if it, ok := c.cache[fid]; ok && time.Now().Before(it.Exp) {
		u := it.URL
		c.mu.Unlock()
		return u, nil
	}
	params := url.Values{}
	for k, v := range c.MParam {
		params.Set(k, v)
	}
	cookie := c.Cookie
	c.mu.Unlock()

	body := map[string]any{
		"fid":         fid,
		"resolutions": "low,normal,high,super,2k,4k",
		"supports":    "m3u8,dolby_vision",
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, baseM+"/1/clouddrive/file/v2/play/project?"+params.Encode(), bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", uaM)
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("Origin", "https://pan.quark.cn")
	req.Header.Set("Referer", "https://pan.quark.cn/")
	req.Header.Set("Accept", "application/json, text/plain, */*")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var js map[string]any
	if err := json.Unmarshal(b, &js); err != nil {
		return "", fmt.Errorf("play json: %v raw=%s", err, truncate(string(b), 200))
	}
	u := pickPlayURL(js)
	if u == "" {
		return "", fmt.Errorf("无转码地址: %s", truncate(string(b), 300))
	}
	c.mu.Lock()
	c.cache[fid] = cacheItem{URL: u, Exp: time.Now().Add(10 * time.Minute)}
	c.mu.Unlock()
	return u, nil
}

func (c *Client) ParseShare(shareURL string) (pwdID, passcode string) {
	re := regexp.MustCompile(`(?:pan\.quark\.cn/s/|quark\.cn/s/)([a-zA-Z0-9]+)`)
	if m := re.FindStringSubmatch(shareURL); len(m) > 1 {
		pwdID = m[1]
	}
	if i := strings.Index(shareURL, "pwd="); i >= 0 {
		passcode = shareURL[i+4:]
		if j := strings.IndexAny(passcode, "&#"); j >= 0 {
			passcode = passcode[:j]
		}
	}
	return
}

func (c *Client) GetSToken(pwdID, passcode string) (string, error) {
	js, err := c.reqPC(http.MethodPost, "/1/clouddrive/share/sharepage/token", nil, map[string]any{
		"pwd_id": pwdID, "passcode": passcode,
	})
	if err != nil {
		return "", err
	}
	data, _ := js["data"].(map[string]any)
	st, _ := data["stoken"].(string)
	if st == "" {
		return "", fmt.Errorf("获取 stoken 失败: %v", js["message"])
	}
	return st, nil
}

func (c *Client) ShareList(pwdID, stoken, pdirFID string) ([]map[string]any, error) {
	if pdirFID == "" {
		pdirFID = "0"
	}
	params := url.Values{}
	params.Set("pwd_id", pwdID)
	params.Set("stoken", stoken)
	params.Set("pdir_fid", pdirFID)
	params.Set("force", "0")
	params.Set("_page", "1")
	params.Set("_size", "200")
	params.Set("_fetch_banner", "0")
	params.Set("_fetch_share", "0")
	js, err := c.reqPC(http.MethodGet, "/1/clouddrive/share/sharepage/detail", params, nil)
	if err != nil {
		return nil, err
	}
	data, _ := js["data"].(map[string]any)
	list, _ := data["list"].([]any)
	var out []map[string]any
	for _, it := range list {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

func (c *Client) MkdirPath(p string) (string, error) {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" {
		return "0", nil
	}
	// try resolve existing
	if fid, err := c.PathToFID(p); err == nil {
		return fid, nil
	}
	js, err := c.reqPC(http.MethodPost, "/1/clouddrive/file", nil, map[string]any{
		"pdir_fid":  "0",
		"file_name": "",
		"dir_path":  "/" + p,
		"dir_init_lock": false,
	})
	if err != nil {
		return "", err
	}
	// clear cache and resolve
	c.mu.Lock()
	c.pathC = map[string]string{}
	c.mu.Unlock()
	_ = js
	return c.PathToFID(p)
}

func (c *Client) WaitTask(taskID string, timeoutSec int) error {
	if taskID == "" {
		return nil
	}
	if timeoutSec <= 0 {
		timeoutSec = 120
	}
	end := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	for time.Now().Before(end) {
		params := url.Values{}
		params.Set("task_id", taskID)
		params.Set("retry_index", "0")
		params.Set("__dt", fmt.Sprint(rand.Intn(4*60*1000)+60*1000))
		params.Set("__t", fmt.Sprint(time.Now().UnixMilli()))
		js, err := c.reqPC(http.MethodGet, "/1/clouddrive/task", params, nil)
		if err != nil {
			return err
		}
		data, _ := js["data"].(map[string]any)
		// status 2 = done commonly
		st := fmt.Sprint(data["status"])
		if st == "2" || st == "3" || data["finished"] == true {
			return nil
		}
		time.Sleep(1500 * time.Millisecond)
	}
	return fmt.Errorf("任务超时: %s", taskID)
}

func (c *Client) SaveShare(shareURL, savePath, passcode string) (map[string]any, error) {
	pwdID, pc := c.ParseShare(shareURL)
	if passcode != "" {
		pc = passcode
	}
	if pwdID == "" {
		return nil, fmt.Errorf("无效分享链接")
	}
	stoken, err := c.GetSToken(pwdID, pc)
	if err != nil {
		return nil, err
	}
	files, err := c.ShareList(pwdID, stoken, "0")
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return map[string]any{"ok": false, "message": "分享为空", "count": 0}, nil
	}
	fidList := make([]string, 0, len(files))
	tokenList := make([]string, 0, len(files))
	for _, f := range files {
		fidList = append(fidList, fmt.Sprint(f["fid"]))
		tok := fmt.Sprint(f["share_fid_token"])
		if tok == "" || tok == "<nil>" {
			tok = fmt.Sprint(f["fid_token"])
		}
		if tok == "<nil>" {
			tok = ""
		}
		tokenList = append(tokenList, tok)
	}
	toFID, err := c.MkdirPath(savePath)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("app", "clouddrive")
	params.Set("__dt", fmt.Sprint(rand.Intn(4*60*1000)+60*1000))
	params.Set("__t", fmt.Sprint(time.Now().UnixMilli()))
	js, err := c.reqPC(http.MethodPost, "/1/clouddrive/share/sharepage/save", params, map[string]any{
		"fid_list":       fidList,
		"fid_token_list": tokenList,
		"to_pdir_fid":    toFID,
		"pwd_id":         pwdID,
		"stoken":         stoken,
		"pdir_fid":       "0",
		"scene":          "link",
	})
	if err != nil {
		return nil, err
	}
	data, _ := js["data"].(map[string]any)
	taskID := fmt.Sprint(data["task_id"])
	if taskID != "" && taskID != "<nil>" {
		_ = c.WaitTask(taskID, 120)
	}
	return map[string]any{"ok": true, "count": len(fidList), "to_pdir_fid": toFID, "response_status": js["status"]}, nil
}

func (c *Client) Probe() bool {
	return c.CookieOK()
}

func pickPlayURL(js map[string]any) string {
	data, _ := js["data"].(map[string]any)
	if data == nil {
		return ""
	}
	var list []any
	if v, ok := data["video_list"].([]any); ok {
		list = v
	} else if v, ok := data["list"].([]any); ok {
		list = v
	}
	order := []string{"super", "high", "normal", "low", "2k", "4k"}
	for _, want := range order {
		for _, item := range list {
			m, _ := item.(map[string]any)
			if m == nil {
				continue
			}
			res := strings.ToLower(fmt.Sprint(m["resolution"]))
			if strings.Contains(res, want) {
				info, _ := m["video_info"].(map[string]any)
				if info == nil {
					info = m
				}
				if u, ok := info["url"].(string); ok && u != "" {
					return u
				}
			}
		}
	}
	for _, item := range list {
		m, _ := item.(map[string]any)
		if m == nil {
			continue
		}
		info, _ := m["video_info"].(map[string]any)
		if info == nil {
			info = m
		}
		if u, ok := info["url"].(string); ok && u != "" {
			return u
		}
	}
	return ""
}

func mparamFromCookie(cookie string) map[string]string {
	out := map[string]string{}
	for _, k := range []string{"kps", "sign", "vcode", "ut"} {
		re := regexp.MustCompile(`(?i)(?:^|[;&\s])` + k + `=([a-zA-Z0-9%+/=]+)`)
		if m := re.FindStringSubmatch(cookie); len(m) > 1 {
			out[k] = strings.ReplaceAll(m[1], "%25", "%")
		}
	}
	return out
}

func mparamFromURL(mURL string) map[string]string {
	out := map[string]string{}
	if mURL == "" {
		return out
	}
	re := regexp.MustCompile(`https://drive-m\.quark\.cn/[^\s'\"<>]+`)
	if m := re.FindString(mURL); m != "" {
		mURL = m
	}
	u, err := url.Parse(mURL)
	if err != nil {
		return out
	}
	q := u.Query()
	keep := map[string]bool{"kps": true, "sign": true, "vcode": true, "ut": true, "pr": true, "fr": true, "uc_param_str": true, "entry": true}
	for k, vs := range q {
		if keep[k] || strings.HasPrefix(k, "uc_") {
			if len(vs) > 0 {
				out[k] = vs[len(vs)-1]
			}
		}
	}
	if out["pr"] == "" {
		out["pr"] = "ucpro"
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
