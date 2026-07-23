package replace

import "testing"

func TestExtractEpisodeNumber(t *testing.T) {
	cases := map[string]int{
		"Show.S01E08.mkv": 8,
		"S01E11.11.mp4":   11,
		"某剧 第12集.mkv":     12,
		"Name.E05.1080p.mkv": 5,
	}
	for name, want := range cases {
		ep := ExtractEpisodeNumber(name)
		if ep == nil || *ep != want {
			t.Fatalf("%s => %v want %d", name, ep, want)
		}
	}
}

func TestSavedEpisodeFloor(t *testing.T) {
	files := []map[string]any{
		{"file_name": "S01E01.mkv", "dir": false},
		{"file_name": "S01E07.mkv", "dir": false},
		{"file_name": "poster.jpg", "dir": false},
	}
	f := GetSavedEpisodeFloor(files, nil)
	if f == nil || *f != 7 {
		t.Fatalf("floor=%v", f)
	}
}

func TestSelectStartFID(t *testing.T) {
	files := []map[string]any{
		{"fid": "a", "file_name": "S01E05.mkv"},
		{"fid": "b", "file_name": "S01E08.mkv"},
		{"fid": "c", "file_name": "S01E09.mkv"},
	}
	sel := SelectReplacementStartFID(files, 7)
	if asStr(sel["startfid"]) == "" {
		t.Fatal("expected startfid")
	}
	// episode > 7
	ep, _ := sel["episode"].(int)
	if ep <= 7 {
		t.Fatalf("ep=%v", sel["episode"])
	}
}

func TestFilterByFloor(t *testing.T) {
	files := []map[string]any{
		{"file_name": "S01E05.mkv", "fid": "1"},
		{"file_name": "S01E08.mkv", "fid": "2"},
		{"dir": true, "file_name": "Season", "fid": "d"},
	}
	out := FilterShareFilesBySavedEpisodeFloor(files, 7)
	if len(out) != 2 { // E08 + dir
		t.Fatalf("len=%d", len(out))
	}
}

func TestCompletedByTMDB(t *testing.T) {
	floor := 12
	task := map[string]any{
		"content_type": "tv",
		"calendar_info": map[string]any{
			"extracted": map[string]any{"total_episode_count": 12},
		},
	}
	if !IsCompletedInvalidShareTask(task, &floor) {
		t.Fatal("should complete")
	}
	floor = 10
	if IsCompletedInvalidShareTask(task, &floor) {
		t.Fatal("should not complete")
	}
}

func TestMovieDisabled(t *testing.T) {
	if !TaskAutoReplaceDisabled(map[string]any{"content_type": "movie"}) {
		t.Fatal("movie disabled")
	}
}
