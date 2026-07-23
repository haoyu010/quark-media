package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	"quark-media/internal/channel"
	"quark-media/internal/config"
	"quark-media/internal/emby"
	"quark-media/internal/pipeline"
	"quark-media/internal/qas"
	"quark-media/internal/quark"
	"quark-media/internal/store"
	"quark-media/internal/strm"
	"quark-media/internal/subs"
	"quark-media/internal/tmdb"
)

type App struct {
	Cfg       *config.Config
	Client    *quark.Client
	Log       *store.Logger
	mu        sync.Mutex
	lastRun   int64
	lastResult map[string]any
	mtpRunning bool
	tgRunning  bool
	activeAcc  int
}

func Listen(addr string, cfg *config.Config, client *quark.Client) error {
	app := &App{Cfg: cfg, Client: client, Log: store.NewLogger(500)}
	app.Log.Add("Quark Media (Go) started")
	mux := http.NewServeMux()
	app.routes(mux)
	if cfg.Interval > 0 {
		go func() {
			t := time.NewTicker(time.Duration(cfg.Interval) * time.Second)
			for range t.C {
				app.mu.Lock()
				res := pipeline.Run(app.Cfg, app.Client, app.Log)
				app.lastRun = time.Now().Unix()
				app.lastResult = res
				app.mu.Unlock()
			}
		}()
	}
	return http.ListenAndServe(addr, withCORS(mux))
}

func RunPipeline(cfg *config.Config, client *quark.Client) (int, error) {
	log := store.NewLogger(100)
	res := pipeline.Run(cfg, client, log)
	n, _ := res["total_videos"].(int)
	return n, nil
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) routes(mux *http.ServeMux) {
	webRoot := findWebRoot()
	fs := http.FileServer(http.Dir(webRoot))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/play/") {
			http.NotFound(w, r)
			return
		}
		p := filepath.Join(webRoot, filepath.Clean("/"+r.URL.Path))
		if r.URL.Path == "/" || !fileExists(p) {
			http.ServeFile(w, r, filepath.Join(webRoot, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})
	mux.HandleFunc("/play/", a.handlePlay)
	mux.HandleFunc("/api/status", a.handleStatus)
	mux.HandleFunc("/api/tasks", a.handleTasks)
	mux.HandleFunc("/api/tasks/", a.handleTaskItem)
	mux.HandleFunc("/api/strm", a.handleStrm)
	mux.HandleFunc("/api/settings", a.handleSettings)
	mux.HandleFunc("/api/config", a.handleSettings)
	mux.HandleFunc("/api/logs", a.handleLogs)
	mux.HandleFunc("/api/logs/clear", a.handleLogsClear)
	mux.HandleFunc("/api/test-play", a.handleTestPlay)
	mux.HandleFunc("/api/pipeline/run", a.handlePipeline)
	mux.HandleFunc("/api/subscriptions", a.handleSubs)
	mux.HandleFunc("/api/subscriptions/", a.handleSubItem)
	mux.HandleFunc("/api/subscriptions/refresh-channels", a.handleSubRefresh)
	mux.HandleFunc("/api/category", a.handleCategory)
	mux.HandleFunc("/api/accounts", a.handleAccounts)
	mux.HandleFunc("/api/accounts/active", a.handleAccountsActive)
	mux.HandleFunc("/api/accounts/test", a.handleAccountsTest)
	mux.HandleFunc("/api/emby/folders", a.handleEmbyFolders)
	mux.HandleFunc("/api/emby/refresh", a.handleEmbyRefresh)
	mux.HandleFunc("/api/tg-inbox", a.handleTgInbox)
	mux.HandleFunc("/api/tg-inbox/", a.handleTgInboxAction)
	mux.HandleFunc("/api/tmdb/discover", a.handleTMDBDiscover)
	mux.HandleFunc("/api/tmdb/search", a.handleTMDBSearch)
	mux.HandleFunc("/api/tmdb/subscribe", a.handleTMDBSubscribe)
	mux.HandleFunc("/api/channel/status", a.handleChannelStatus)
	mux.HandleFunc("/api/channel/index", a.handleChannelIndex)
	mux.HandleFunc("/api/channel/search", a.handleChannelSearch)
	mux.HandleFunc("/api/mtproto/status", a.handleMtpStatus)
	mux.HandleFunc("/api/mtproto/config", a.handleMtpConfig)
	mux.HandleFunc("/api/mtproto/start", a.handleMtpStart)
	mux.HandleFunc("/api/mtproto/stop", a.handleMtpStop)
	mux.HandleFunc("/api/mtproto/restart", a.handleMtpRestart)
	mux.HandleFunc("/api/mtproto/send-code", a.handleMtpSendCode)
	mux.HandleFunc("/api/mtproto/sign-in", a.handleMtpSignIn)
	mux.HandleFunc("/api/mtproto/logout", a.handleMtpLogout)
}

