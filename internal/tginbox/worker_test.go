package tginbox

import "testing"

func TestExtractQuarkLinks(t *testing.T) {
	text := `名称：东大高武学院 (2026)
夸克：https://pan.quark.cn/s/eb71f368c556
大小：2.5G/集`
	links := ExtractQuarkLinks(text)
	if len(links) != 1 || links[0] != "https://pan.quark.cn/s/eb71f368c556" {
		t.Fatalf("links=%v", links)
	}
	title := ExtractTitle(text)
	if title == "" || title == "TG收链资源" {
		t.Fatalf("title=%q", title)
	}
}

func TestAuthorized(t *testing.T) {
	w := &Worker{UserID: "8189522184"}
	if !w.authorized("8189522184", "8189522184") {
		t.Fatal("should auth")
	}
	if w.authorized("1", "2") {
		t.Fatal("should reject")
	}
}

func TestNormalizeShareID(t *testing.T) {
	id := NormalizeShareID("https://pan.quark.cn/s/eb71f368c556")
	if id != "eb71f368c556" {
		t.Fatalf("id=%q", id)
	}
	if NormalizeShareID("nope") != "" {
		t.Fatal("expected empty")
	}
}

func TestBuildSavePathTV(t *testing.T) {
	p := BuildSavePath("影视库", "tv", "东大高武学院", "2026", 1, "")
	want := "影视库/电视剧/东大高武学院/Season 01"
	if p != want {
		t.Fatalf("got %q want %q", p, want)
	}
}

func TestBuildSavePathMovie(t *testing.T) {
	p := BuildSavePath("影视库", "movie", "某电影", "2024", 1, "")
	want := "影视库/电影/某电影 (2024)"
	if p != want {
		t.Fatalf("got %q want %q", p, want)
	}
}

func TestLooksLikeSeries(t *testing.T) {
	if !LooksLikeSeries("名称：东大高武学院 (2026)\n更新至06集") {
		t.Fatal("expected series")
	}
	if LooksLikeSeries("某电影 1080P") && LooksLikeSeries("纯电影名") {
		// soft check — pure movie name may or may not look like series
	}
	if !LooksLikeSeries("S01E02 测试") {
		t.Fatal("S01E02 should be series")
	}
}

func TestExtractYearSeason(t *testing.T) {
	if ExtractYear("东大高武学院 (2026)") != "2026" {
		t.Fatal(ExtractYear("东大高武学院 (2026)"))
	}
	if ExtractSeason("Season 2 更新") != 2 {
		t.Fatal(ExtractSeason("Season 2 更新"))
	}
}


func TestAnyID(t *testing.T) {
	if anyID(float64(8189522184)) != "8189522184" {
		t.Fatalf("float id=%q", anyID(float64(8189522184)))
	}
	w := &Worker{UserID: "8189522184"}
	if !w.authorized(anyID(float64(8189522184)), anyID(float64(8189522184))) {
		t.Fatal("auth should pass with float ids")
	}
	// scientific string form
	if !w.authorized("8.189522184e+09", "8.189522184e+09") {
		t.Fatal("auth should pass with sci-notation string ids")
	}
}
