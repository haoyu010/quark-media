package pipeline

import (
	"fmt"
	"path/filepath"
	"strings"

	"quark-media/internal/config"
	"quark-media/internal/emby"
	"quark-media/internal/qas"
	"quark-media/internal/quark"
	"quark-media/internal/replace"
	"quark-media/internal/store"
	"quark-media/internal/strm"
)

func CollectTasks(cfg *config.Config) []map[string]any {
	var tasks []map[string]any
	for _, t := range cfg.Tasks {
		if t.Enabled != nil && !*t.Enabled {
			continue
		}
		save := strings.TrimSpace(t.SavePath)
		share := strings.TrimSpace(t.ShareURL)
		if share == "" {
			continue
		}
		if save == "" {
			save = "转存/" + strings.TrimSpace(t.Name)
			if save == "转存/" {
				save = "转存"
			}
		}
		name := t.Name
		if name == "" {
			name = save
		}
		tasks = append(tasks, map[string]any{
			"name": name, "save_path": save, "quark_path": save, "share_url": share,
			"passcode": t.Passcode, "strm_subdir": strings.Trim(t.StrmSubdir, "/"),
			"enabled": true, "source": "config",
		})
	}
	if cfg.UseQASTransfer || cfg.ImportQASTasks {
		for _, t := range qas.ListTasks(cfg.QASConfig) {
			if asStr(t["share_url"]) == "" {
				continue
			}
			tasks = append(tasks, t)
		}
	}
	for i, s := range cfg.Subscriptions {
		if s.Enabled != nil && !*s.Enabled {
			continue
		}
		share := strings.TrimSpace(s.ShareURL)
		if share == "" {
			continue
		}
		save := strings.TrimSpace(s.SavePath)
		if save == "" {
			ct := s.ContentType
			if ct == "" {
				ct = "tv"
			}
			save = ct + "/" + s.Name
		}
		name := s.Name
		if name == "" {
			name = save
		}
		tasks = append(tasks, map[string]any{
			"name": name, "save_path": save, "quark_path": save, "share_url": share,
			"strm_subdir": strings.Trim(s.StrmSubdir, "/"), "enabled": true,
			"source": "subscription", "sub_id": i,
		})
	}
	seen := map[string]bool{}
	var uniq []map[string]any
	for _, t := range tasks {
		key := asStr(t["share_url"]) + "|" + strings.Trim(asStr(t["save_path"]), "/")
		if key == "|" || seen[key] {
			continue
		}
		seen[key] = true
		uniq = append(uniq, t)
	}
	return uniq
}

func asBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "1" || s == "true" || s == "yes"
	default:
		return false
	}
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