func findWebRoot() string {
	for _, c := range []string{"web", "/app/web"} {
		if fileExists(filepath.Join(c, "index.html")) {
			return c
		}
	}
	return "web"
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request) (map[string]any, error) {
	b, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func asStr(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		s := strings.TrimSpace(fmt.Sprint(t))
		if s == "<nil>" {
			return ""
		}
		return s
	}
}

func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		return n, err == nil
	default:
		return 0, false
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func readVersion() string {
	b, err := os.ReadFile("VERSION")
	if err != nil {
		return "0.0.0"
	}
	return strings.TrimSpace(string(b))
}

func (a *App) handlePlay(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/play/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing fid", 400)
		return
	}
	fid := parts[0]
	url, err := a.Client.GetPlayURL(fid)
	if err != nil {
		a.Log.Add("play fail " + fid + " " + err.Error())
		http.Error(w, err.Error(), 502)
		return
	}
	a.Log.Add("302 " + fid[:min(12, len(fid))] + "...")
	w.Header().Set("Location", url)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusFound)
}

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	tasks := pipeline.CollectTasks(a.Cfg)
	items, _ := strm.List(a.Cfg.StrmRoot)
	embyConfigured := a.Cfg.Emby.Enabled && a.Cfg.Emby.APIKey != ""
	embyOK := false
	if embyConfigured {
		embyOK = emby.New(a.Cfg.Emby.BaseURL, a.Cfg.Emby.APIKey).Ping()
	}
	ex := qas.LoadExtras(a.Cfg.QASConfig)
	tgEnabled := false
	if pc := ex.PushConfig; pc != nil {
		if v, ok := pc["TG_ENABLED"].(bool); ok {
			tgEnabled = v
		}
	}
	hasToken := asStr(ex.PushConfig["TG_BOT_TOKEN"]) != ""
	a.mu.Lock()
	lastRun := a.lastRun
	lastResult := a.lastResult
	a.mu.Unlock()
	writeJSON(w, 200, map[string]any{
		"ok": true, "engine": "go", "version": readVersion(),
		"port": a.Cfg.Server.Port, "public_base": a.Cfg.Server.PublicBase,
		"quark_ok": a.Client.CookieOK(), "mparam_ok": a.Client.MParamOK(),
		"task_count": len(tasks), "strm_count": len(items),
		"subscription_count": len(a.Cfg.Subscriptions),
		"emby_configured": embyConfigured, "emby_ok": embyOK,
		"last_run": lastRun, "last_result": lastResult,
		"tg_inbox": map[string]any{
			"enabled": tgEnabled && hasToken, "running": a.tgRunning,
			"has_token": hasToken, "has_user": asStr(ex.PushConfig["TG_USER_ID"]) != "",
			"inbox_root": asStr(ex.TaskSettings["inbox_root"]),
		},
		"mtproto": map[string]any{"enabled": a.Cfg.Mtproto.Enabled, "running": a.mtpRunning},
	})
}

