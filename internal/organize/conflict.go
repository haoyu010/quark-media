//go:build ignore
// +build ignore

package organize

import (
	"fmt"
	"path/filepath"
	"strings"
)

func MarkConflicts(items []PlanItem) int {
	byPath := map[string][]int{}
	display := map[string]string{}
	for i, item := range items {
		path := strings.TrimSpace(item.TargetPath)
		if path == "" {
			continue
		}
		clean := filepath.Clean(path)
		// 大小写不敏感文件系统（APFS/exFAT/115）上 Friends 与 friends 会互相覆盖，
		// 去重 key 统一小写化；展示仍用首次出现的原始路径。
		key := strings.ToLower(clean)
		if _, ok := display[key]; !ok {
			display[key] = clean
		}
		byPath[key] = append(byPath[key], i)
	}

	conflicted := 0
	for key, indexes := range byPath {
		if len(indexes) < 2 {
			continue
		}
		path := display[key]
		conflicted += len(indexes)
		names := make([]string, 0, len(indexes))
		for _, idx := range indexes {
			name := items[idx].FileInput.Name
			if name == "" {
				name = items[idx].FileInput.Path
			}
			if name != "" {
				names = append(names, name)
			}
		}
		detail := path
		if len(names) > 0 {
			detail = fmt.Sprintf("%s <= %s", path, strings.Join(names, ", "))
		}
		for _, idx := range indexes {
			items[idx].Conflict = true
			items[idx].ConflictMsg = fmt.Sprintf("目标路径冲突: %s", detail)
		}
	}
	return conflicted
}
