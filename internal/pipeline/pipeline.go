package pipeline

import (
	"fmt"
	"path/filepath"
	"strings"

	"quark-media/internal/config"
	"quark-media/internal/emby"
	"quark-media/internal/qas"
	"quark-media/internal/quark"
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
		if save == "" && share == "" {
			continue
		}
		name := t.Name
		if name == "" {
			name = save
			if name == "" {
				name = share
			}
		}
		doSave := share != ""
		if t.DoSave != nil {
			doSave = *t.DoSave
		}
		tasks = append(tasks, map[string]any{
			"name": name, "save_path": save, "quark_path": save, "share_url": share,
			"passcode": t.Passcode, "strm_subdir": strings.Trim(t.StrmSubdir, "/"),
			"enabled": true, "do_save": doSave, "source": "config",
		})
	}
	if cfg.UseQASTransfer || cfg.ImportQASTasks {
		for _, t := range qas.ListTasks(cfg.QASConfig) {
			tasks = append(tasks, t)
		}
	}
	for i, s := range cfg.Subscriptions {
		if s.Enabled != nil && !*s.Enabled {
			continue
		}
		save := strings.TrimSpace(s.SavePath)
		if save == "" && s.ShareURL == "" {
			continue
		}
		name := s.Name
		if name == "" {
			name = save
		}
		tasks = append(tasks, map[string]any{
			"name": name, "save_path": save, "quark_path": save, "share_url": s.ShareURL,
			"strm_subdir": strings.Trim(s.StrmSubdir, "/"), "enabled": true,
			"do_save": s.ShareURL != "", "source": "subscription", "sub_id": i,
		})
	}
	seen := map[string]bool{}
	var uniq []map[string]any
	for _, t := range tasks {
		key := strings.Trim(asStr(t["save_path"]), "/")
		if key == "" {
			key = asStr(t["share_url"])
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		uniq = append(uniq, t)
	}
	return uniq
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

func Run(cfg *config.Config, client *quark.Client, log *store.Logger) map[string]any {
	if log != nil {
		log.Add("pipeline start")
	}
	tasks := CollectTasks(cfg)
	results := make([]map[string]any, 0, len(tasks))
	total := 0
	for _, t := range tasks {
		name := asStr(t["name"])
		qpath := strings.Trim(asStr(t["save_path"]), "/")
		item := map[string]any{"task": name, "path": qpath, "source": t["source"]}
		if log != nil {
			log.Add("STRM " + name + " path=" + qpath)
		}
		if qpath == "" {
			item["error"] = "missing save_path"
			results = append(results, item)
			continue
		}
		if t["do_save"] == true && asStr(t["share_url"]) != "" {
			res, err := client.SaveShare(asStr(t["share_url"]), qpath, asStr(t["passcode"]))
			if err != nil {
				item["save_error"] = err.Error()
				if log != nil {
					log.Add("save_error " + err.Error())
				}
			} else {
				item["save"] = res
			}
		}
		videos, err := client.WalkVideos(qpath, cfg.VideoExts, 12)
		if err != nil {
			item["error"] = err.Error()
			results = append(results, item)
			if log != nil {
				log.Add("walk_error " + err.Error())
			}
			continue
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
			item["error"] = err.Error()
		} else {
			item["videos"] = len(videos)
			item["created"] = created
			item["updated"] = updated
			item["skipped"] = skipped
			total += len(videos)
		}
		results = append(results, item)
		if log != nil {
			log.Add(fmt.Sprintf("videos=%d", len(videos)))
		}
	}
	result := map[string]any{
		"ok": true, "tasks": results, "total_videos": total, "emby": nil,
	}
	if cfg.Emby.Enabled && cfg.Emby.APIKey != "" {
		ec := emby.New(cfg.Emby.BaseURL, cfg.Emby.APIKey)
		if err := ec.Refresh(""); err != nil {
			result["emby"] = map[string]any{"ok": false, "error": err.Error()}
		} else {
			result["emby"] = map[string]any{"ok": true}
		}
	}
	if log != nil {
		log.Add(fmt.Sprintf("pipeline done videos=%d", total))
	}
	return result
}