func (a *App) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tasks := make([]map[string]any, 0)
		for i, t := range a.Cfg.Tasks {
			en := true
			if t.Enabled != nil {
				en = *t.Enabled
			}
			tasks = append(tasks, map[string]any{
				"id": fmt.Sprintf("task:%d", i), "name": t.Name, "save_path": t.SavePath,
				"share_url": t.ShareURL, "passcode": t.Passcode, "pattern": t.Pattern,
				"replace": t.Replace, "strm_subdir": t.StrmSubdir, "enabled": en, "source": "config",
			})
		}
		writeJSON(w, 200, map[string]any{"ok": true, "tasks": tasks, "all": pipeline.CollectTasks(a.Cfg), "items": tasks})
	case http.MethodPost:
		body, err := readJSON(r)
		if err != nil {
			writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		t := config.Task{
			Name: asStr(body["name"]), SavePath: asStr(body["save_path"]),
			ShareURL: asStr(body["share_url"]), Passcode: asStr(body["passcode"]),
			Pattern: asStr(body["pattern"]), Replace: asStr(body["replace"]),
			StrmSubdir: asStr(body["strm_subdir"]),
		}
		en := true
		if v, ok := body["enabled"].(bool); ok {
			en = v
		}
		t.Enabled = &en
		a.Cfg.Tasks = append(a.Cfg.Tasks, t)
		if err := a.Cfg.Save(); err != nil {
			writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "id": fmt.Sprintf("task:%d", len(a.Cfg.Tasks)-1)})
	default:
		writeJSON(w, 405, map[string]any{"ok": false, "error": "method"})
	}
}

func (a *App) handleTaskItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	id = strings.Trim(id, "/")
	isDelete := strings.HasSuffix(id, "/delete") || r.Method == http.MethodDelete
	isUpdate := strings.HasSuffix(id, "/update") || r.Method == http.MethodPut || r.Method == http.MethodPost
	id = strings.TrimSuffix(id, "/delete")
	id = strings.TrimSuffix(id, "/update")
	id = strings.Trim(id, "/")
	var idx int
	if _, err := fmt.Sscanf(id, "task:%d", &idx); err != nil || idx < 0 || idx >= len(a.Cfg.Tasks) {
		writeJSON(w, 404, map[string]any{"ok": false, "error": "task not found"})
		return
	}
	if isDelete {
		a.Cfg.Tasks = append(a.Cfg.Tasks[:idx], a.Cfg.Tasks[idx+1:]...)
		_ = a.Cfg.Save()
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	if isUpdate {
		body, err := readJSON(r)
		if err != nil {
			writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		t := a.Cfg.Tasks[idx]
		if v := asStr(body["name"]); v != "" {
			t.Name = v
		}
		if _, ok := body["save_path"]; ok {
			t.SavePath = asStr(body["save_path"])
		}
		if _, ok := body["share_url"]; ok {
			t.ShareURL = asStr(body["share_url"])
		}
		if _, ok := body["passcode"]; ok {
			t.Passcode = asStr(body["passcode"])
		}
		if _, ok := body["strm_subdir"]; ok {
			t.StrmSubdir = asStr(body["strm_subdir"])
		}
		if v, ok := body["enabled"].(bool); ok {
			t.Enabled = &v
		}
		a.Cfg.Tasks[idx] = t
		_ = a.Cfg.Save()
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	writeJSON(w, 405, map[string]any{"ok": false})
}

func (a *App) handleStrm(w http.ResponseWriter, r *http.Request) {
	items, _ := strm.List(a.Cfg.StrmRoot)
	writeJSON(w, 200, map[string]any{"ok": true, "root": a.Cfg.StrmRoot, "items": items})
}

func (a *App) handleLogs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "logs": a.Log.List()})
}

