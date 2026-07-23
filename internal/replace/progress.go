package replace

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// GetAutoReplaceSavedEpisodeFloor ports get_auto_replace_saved_episode_floor.
func GetAutoReplaceSavedEpisodeFloor(task map[string]any) *int {
	if task == nil {
		return nil
	}
	for _, key := range []string{"_auto_replace_saved_episode_floor", "auto_replace_saved_episode_floor"} {
		if n := positiveInt(task[key]); n != nil {
			return n
		}
	}
	return nil
}

// GetSavedEpisodeFloor ports get_saved_episode_floor.
func GetSavedEpisodeFloor(savedFiles []map[string]any, transferRecords []map[string]any) *int {
	var eps []int
	for _, item := range savedFiles {
		if item == nil || isDir(item) {
			continue
		}
		if ep := EpisodeFromFileInfo(item); ep != nil {
			eps = append(eps, *ep)
		}
	}
	for _, rec := range transferRecords {
		if ep := EpisodeFromFileInfo(rec); ep != nil {
			eps = append(eps, *ep)
		}
	}
	if len(eps) == 0 {
		return nil
	}
	max := eps[0]
	for _, e := range eps[1:] {
		if e > max {
			max = e
		}
	}
	return &max
}

// SelectReplacementStartFID ports select_replacement_startfid_by_saved_progress.
func SelectReplacementStartFID(replacementFiles []map[string]any, savedEpisodeFloor any) map[string]any {
	selection := map[string]any{
		"startfid": "", "file_name": "", "episode": nil, "saved_episode_floor": savedEpisodeFloor,
	}
	floorPtr := positiveInt(savedEpisodeFloor)
	if floorPtr == nil {
		return selection
	}
	floor := *floorPtr

	type cand struct {
		item map[string]any
		ep   int
	}
	var candidates []cand
	for _, item := range replacementFiles {
		if item == nil || isDir(item) || asStr(item["fid"]) == "" {
			continue
		}
		ep := EpisodeFromFileInfo(item)
		if ep == nil || *ep <= floor {
			continue
		}
		candidates = append(candidates, cand{item: item, ep: *ep})
	}
	if len(candidates) == 0 {
		return selection
	}

	// prefer earliest update timestamp if available
	hasTS := false
	for _, c := range candidates {
		if fileUpdateTimestamp(c.item) > 0 {
			hasTS = true
			break
		}
	}
	var selected map[string]any
	var selectedEp int
	if hasTS {
		bestI := 0
		bestKey := [3]float64{1e18, 1e18, 0}
		for i, c := range candidates {
			ts := fileUpdateTimestamp(c.item)
			if ts <= 0 {
				ts = 1e18
			}
			key := [3]float64{ts, float64(c.ep), 0}
			if i == 0 || key[0] < bestKey[0] || (key[0] == bestKey[0] && key[1] < bestKey[1]) ||
				(key[0] == bestKey[0] && key[1] == bestKey[1] && asStr(c.item["file_name"]) < asStr(selected["file_name"])) {
				bestI = i
				bestKey = key
				selected = c.item
				selectedEp = c.ep
			}
		}
		_ = bestI
	} else {
		// sort by name reverse, take last (QAS sort_file_by_name reverse then [-1])
		sort.Slice(candidates, func(i, j int) bool {
			return asStr(candidates[i].item["file_name"]) > asStr(candidates[j].item["file_name"])
		})
		// QAS: ordered = sorted(..., reverse=True); selected = ordered[-1]
		// reverse sort then last = lexicographically smallest name among candidates
		sort.Slice(candidates, func(i, j int) bool {
			return asStr(candidates[i].item["file_name"]) < asStr(candidates[j].item["file_name"])
		})
		// actually re-read: ordered reverse=True means descending; ordered[-1] is smallest
		selected = candidates[0].item
		selectedEp = candidates[0].ep
		for _, c := range candidates {
			if asStr(c.item["file_name"]) < asStr(selected["file_name"]) {
				selected = c.item
				selectedEp = c.ep
			}
		}
	}
	if selected == nil {
		return selection
	}
	selection["startfid"] = asStr(selected["fid"])
	selection["file_name"] = fileName(selected)
	selection["episode"] = selectedEp
	return selection
}

