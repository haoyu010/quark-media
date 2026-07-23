package replace

import (
	"fmt"
	"strings"

	"quark-media/internal/qas"
	"quark-media/internal/quark"
)

// AutoReplaceResult is QAS try_auto_replace_invalid_shareurl return + extras.
type AutoReplaceResult struct {
	Result
	StartFIDUpdate map[string]any `json:"startfid_update,omitempty"`
	SavedFloor     *int           `json:"saved_episode_floor,omitempty"`
}

// Service orchestrates QAS try_auto_replace + prepare_startfid + persist.
type Service struct {
	Replacer *Replacer
	Client   *quark.Client
	QASPath  string
	Log      func(string)
}

// NewService builds full QAS-compatible auto replace service.
func NewService(qasPath string, client *quark.Client, channels []string, logfn func(string)) *Service {
	if logfn == nil {
		logfn = func(string) {}
	}
	r := NewFromQAS(qasPath, client, channels, logfn)
	return &Service{Replacer: r, Client: client, QASPath: qasPath, Log: logfn}
}

// LoadSavedFiles ports ResourceAutoReplacer._load_saved_files via quark PathToFID+LS.
func (s *Service) LoadSavedFiles(task map[string]any) []map[string]any {
	if s.Client == nil {
		return nil
	}
	save := firstSave(task)
	save = strings.Trim(strings.ReplaceAll(save, "\\", "/"), "/")
	if save == "" {
		return nil
	}
	fid, err := s.Client.PathToFID(save)
	if err != nil || fid == "" || fid == "0" {
		return nil
	}
	items, err := s.Client.LS(fid, 200)
	if err != nil {
		s.Log("auto replace read target directory failed: " + err.Error())
		return nil
	}
	return items
}

// SavedEpisodeFloorForTask ports get_saved_episode_floor_for_task (dir listing; no transfer records db).
func (s *Service) SavedEpisodeFloorForTask(task map[string]any) *int {
	// prefer explicit floor already on task
	if f := GetAutoReplaceSavedEpisodeFloor(task); f != nil {
		return f
	}
	files := s.LoadSavedFiles(task)
	return GetSavedEpisodeFloor(files, nil)
}

// TryAutoReplaceInvalidShareURL ports QuarkAccount.try_auto_replace_invalid_shareurl 1:1 flow.
func (s *Service) TryAutoReplaceInvalidShareURL(task map[string]any, reason string) AutoReplaceResult {
	out := AutoReplaceResult{}
	if task == nil {
		out.Message = "任务为空"
		return out
	}
	savedFloor := s.SavedEpisodeFloorForTask(task)
	if IsCompletedInvalidShareTask(task, savedFloor) {
		out.Message = "任务已完结，跳过自动换源"
		return out
	}
	if TaskAutoReplaceDisabled(task) {
		out.Message = "任务已禁用自动换源"
		return out
	}

	// inject episode extractor into scoring path via Replacer (filter already uses EpisodeFromFileInfo)
	// refresh baseline saved files into replacer via task side channel
	task["_saved_files_for_baseline"] = s.LoadSavedFiles(task)

	rr := s.Replacer.TryReplace(task, reason)
	out.Result = rr
	if !rr.Attempted {
		return out
	}
	if !rr.Replaced {
		s.Log(fmt.Sprintf("自动换源未替换《%s》: %s", firstName(task), rr.Message))
		return out
	}

	// prepare startfid
	sel := s.PrepareAutoReplaceStartFID(task, &out)
	if sel != nil {
		out.StartFIDUpdate = sel
		s.Log(fmt.Sprintf("auto replace adjusted startfid: %v -> %v", sel["file_name"], sel["startfid"]))
	}

	// persist
	_ = PersistAutoReplacedShareURL(s.QASPath, task, out)
	src := ""
	score := any(nil)
	if out.Best != nil {
		src = asStr(out.Best["source"])
		score = out.Best["score"]
	}
	if src == "" {
		src = "搜索来源"
	}
	scoreText := ""
	if score != nil {
		scoreText = fmt.Sprintf("，评分 %v", score)
	}
	msg := fmt.Sprintf("♻️《%s》失效链接已自动换源（%s%s）", firstName(task), src, scoreText)
	s.Log(msg)
	out.Message = msg
	out.SavedFloor = GetAutoReplaceSavedEpisodeFloor(task)
	return out
}