func (a *App) handleLogsClear(w http.ResponseWriter, r *http.Request) {
	a.Log.Clear()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleTestPlay(w http.ResponseWriter, r *http.Request) {
	fid := strings.TrimSpace(r.URL.Query().Get("fid"))
	if fid == "" {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "missing fid"})
		return
	}
	url, err := a.Client.GetPlayURL(fid)
	if err != nil {
		a.Log.Add("test-play fail " + err.Error())
		writeJSON(w, 502, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	a.Log.Add("test-play ok " + fid[:min(12, len(fid))])
	writeJSON(w, 200, map[string]any{
		"ok": true, "fid": fid, "url": url, "is_m3u8": strings.Contains(url, "m3u8"),
		"proxy_url": strings.TrimRight(a.Cfg.Server.PublicBase, "/") + "/play/" + fid + "/test.mp4",
	})
}

func (a *App) handlePipeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"ok": false})
		return
	}
	res := pipeline.Run(a.Cfg, a.Client, a.Log)
	a.mu.Lock()
	a.lastRun = time.Now().Unix()
	a.lastResult = res
	a.mu.Unlock()
	writeJSON(w, 200, res)
}

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		view := a.Cfg.SettingsPublic()
		ex := qas.LoadExtras(a.Cfg.QASConfig)
		view["qas_extras"] = qas.PublicExtras(ex)
		view["tmdb_api_key"] = ""
		view["tmdb_set"] = ex.TMDBAPIKey != ""
		writeJSON(w, 200, view)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"ok": false})
		return
	}
	body, err := readJSON(r)
	if err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if v := asStr(body["cookie"]); v != "" {
		a.Cfg.Cookie = v
		a.Client.SetCookie(v)
	}
	if v := asStr(body["m_url"]); v != "" {
		a.Cfg.MURL = v
		a.Client.SetMURL(v)
	}
	if _, ok := body["m_url_file"]; ok {
		a.Cfg.MURLFile = asStr(body["m_url_file"])
	}
	if _, ok := body["openlist_db"]; ok {
		a.Cfg.OpenListDB = asStr(body["openlist_db"])
	}
	if v, ok := body["use_qas_transfer"].(bool); ok {
		a.Cfg.UseQASTransfer = v
	}
	if v, ok := body["import_qas_tasks"].(bool); ok {
		a.Cfg.ImportQASTasks = v
	}
	if v, ok := body["qas_write_back"].(bool); ok {
		a.Cfg.QASWriteBack = v
	}
	if _, ok := body["qas_root"]; ok {
		a.Cfg.QASRoot = asStr(body["qas_root"])
	}
	if _, ok := body["qas_config"]; ok {
		a.Cfg.QASConfig = asStr(body["qas_config"])
	}
	if _, ok := body["strm_root"]; ok {
		a.Cfg.StrmRoot = asStr(body["strm_root"])
	}
	if _, ok := body["category_file"]; ok {
		a.Cfg.CategoryFile = asStr(body["category_file"])
	}
	if n, ok := asInt(body["interval_seconds"]); ok {
		a.Cfg.Interval = n
	}
	if v := body["video_exts"]; v != nil {
		switch t := v.(type) {
		case string:
			parts := strings.FieldsFunc(t, func(r rune) bool { return r == ',' || r == ' ' || r == ';' })
			var exts []string
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				if !strings.HasPrefix(p, ".") {
					p = "." + p
				}
				exts = append(exts, p)
			}
			if len(exts) > 0 {
				a.Cfg.VideoExts = exts
			}
		case []any:
			var exts []string
			for _, x := range t {
				p := asStr(x)
				if p == "" {
					continue
				}
				if !strings.HasPrefix(p, ".") {
					p = "." + p
				}
				exts = append(exts, p)
			}
			if len(exts) > 0 {
				a.Cfg.VideoExts = exts
			}
		}
	}
	if s, ok := body["server"].(map[string]any); ok {
		if _, ok := s["host"]; ok {
			a.Cfg.Server.Host = asStr(s["host"])
		}
		if n, ok := asInt(s["port"]); ok {
			a.Cfg.Server.Port = n
		}
		if _, ok := s["public_base"]; ok {
			a.Cfg.Server.PublicBase = asStr(s["public_base"])
		}
	}
	if e, ok := body["emby"].(map[string]any); ok {
		if v, ok := e["enabled"].(bool); ok {
			a.Cfg.Emby.Enabled = v
		}
		if _, ok := e["base_url"]; ok {
			a.Cfg.Emby.BaseURL = asStr(e["base_url"])
		}
		if v := asStr(e["api_key"]); v != "" {
			a.Cfg.Emby.APIKey = v
		}
		if _, ok := e["path"]; ok {
			a.Cfg.Emby.Path = asStr(e["path"])
		}
	}
	qasPatch := map[string]any{}
	if qx, ok := body["qas_extras"].(map[string]any); ok {
		for k, v := range qx {
			qasPatch[k] = v
		}
	}
	for _, k := range []string{"tmdb_api_key", "push_notify_type", "push_config", "telegram_source", "task_settings"} {
		if v, ok := body[k]; ok {
			qasPatch[k] = v
		}
	}
	if cookiesText := asStr(qasPatch["cookies_text"]); cookiesText != "" {
		lines := strings.Split(cookiesText, "\n")
		var cookies []string
		for _, ln := range lines {
			ln = strings.TrimSpace(ln)
			if ln != "" {
				cookies = append(cookies, ln)
			}
		}
		if len(cookies) > 0 {
			a.Cfg.Cookie = cookies[0]
			a.Client.SetCookie(cookies[0])
			a.Cfg.Accounts = cookies
			qasPatch["cookie"] = cookies
		}
		delete(qasPatch, "cookies_text")
	}
	if len(qasPatch) > 0 {
		ex, err := qas.SaveExtrasMerge(a.Cfg.QASConfig, qasPatch)
		if err != nil {
			a.Log.Add("save qas extras: " + err.Error())
		} else if ex.TMDBAPIKey != "" {
			a.Cfg.TMDBAPIKey = ex.TMDBAPIKey
		}
	}
	if v := asStr(body["tmdb_api_key"]); v != "" {
		a.Cfg.TMDBAPIKey = v
		_, _ = qas.SaveExtrasMerge(a.Cfg.QASConfig, map[string]any{"tmdb_api_key": v})
	}
	if err := a.Cfg.Save(); err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	view := a.Cfg.SettingsPublic()
	view["qas_extras"] = qas.PublicExtras(qas.LoadExtras(a.Cfg.QASConfig))
	writeJSON(w, 200, map[string]any{"ok": true, "settings": view})
}

