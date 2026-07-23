package strm

import (
	"os"
	"path/filepath"
	"strings"
)

func PlayURL(publicBase, fid, name string) string {
	base := strings.TrimRight(publicBase, "/")
	fn := name
	if fn == "" {
		fn = "video.mp4"
	}
	// keep extension-ish for players
	return base + "/play/" + fid + "/" + filepath.Base(fn)
}

func SafeName(s string) string {
	repl := []string{"<", ">", ":", "\"", "/", "\\", "|", "?", "*"}
	for _, r := range repl {
		s = strings.ReplaceAll(s, r, "_")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		s = "unnamed"
	}
	return s
}

func Write(path, content string) (changed bool, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if b, e := os.ReadFile(path); e == nil {
		if strings.TrimSpace(string(b)) == strings.TrimSpace(content) {
			return false, nil
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content+"\n"), 0o644); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return false, err
	}
	return true, nil
}

type Video struct {
	FID  string
	Name string
	Path string
}

func Generate(videos []Video, strmRoot, publicBase, stripPrefix string) (created, updated, skipped int, err error) {
	stripPrefix = strings.Trim(stripPrefix, "/")
	for _, v := range videos {
		rel := strings.Trim(v.Path, "/")
		if stripPrefix != "" && (rel == stripPrefix || strings.HasPrefix(rel, stripPrefix+"/")) {
			rel = strings.TrimPrefix(rel, stripPrefix)
			rel = strings.Trim(rel, "/")
		}
		if rel == "" {
			rel = v.Name
		}
		parts := strings.Split(rel, "/")
		for i := range parts {
			if i < len(parts)-1 {
				parts[i] = SafeName(parts[i])
			}
		}
		name := parts[len(parts)-1]
		stem := name
		ext := ""
		if i := strings.LastIndex(name, "."); i > 0 {
			stem = name[:i]
			ext = name[i+1:]
		}
		stem = SafeName(stem)
		fname := stem + ".strm"
		if ext != "" {
			fname = stem + ".(" + ext + ").strm"
		}
		dirParts := parts[:len(parts)-1]
		out := filepath.Join(append([]string{strmRoot}, dirParts...)...)
		out = filepath.Join(out, fname)
		existed := false
		if _, e := os.Stat(out); e == nil {
			existed = true
		}
		ch, e := Write(out, PlayURL(publicBase, v.FID, v.Name))
		if e != nil {
			return created, updated, skipped, e
		}
		if !ch {
			skipped++
		} else if existed {
			updated++
		} else {
			created++
		}
	}
	return
}

func List(strmRoot string) ([]map[string]string, error) {
	var out []map[string]string
	_ = filepath.Walk(strmRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".strm") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(strmRoot, path)
		out = append(out, map[string]string{
			"name": info.Name(),
			"rel":  filepath.ToSlash(rel),
			"url":  strings.TrimSpace(string(b)),
		})
		return nil
	})
	return out, nil
}