// RunOne transfers one share, generates STRM, optional Emby path refresh.
func RunOne(cfg *config.Config, client *quark.Client, log *store.Logger, t map[string]any) map[string]any {
	name := asStr(t["name"])
	qpath := strings.Trim(asStr(t["save_path"]), "/")
	if qpath == "" {
		qpath = strings.Trim(asStr(t["quark_path"]), "/")
	}
	share := asStr(t["share_url"])
	item := map[string]any{"task": name, "path": qpath, "share_url": share, "source": t["source"]}
	if share == "" || qpath == "" {
		item["ok"] = false
		item["error"] = "missing share_url or save_path"
		return item
	}
	if log != nil {
		log.Add("transfer " + name + " -> " + qpath)
	}
	res, err := client.SaveShare(share, qpath, asStr(t["passcode"]))
	if err != nil {
		// QAS retry_save_after_auto_replace 1:1
		if replace.IsInvalidShareErr(err) && !asBool(t["_auto_replace_retrying"]) {
			if log != nil {
				log.Add("share invalid, try auto replace: " + err.Error())
			}
			ex := qas.LoadExtras(cfg.QASConfig)
			var chs []string
			if v, ok := ex.TelegramSource["channels"].(string); ok {
				for _, p := range strings.FieldsFunc(v, func(r rune) bool {
					return r == ',' || r == '\n' || r == ';' || r == '\r'
				}) {
					p = strings.TrimSpace(p)
					if p != "" {
						chs = append(chs, p)
					}
				}
			}
			if v, ok := ex.TelegramSource["channels"].([]any); ok {
				for _, x := range v {
					if s := asStr(x); s != "" {
						chs = append(chs, s)
					}
				}
			}
			svc := replace.NewService(cfg.QASConfig, client, chs, func(s string) {
				if log != nil {
					log.Add(s)
				}
			})
			// normalize task keys for QAS
			t["taskname"] = name
			t["shareurl"] = share
			t["savepath"] = qpath
			okRep, newShare, floor, startfid, msg := svc.RetrySaveAfterAutoReplace(t, err.Error())
			item["auto_replace"] = map[string]any{
				"replaced": okRep, "message": msg, "new_shareurl": newShare,
				"saved_episode_floor": floor, "startfid": startfid,
			}
			if okRep && newShare != "" {
				share = newShare
				t["share_url"] = newShare
				t["shareurl"] = newShare
				t["_auto_replace_retrying"] = true
				// save with episode floor filter (QAS filter_share_files_by_saved_episode_floor)
				var keep func(map[string]any) bool
				if floor != nil {
					fval := *floor
					keep = func(f map[string]any) bool {
						// dirs keep
						if b, ok := f["dir"].(bool); ok && b {
							return true
						}
						ep := replace.EpisodeFromFileInfo(f)
						return ep == nil || *ep > fval
					}
				}
				var err2 error
				if keep != nil {
					res, err2 = client.SaveShareFiltered(share, qpath, asStr(t["passcode"]), keep)
				} else {
					res, err2 = client.SaveShare(share, qpath, asStr(t["passcode"]))
				}
				if err2 == nil {
					err = nil
					item["share_url"] = share
					if log != nil {
						log.Add("auto replace retry save ok")
					}
				} else {
					err = err2
				}
				delete(t, "_auto_replace_retrying")
			} else if log != nil {
				log.Add("auto replace skip: " + msg)
			}
		}
		if err != nil {
			item["ok"] = false
			item["error"] = err.Error()
			item["save_error"] = err.Error()
			if log != nil {
				log.Add("save_error " + err.Error())
			}
			return item
		}
	}
	item["save"] = res
	videos, err := client.WalkVideos(qpath, cfg.VideoExts, 12)
	if err != nil {
		item["ok"] = false
		item["error"] = err.Error()
		if log != nil {
			log.Add("list_after_save_error " + err.Error())
		}
		return item
	}
	sub := asStr(t["strm_subdir"])
	outRoot := cfg.StrmRoot
	if sub != "" {
		outRoot = filepath.Join(cfg.StrmRoot, sub)
	}
	sv := make([]strm.Video, 0, len(videos))
	for _, v := range videos {
		sv = append(sv, strm.Video{FID: v.FID, Name: v.Name, Path: v.Path})
	}
	created, updated, skipped, err := strm.Generate(sv, outRoot, cfg.Server.PublicBase, qpath)
	if err != nil {
		item["ok"] = false
		item["error"] = err.Error()
		return item
	}
	item["ok"] = true
	item["videos"] = len(videos)
	item["created"] = created
	item["updated"] = updated
	item["skipped"] = skipped
	item["strm_dir"] = outRoot
	if log != nil {
		log.Add(fmt.Sprintf("strm ok videos=%d created=%d updated=%d", len(videos), created, updated))
	}

	// Emby path-only refresh
	if cfg.Emby.Enabled && cfg.Emby.APIKey != "" && len(videos) > 0 {
		ec := emby.New(cfg.Emby.BaseURL, cfg.Emby.APIKey).WithMediaRoot(cfg.Emby.Path)
		var mapped []string
		seenP := map[string]bool{}
		for _, v := range videos {
			rel := strings.Trim(v.Path, "/")
			if qpath != "" && (rel == qpath || strings.HasPrefix(rel, qpath+"/")) {
				rel = strings.TrimPrefix(rel, qpath)
				rel = strings.Trim(rel, "/")
			}
			dir := outRoot
			if i := strings.LastIndex(rel, "/"); i >= 0 {
				dir = filepath.Join(outRoot, filepath.FromSlash(rel[:i]))
			}
			mp := ec.MapToEmbyPath(cfg.StrmRoot, dir)
			mp = strings.ReplaceAll(mp, "\\", "/")
			if mp != "" && !seenP[mp] {
				seenP[mp] = true
				mapped = append(mapped, mp)
			}
		}
		if len(mapped) == 0 && cfg.Emby.Path != "" {
			mapped = []string{cfg.Emby.Path}
		}
		rr := ec.RefreshPaths(mapped)
		item["emby"] = rr
	}
	return item
}

func Run(cfg *config.Config, client *quark.Client, log *store.Logger) map[string]any {
	if log != nil {
		log.Add("pipeline start (transfer → strm → emby path)")
	}
	tasks := CollectTasks(cfg)
	if len(tasks) == 0 {
		msg := "no transfer tasks (need share_url)"
		if log != nil {
			log.Add(msg)
		}
		return map[string]any{"ok": false, "message": msg, "total_videos": 0, "tasks": []any{}}
	}
	results := make([]map[string]any, 0, len(tasks))
	total := 0
	for _, t := range tasks {
		item := RunOne(cfg, client, log, t)
		if n, ok := item["videos"].(int); ok {
			total += n
		}
		results = append(results, item)
	}
	result := map[string]any{"ok": true, "tasks": results, "total_videos": total, "emby": nil}
	if log != nil {
		log.Add(fmt.Sprintf("pipeline done videos=%d", total))
	}
	return result
}