func channelList(ex qas.Extras, cfg *config.Config) []string {
	var chs []string
	if v, ok := ex.TelegramSource["channels"].(string); ok {
		for _, p := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' }) {
			p = strings.TrimSpace(p)
			if p != "" {
				chs = append(chs, p)
			}
		}
	}
	if v, ok := ex.TelegramSource["channels"].([]any); ok {
		for _, x := range v {
			chs = append(chs, asStr(x))
		}
	}
	chs = append(chs, cfg.Mtproto.Channels...)
	return chs
}

func (a *App) handleSubs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, map[string]any{"ok": true, "items": subs.List(a.Cfg), "subscriptions": subs.List(a.Cfg)})
	case http.MethodPost:
		body, err := readJSON(r)
		if err != nil {
			writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		v, err := subs.Create(a.Cfg, body)
		if err != nil {
			writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "item": v})
	default:
		writeJSON(w, 405, map[string]any{"ok": false})
	}
}

func (a *App) handleSubItem(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/subscriptions/")
	if path == "refresh-channels" {
		a.handleSubRefresh(w, r)
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, 404, map[string]any{"ok": false})
		return
	}
	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	if action == "search" && r.Method == http.MethodGet {
		var idx int
		if _, err := fmt.Sscanf(id, "sub:%d", &idx); err != nil || idx < 0 || idx >= len(a.Cfg.Subscriptions) {
			writeJSON(w, 404, map[string]any{"ok": false, "error": "not found"})
			return
		}
		s := a.Cfg.Subscriptions[idx]
		ex := qas.LoadExtras(a.Cfg.QASConfig)
		chs := channelList(ex, a.Cfg)
		kws := s.Keywords
		if len(kws) == 0 {
			kws = []string{s.Name}
		}
		hits, err := channel.SearchPublic(chs, kws, 40)
		if err != nil {
			writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		items := make([]map[string]any, 0, len(hits))
		for _, h := range hits {
			items = append(items, map[string]any{"url": h.URL, "share_url": h.URL, "text": h.Text, "channel": h.Channel, "title": h.Text})
		}
		writeJSON(w, 200, map[string]any{"ok": true, "items": items, "results": items})
		return
	}
	if action == "apply" && r.Method == http.MethodPost {
		body, _ := readJSON(r)
		share := asStr(body["share_url"])
		if share == "" {
			share = asStr(body["url"])
		}
		var idx int
		if _, err := fmt.Sscanf(id, "sub:%d", &idx); err != nil || idx < 0 || idx >= len(a.Cfg.Subscriptions) {
			writeJSON(w, 404, map[string]any{"ok": false})
			return
		}
		a.Cfg.Subscriptions[idx].ShareURL = share
		_ = a.Cfg.Save()
		if a.Cfg.Subscriptions[idx].SavePath != "" && share != "" {
			_, err := a.Client.SaveShare(share, a.Cfg.Subscriptions[idx].SavePath, "")
			if err != nil {
				writeJSON(w, 200, map[string]any{"ok": true, "share_url": share, "save_error": err.Error()})
				return
			}
		}
		writeJSON(w, 200, map[string]any{"ok": true, "share_url": share})
		return
	}
	if action == "delete" || r.Method == http.MethodDelete {
		if err := subs.Delete(a.Cfg, id); err != nil {
			writeJSON(w, 404, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	if r.Method == http.MethodPut || r.Method == http.MethodPost || action == "update" {
		body, err := readJSON(r)
		if err != nil {
			writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		v, err := subs.Update(a.Cfg, id, body)
		if err != nil {
			writeJSON(w, 404, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "item": v})
		return
	}
	writeJSON(w, 405, map[string]any{"ok": false})
}

func (a *App) handleSubRefresh(w http.ResponseWriter, r *http.Request) {
	ex := qas.LoadExtras(a.Cfg.QASConfig)
	chs := channelList(ex, a.Cfg)
	updated := 0
	for i, s := range a.Cfg.Subscriptions {
		if s.ShareURL != "" && a.Cfg.SubSearch.OnlyMissingShare {
			continue
		}
		kws := s.Keywords
		if len(kws) == 0 {
			kws = []string{s.Name}
		}
		hits, _ := channel.SearchPublic(chs, kws, 5)
		if len(hits) == 0 {
			continue
		}
		a.Cfg.Subscriptions[i].ShareURL = hits[0].URL
		updated++
	}
	_ = a.Cfg.Save()
	writeJSON(w, 200, map[string]any{"ok": true, "updated": updated})
}

func (a *App) handleCategory(w http.ResponseWriter, r *http.Request) {
	path := a.Cfg.CategoryFile
	b, err := os.ReadFile(path)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": true, "path": path, "rules": map[string]any{}, "raw": ""})
		return
	}
	var rules any
	_ = yaml.Unmarshal(b, &rules)
	writeJSON(w, 200, map[string]any{"ok": true, "path": path, "rules": rules, "raw": string(b)})
}

func (a *App) handleAccounts(w http.ResponseWriter, r *http.Request) {
	list := []map[string]any{}
	cookies := a.Cfg.Accounts
	if len(cookies) == 0 && a.Cfg.Cookie != "" {
		cookies = []string{a.Cfg.Cookie}
	}
	for i, c := range cookies {
		list = append(list, map[string]any{
			"index": i, "active": i == a.activeAcc,
			"cookie_set": c != "", "cookie_masked": config.MaskSecret(c, 6),
		})
	}
	writeJSON(w, 200, map[string]any{"ok": true, "count": len(list), "active_index": a.activeAcc, "accounts": list})
}

func (a *App) handleAccountsActive(w http.ResponseWriter, r *http.Request) {
	body, _ := readJSON(r)
	idx, _ := asInt(body["index"])
	cookies := a.Cfg.Accounts
	if len(cookies) == 0 && a.Cfg.Cookie != "" {
		cookies = []string{a.Cfg.Cookie}
	}
	if idx < 0 || idx >= len(cookies) {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "bad index"})
		return
	}
	a.activeAcc = idx
	a.Cfg.Cookie = cookies[idx]
	a.Client.SetCookie(cookies[idx])
	_ = a.Cfg.Save()
	writeJSON(w, 200, map[string]any{"ok": true, "active_index": idx})
}

func (a *App) handleAccountsTest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": a.Client.CookieOK(), "quark_ok": a.Client.CookieOK(), "mparam_ok": a.Client.MParamOK()})
}

