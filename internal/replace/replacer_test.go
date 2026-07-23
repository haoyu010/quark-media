package replace

import "testing"

func TestLoadSettingsEnabled(t *testing.T) {
	s := LoadSettings("enabled", map[string]any{"auto_replace_min_score": 80})
	if !s.Enabled {
		t.Fatal("should enable")
	}
	if s.MinScore != 80 {
		t.Fatalf("score=%d", s.MinScore)
	}
}

func TestBuildSearchQueries(t *testing.T) {
	r := &Replacer{Settings: Settings{Enabled: true, MinScore: 85}}
	q := r.BuildSearchQueries(map[string]any{
		"taskname": "东大高武学院 (2026) 1080p",
		"savepath": "影视库/电视剧/东大高武学院 (2026)/Season 01",
	})
	if len(q) == 0 {
		t.Fatal("no queries")
	}
	if q[0] == "" {
		t.Fatal("empty")
	}
}

func TestScoreRejectLowRes(t *testing.T) {
	r := &Replacer{Settings: Settings{Enabled: true, MinScore: 85, QualityPolicy: "no_downgrade"}}
	score, why := r.ScoreCandidate(
		map[string]any{"taskname": "某剧 1080p"},
		map[string]any{"taskname": "某剧 720p", "content": "720p"},
		map[string]any{"files": []map[string]any{{"file_name": "E01.720p.mkv", "size": 1e9}}},
		map[string]any{"required_resolution": 1080, "avg_size": 0.0},
	)
	if score != 0 {
		t.Fatalf("score=%d why=%s", score, why)
	}
}

func TestTaskDisabledMovie(t *testing.T) {
	if !TaskDisabled(map[string]any{"content_type": "movie"}) {
		t.Fatal("movie should disable")
	}
}

func TestIsInvalidShareErr(t *testing.T) {
	if !IsInvalidShareErr(errStr("获取 stoken 失败")) {
		t.Fatal("should detect")
	}
}

type errStr string

func (e errStr) Error() string { return string(e) }