// PrepareAutoReplaceStartFID ports prepare_auto_replace_startfid.
func (s *Service) PrepareAutoReplaceStartFID(task map[string]any, replaceResult *AutoReplaceResult) map[string]any {
	if replaceResult == nil || replaceResult.Best == nil {
		return nil
	}
	var files []map[string]any
	switch v := replaceResult.Best["files"].(type) {
	case []map[string]any:
		files = v
	case []any:
		for _, x := range v {
			if m, ok := x.(map[string]any); ok {
				files = append(files, m)
			}
		}
	}
	if len(files) == 0 {
		return nil
	}
	savedFloor := s.SavedEpisodeFloorForTask(task)
	if savedFloor == nil {
		return nil
	}
	task["_auto_replace_saved_episode_floor"] = *savedFloor
	task["auto_replace_saved_episode_floor"] = *savedFloor
	task["_auto_replace_ignore_startfid_once"] = true
	selection := SelectReplacementStartFID(files, *savedFloor)
	if asStr(selection["startfid"]) == "" {
		return nil
	}
	task["startfid"] = selection["startfid"]
	replaceResult.StartFIDUpdate = selection
	return selection
}

// PersistAutoReplacedShareURL ports persist_auto_replaced_shareurl.
func PersistAutoReplacedShareURL(qasPath string, task map[string]any, replaceResult AutoReplaceResult) error {
	name := firstName(task)
	oldShare := replaceResult.OldShareURL
	if oldShare == "" {
		oldShare = asStr(task["_auto_replace_old_shareurl"])
	}
	newShare := firstShare(task)
	if newShare == "" {
		newShare = replaceResult.NewShareURL
	}
	if newShare == "" {
		return fmt.Errorf("empty new share")
	}
	startfid := ""
	if replaceResult.StartFIDUpdate != nil {
		startfid = asStr(replaceResult.StartFIDUpdate["startfid"])
	}
	if startfid == "" {
		startfid = asStr(task["startfid"])
	}
	floor := GetAutoReplaceSavedEpisodeFloor(task)
	return qas.UpdateTaskShareFull(qasPath, name, oldShare, newShare, startfid, floor)
}

// RetrySaveAfterAutoReplace ports retry_save_after_auto_replace semantics.
// Returns (replaced, newShare, floor, startfid, errMsg).
func (s *Service) RetrySaveAfterAutoReplace(task map[string]any, reason string) (bool, string, *int, string, string) {
	if asBool(task["_auto_replace_retrying"]) {
		return false, "", nil, "", "already retrying"
	}
	savedFloor := s.SavedEpisodeFloorForTask(task)
	if IsCompletedInvalidShareTask(task, savedFloor) {
		return false, "", nil, "", "completed"
	}
	if TaskAutoReplaceDisabled(task) {
		return false, "", nil, "", "disabled"
	}
	rr := s.TryAutoReplaceInvalidShareURL(task, reason)
	if !rr.Replaced {
		return false, "", nil, "", rr.Message
	}
	task["_auto_replace_retrying"] = true
	defer func() {
		delete(task, "_auto_replace_retrying")
		delete(task, "_auto_replace_saved_episode_floor")
		delete(task, "_auto_replace_ignore_startfid_once")
	}()
	startfid := asStr(task["startfid"])
	floor := GetAutoReplaceSavedEpisodeFloor(task)
	return true, firstShare(task), floor, startfid, ""
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