func (a *App) handleEmbyFolders(w http.ResponseWriter, r *http.Request) {
	if !a.Cfg.Emby.Enabled || a.Cfg.Emby.APIKey == "" {
		writeJSON(w, 200, map[string]any{"ok": false, "error": "emby not configured", "folders": []any{}})
		return
	}
	ec := emby.New(a.Cfg.Emby.BaseURL, a.Cfg.Emby.APIKey)
	v, err := ec.FoldersRaw()
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error(), "folders": []any{}})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "folders": v})
}

func (a *App) handleEmbyRefresh(w http.ResponseWriter, r *http.Request) {
	body, _ := readJSON(r)
	id := asStr(body["id"])
	if id == "" {
		id = asStr(body["item_id"])
	}
	ec := emby.New(a.Cfg.Emby.BaseURL, a.Cfg.Emby.APIKey)
	if err := ec.Refresh(id); err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleTgInbox(w http.ResponseWriter, r *http.Request) {
	ex := qas.LoadExtras(a.Cfg.QASConfig)
	hasToken := asStr(ex.PushConfig["TG_BOT_TOKEN"]) != ""
	hasUser := asStr(ex.PushConfig["TG_USER_ID"]) != ""
	enabled := false
	if v, ok := ex.PushConfig["TG_ENABLED"].(bool); ok {
		enabled = v
	}
	var missing []string
	if !hasToken {
		missing = append(missing, "TG_BOT_TOKEN")
	}
	if !hasUser {
		missing = append(missing, "TG_USER_ID")
	}
	writeJSON(w, 200, map[string]any{
		"ok": true, "enabled": enabled && hasToken, "running": a.tgRunning,
		"has_token": hasToken, "has_user": hasUser,
		"inbox_root": asStr(ex.TaskSettings["inbox_root"]), "missing": missing,
		"note": "Go core ready; bot inbox worker next",
	})
}

func (a *App) handleTgInboxAction(w http.ResponseWriter, r *http.Request) {
	act := strings.TrimPrefix(r.URL.Path, "/api/tg-inbox/")
	switch act {
	case "start", "restart":
		a.tgRunning = true
	case "stop":
		a.tgRunning = false
	case "test":
		writeJSON(w, 200, map[string]any{"ok": true, "message": "TG bot test stub"})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "running": a.tgRunning})
}

