package qas

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Extras struct {
	TMDBAPIKey     string
	PushNotifyType string
	PushConfig     map[string]any
	TelegramSource map[string]any
	TaskSettings   map[string]any
	Cookies        []string
}

func LoadExtras(path string) Extras {
	ex := Extras{
		PushNotifyType: "full",
		PushConfig:     map[string]any{},
		TelegramSource: map[string]any{},
		TaskSettings:   map[string]any{},
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ex
	}
	var raw map[string]any
	if json.Unmarshal(b, &raw) != nil {
		return ex
	}
	if v, ok := raw["tmdb_api_key"].(string); ok {
		ex.TMDBAPIKey = v
	}
	if v, ok := raw["push_notify_type"].(string); ok {
		ex.PushNotifyType = v
	}
	if v, ok := raw["push_config"].(map[string]any); ok {
		ex.PushConfig = v
	}
	if v, ok := raw["telegram_source"].(map[string]any); ok {
		ex.TelegramSource = v
	}
	if v, ok := raw["task_settings"].(map[string]any); ok {
		ex.TaskSettings = v
	}
	switch v := raw["cookie"].(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			ex.Cookies = []string{v}
		}
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				ex.Cookies = append(ex.Cookies, s)
			}
		}
	}
	return ex
}

func SaveExtrasMerge(path string, patch map[string]any) (Extras, error) {
	raw := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &raw)
	}
	for k, v := range patch {
		if v == nil {
			continue
		}
		if nm, ok := v.(map[string]any); ok {
			old, _ := raw[k].(map[string]any)
			if old == nil {
				old = map[string]any{}
			}
			for kk, vv := range nm {
				if ss, ok := vv.(string); ok && ss == "" {
					continue
				}
				old[kk] = vv
			}
			raw[k] = old
			continue
		}
		if ss, ok := v.(string); ok && ss == "" && k == "tmdb_api_key" {
			continue
		}
		raw[k] = v
	}
	if err := writeJSONAtomic(path, raw); err != nil {
		return Extras{}, err
	}
	return LoadExtras(path), nil
}

func ListTasks(path string) []map[string]any {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw map[string]any
	if json.Unmarshal(b, &raw) != nil {
		return nil
	}
	var arr []any
	switch v := raw["tasklist"].(type) {
	case []any:
		arr = v
	case map[string]any:
		for name, item := range v {
			m, _ := item.(map[string]any)
			if m == nil {
				m = map[string]any{}
			}
			m2 := map[string]any{}
			for k, vv := range m {
				m2[k] = vv
			}
			if _, ok := m2["name"]; !ok {
				m2["name"] = name
			}
			arr = append(arr, m2)
		}
	}
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		name := asStr(m["taskname"])
		if name == "" {
			name = asStr(m["name"])
		}
		save := asStr(m["savepath"])
		if save == "" {
			save = asStr(m["save_path"])
		}
		share := asStr(m["shareurl"])
		if share == "" {
			share = asStr(m["share_url"])
		}
		out = append(out, map[string]any{
			"name":        name,
			"save_path":   save,
			"quark_path":  save,
			"share_url":   share,
			"passcode":    asStr(m["passcode"]),
			"strm_subdir": asStr(m["strm_subdir"]),
			"enabled":     m["enabled"] != false,
			"do_save":     share != "",
			"source":      "qas",
		})
	}
	return out
}

func writeJSONAtomic(path string, raw map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o664)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Chmod(path, 0o664)
	return nil
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

func maskKeep(s string, keep int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if keep < 0 {
		keep = 0
	}
	if len(s) <= keep*2 {
		return "****"
	}
	return s[:keep] + "****" + s[len(s)-keep:]
}

func PublicExtras(ex Extras) map[string]any {
	pc := map[string]any{}
	for k, v := range ex.PushConfig {
		pc[k] = v
	}
	if tok, ok := pc["TG_BOT_TOKEN"].(string); ok && tok != "" {
		pc["TG_BOT_TOKEN_masked"] = maskKeep(tok, 4)
		pc["TG_BOT_TOKEN"] = ""
		pc["TG_BOT_TOKEN_SET"] = true
		pc["TG_BOT_TOKEN_set"] = true // compat frontend
	} else {
		// also accept non-string
		if tok2 := asStr(pc["TG_BOT_TOKEN"]); tok2 != "" {
			pc["TG_BOT_TOKEN_masked"] = maskKeep(tok2, 4)
			pc["TG_BOT_TOKEN"] = ""
			pc["TG_BOT_TOKEN_SET"] = true
			pc["TG_BOT_TOKEN_set"] = true
		} else {
			pc["TG_BOT_TOKEN_SET"] = false
			pc["TG_BOT_TOKEN_set"] = false
		}
	}
	// normalize user id to string for UI
	if v, ok := pc["TG_USER_ID"]; ok && v != nil {
		pc["TG_USER_ID"] = asStr(v)
	}
	return map[string]any{
		"tmdb_api_key":        "",
		"tmdb_set":            ex.TMDBAPIKey != "",
		"tmdb_api_key_set":    ex.TMDBAPIKey != "",
		"tmdb_api_key_masked": maskKeep(ex.TMDBAPIKey, 4),
		"push_notify_type": ex.PushNotifyType,
		"push_config":      pc,
		"telegram_source":  ex.TelegramSource,
		"task_settings":    ex.TaskSettings,
		"cookies_count":    len(ex.Cookies),
	}
}


// UpsertTask creates or updates a QAS tasklist item by name.
func UpsertTask(path, name, savePath, shareURL, passcode string) error {
	raw := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &raw)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = strings.TrimSpace(savePath)
	}
	if name == "" {
		name = "sub-task"
	}
	item := map[string]any{
		"taskname":  name,
		"shareurl":  strings.TrimSpace(shareURL),
		"savepath":  strings.TrimSpace(savePath),
		"pattern":   "",
		"replace":   "",
		"enddate":   "",
		"emby_id":   "",
		"ignore_extension": false,
		"runweek":   []int{1, 2, 3, 4, 5, 6, 7},
	}
	if strings.TrimSpace(passcode) != "" {
		item["shareurl"] = strings.TrimSpace(shareURL)
	}

	list := []any{}
	switch v := raw["tasklist"].(type) {
	case []any:
		list = v
	case map[string]any:
		for n, it := range v {
			m, _ := it.(map[string]any)
			if m == nil {
				m = map[string]any{}
			}
			if asStr(m["taskname"]) == "" && asStr(m["name"]) == "" {
				m["taskname"] = n
			}
			list = append(list, m)
		}
	}
	found := false
	for i, it := range list {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		n := asStr(m["taskname"])
		if n == "" {
			n = asStr(m["name"])
		}
		if n == name {
			m["taskname"] = name
			m["shareurl"] = strings.TrimSpace(shareURL)
			m["savepath"] = strings.TrimSpace(savePath)
			list[i] = m
			found = true
			break
		}
	}
	if !found {
		list = append(list, item)
	}
	raw["tasklist"] = list
	return writeJSONAtomic(path, raw)
}