// FilterShareFilesBySavedEpisodeFloor ports filter_share_files_by_saved_episode_floor.
func FilterShareFilesBySavedEpisodeFloor(shareFileList []map[string]any, savedEpisodeFloor any) []map[string]any {
	floorPtr := positiveInt(savedEpisodeFloor)
	if floorPtr == nil {
		return shareFileList
	}
	floor := *floorPtr
	var filtered []map[string]any
	for _, item := range shareFileList {
		if item == nil || isDir(item) {
			filtered = append(filtered, item)
			continue
		}
		ep := EpisodeFromFileInfo(item)
		if ep == nil || *ep > floor {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// IsMovieTask ports is_movie_task.
func IsMovieTask(task map[string]any) bool {
	if task == nil {
		return false
	}
	if b, ok := task["movie_once"].(bool); ok && b {
		return true
	}
	return TaskContentType(task) == "movie"
}

// TaskContentType ports task_content_type simplified.
func TaskContentType(task map[string]any) string {
	if task == nil {
		return ""
	}
	for _, k := range []string{"content_type", "media_type", "type"} {
		v := strings.ToLower(asStr(task[k]))
		if v != "" {
			if v == "movies" || v == "film" {
				return "movie"
			}
			return v
		}
	}
	// calendar extracted
	if cal, ok := task["calendar_info"].(map[string]any); ok {
		if ex, ok := cal["extracted"].(map[string]any); ok {
			v := strings.ToLower(asStr(ex["content_type"]))
			if v != "" {
				return v
			}
		}
	}
	return ""
}

// TaskAutoReplaceDisabled ports task_auto_replace_disabled.
func TaskAutoReplaceDisabled(task map[string]any) bool {
	if IsMovieTask(task) {
		return true
	}
	return strings.ToLower(asStr(task["auto_replace_invalid_shareurl"])) == "disabled"
}

// IsCompletedInvalidShareTask ports is_completed_task_for_invalid_share.
func IsCompletedInvalidShareTask(task map[string]any, savedFloor *int) bool {
	if IsMovieTask(task) {
		return true
	}
	return IsTaskCompletedByTMDBEpisodeCount(task, savedFloor)
}

// IsTaskCompletedByTMDBEpisodeCount ports is_task_completed_by_tmdb_episode_count.
func IsTaskCompletedByTMDBEpisodeCount(task map[string]any, savedFloor *int) bool {
	if task == nil {
		return false
	}
	if savedFloor == nil || *savedFloor <= 0 {
		return false
	}
	total := ResolveTaskTMDBTotalEpisodeCount(task)
	if total == nil || *total <= 0 {
		return false
	}
	return *savedFloor >= *total
}

// ResolveTaskTMDBTotalEpisodeCount ports resolve_task_tmdb_total_episode_count (fields/cache only, no live API required).
func ResolveTaskTMDBTotalEpisodeCount(task map[string]any) *int {
	if task == nil {
		return nil
	}
	if n := tmdbTotalFromTaskFields(task); n != nil {
		return n
	}
	return nil
}

func tmdbTotalFromTaskFields(task map[string]any) *int {
	if sc, ok := task["season_counts"].(map[string]any); ok {
		if n := positiveCountFromMapping(sc, "total_count", "episode_count", "total_episode_count"); n != nil {
			return n
		}
	}
	sources := []map[string]any{task}
	if cal, ok := task["calendar_info"].(map[string]any); ok {
		sources = append(sources, cal)
		if ex, ok := cal["extracted"].(map[string]any); ok {
			sources = append(sources, ex)
		}
		if match, ok := cal["match"].(map[string]any); ok {
			sources = append(sources, match)
		}
	}
	for _, src := range sources {
		if n := positiveCountFromMapping(src, "total_count", "episode_count", "total_episode_count", "matched_episode_count"); n != nil {
			return n
		}
	}
	return nil
}

func positiveCountFromMapping(m map[string]any, keys ...string) *int {
	if m == nil {
		return nil
	}
	for _, k := range keys {
		if n := positiveInt(m[k]); n != nil {
			return n
		}
	}
	return nil
}

// GetTaskTMDBID ports get_task_tmdb_id.
func GetTaskTMDBID(task map[string]any) *int {
	if task == nil {
		return nil
	}
	for _, k := range []string{"match_tmdb_id", "tmdb_id", "matched_tmdb_id"} {
		if n := positiveInt(task[k]); n != nil {
			return n
		}
	}
	if cal, ok := task["calendar_info"].(map[string]any); ok {
		if match, ok := cal["match"].(map[string]any); ok {
			if n := positiveInt(match["tmdb_id"]); n != nil {
				return n
			}
			if n := positiveInt(match["id"]); n != nil {
				return n
			}
		}
		if n := positiveInt(cal["tmdb_id"]); n != nil {
			return n
		}
	}
	return nil
}

// GetTaskTMDBSeasonNumber ports get_task_tmdb_season_number.
func GetTaskTMDBSeasonNumber(task map[string]any) *int {
	if task == nil {
		return nil
	}
	for _, k := range []string{"matched_latest_season_number", "latest_season_number", "season_number"} {
		if n := positiveInt(task[k]); n != nil {
			return n
		}
	}
	if cal, ok := task["calendar_info"].(map[string]any); ok {
		if match, ok := cal["match"].(map[string]any); ok {
			for _, k := range []string{"latest_season_number", "season_number", "matched_latest_season_number"} {
				if n := positiveInt(match[k]); n != nil {
					return n
				}
			}
		}
		if ex, ok := cal["extracted"].(map[string]any); ok {
			if n := positiveInt(ex["season_number"]); n != nil {
				return n
			}
		}
	}
	return nil
}

func positiveInt(v any) *int {
	switch t := v.(type) {
	case int:
		if t > 0 {
			return &t
		}
	case int64:
		if t > 0 {
			n := int(t)
			return &n
		}
	case float64:
		if t > 0 {
			n := int(t)
			return &n
		}
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err == nil && n > 0 {
			return &n
		}
	}
	return nil
}

func fileUpdateTimestamp(item map[string]any) float64 {
	for _, k := range []string{"updated_at", "l_updated_at", "u_at", "modify_time", "mtime"} {
		switch v := item[k].(type) {
		case float64:
			if v > 0 {
				// ms vs s
				if v > 1e12 {
					return v / 1000
				}
				return v
			}
		case int64:
			if v > 0 {
				f := float64(v)
				if f > 1e12 {
					return f / 1000
				}
				return f
			}
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
				if f > 1e12 {
					return f / 1000
				}
				return f
			}
		}
	}
	return 0
}

// FormatFloor for logs
func FormatFloor(n *int) string {
	if n == nil {
		return "nil"
	}
	return fmt.Sprintf("%d", *n)
}