func (a *App) tmdbClient() *tmdb.Client {
	key := a.Cfg.TMDBAPIKey
	if key == "" {
		key = qas.LoadExtras(a.Cfg.QASConfig).TMDBAPIKey
	}
	return tmdb.New(key)
}

func (a *App) handleTMDBDiscover(w http.ResponseWriter, r *http.Request) {
	tab := r.URL.Query().Get("tab")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	res, err := a.tmdbClient().Discover(tab, page)
	if err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, res)
}

func (a *App) handleTMDBSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	typ := r.URL.Query().Get("type")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	res, err := a.tmdbClient().Search(q, typ, page)
	if err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, res)
}

func (a *App) handleTMDBSubscribe(w http.ResponseWriter, r *http.Request) {
	body, err := readJSON(r)
	if err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if asStr(body["name"]) == "" {
		body["name"] = asStr(body["title"])
	}
	if asStr(body["content_type"]) == "" {
		body["content_type"] = asStr(body["media_type"])
	}
	if asStr(body["tmdb_id"]) == "" {
		body["tmdb_id"] = asStr(body["id"])
	}
	if asStr(body["save_path"]) == "" {
		ct := asStr(body["content_type"])
		if ct == "" {
			ct = "tv"
		}
		body["save_path"] = ct + "/" + asStr(body["name"])
	}
	v, err := subs.Create(a.Cfg, body)
	if err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if a.Cfg.SubSearch.Enabled {
		ex := qas.LoadExtras(a.Cfg.QASConfig)
		chs := channelList(ex, a.Cfg)
		kws := []string{asStr(body["name"])}
		hits, _ := channel.SearchPublic(chs, kws, 5)
		if len(hits) > 0 && a.Cfg.SubSearch.ApplyBest {
			i := len(a.Cfg.Subscriptions) - 1
			if i >= 0 {
				a.Cfg.Subscriptions[i].ShareURL = hits[0].URL
				_ = a.Cfg.Save()
				v.ShareURL = hits[0].URL
			}
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true, "item": v, "subscription": v})
}

func (a *App) handleChannelStatus(w http.ResponseWriter, r *http.Request) {
	ex := qas.LoadExtras(a.Cfg.QASConfig)
	chs := channelList(ex, a.Cfg)
	writeJSON(w, 200, map[string]any{"ok": true, "channels": chs, "count": len(chs), "mode": "public_scrape"})
}

func (a *App) handleChannelIndex(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "message": "index noop"})
}

func (a *App) handleChannelSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	ex := qas.LoadExtras(a.Cfg.QASConfig)
	chs := channelList(ex, a.Cfg)
	var kws []string
	if q != "" {
		kws = append(kws, q)
	}
	hits, err := channel.SearchPublic(chs, kws, 40)
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	items := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		items = append(items, map[string]any{"url": h.URL, "share_url": h.URL, "text": h.Text, "channel": h.Channel})
	}
	writeJSON(w, 200, map[string]any{"ok": true, "items": items})
}

func (a *App) handleMtpStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"ok": true, "enabled": a.Cfg.Mtproto.Enabled, "running": a.mtpRunning,
		"api_id": a.Cfg.Mtproto.APIID, "api_hash_set": a.Cfg.Mtproto.APIHash != "",
		"phone": a.Cfg.Mtproto.Phone, "channels": a.Cfg.Mtproto.Channels,
		"session_path": a.Cfg.Mtproto.SessionPath, "authorized": false,
		"note": "gotd MTProto next patch",
	})
}

func (a *App) handleMtpConfig(w http.ResponseWriter, r *http.Request) {
	body, _ := readJSON(r)
	if _, ok := body["api_id"]; ok {
		a.Cfg.Mtproto.APIID = asStr(body["api_id"])
	}
	if v := asStr(body["api_hash"]); v != "" {
		a.Cfg.Mtproto.APIHash = v
	}
	if _, ok := body["phone"]; ok {
		a.Cfg.Mtproto.Phone = asStr(body["phone"])
	}
	if v, ok := body["enabled"].(bool); ok {
		a.Cfg.Mtproto.Enabled = v
	}
	if _, ok := body["session_path"]; ok {
		a.Cfg.Mtproto.SessionPath = asStr(body["session_path"])
	}
	if v, ok := body["auto_apply"].(bool); ok {
		a.Cfg.Mtproto.AutoApply = v
	}
	if v, ok := body["also_qas_task"].(bool); ok {
		a.Cfg.Mtproto.AlsoQASTask = v
	}
	if ch, ok := body["channels"].(string); ok {
		var arr []string
		for _, p := range strings.FieldsFunc(ch, func(r rune) bool { return r == ',' || r == '\n' }) {
			p = strings.TrimSpace(p)
			if p != "" {
				arr = append(arr, p)
			}
		}
		a.Cfg.Mtproto.Channels = arr
	}
	if ch, ok := body["channels"].([]any); ok {
		var arr []string
		for _, x := range ch {
			arr = append(arr, asStr(x))
		}
		a.Cfg.Mtproto.Channels = arr
	}
	_ = a.Cfg.Save()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleMtpStart(w http.ResponseWriter, r *http.Request) {
	a.mtpRunning = a.Cfg.Mtproto.Enabled
	writeJSON(w, 200, map[string]any{"ok": true, "running": a.mtpRunning})
}
func (a *App) handleMtpStop(w http.ResponseWriter, r *http.Request) {
	a.mtpRunning = false
	writeJSON(w, 200, map[string]any{"ok": true, "running": false})
}
func (a *App) handleMtpRestart(w http.ResponseWriter, r *http.Request) {
	a.mtpRunning = a.Cfg.Mtproto.Enabled
	writeJSON(w, 200, map[string]any{"ok": true, "running": a.mtpRunning})
}
func (a *App) handleMtpSendCode(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": false, "error": "gotd login next version"})
}
func (a *App) handleMtpSignIn(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": false, "error": "gotd login next version"})
}
func (a *App) handleMtpLogout(w http.ResponseWriter, r *http.Request) {
	a.mtpRunning = false
	writeJSON(w, 200, map[string]any{"ok": true})
}
