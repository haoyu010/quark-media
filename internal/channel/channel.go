package channel

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var reQuark = regexp.MustCompile(`https?://(?:pan\.)?quark\.cn/s/[a-zA-Z0-9]+(?:\?[^\s"'<>]*)?`)

type Hit struct {
	URL     string `json:"url"`
	Text    string `json:"text"`
	Channel string `json:"channel"`
}

// NormalizeChannel turns t.me / telegram.me URLs into bare channel username/id.
func NormalizeChannel(ch string) string {
	ch = strings.TrimSpace(ch)
	ch = strings.TrimPrefix(ch, "@")
	low := strings.ToLower(ch)
	for _, p := range []string{
		"https://telegram.me/s/", "http://telegram.me/s/",
		"https://t.me/s/", "http://t.me/s/",
		"https://telegram.me/", "http://telegram.me/",
		"https://t.me/", "http://t.me/",
		"telegram.me/s/", "t.me/s/",
		"telegram.me/", "t.me/",
	} {
		if strings.HasPrefix(low, p) {
			ch = ch[len(p):]
			low = strings.ToLower(ch)
			break
		}
	}
	// drop query/fragment
	if i := strings.IndexAny(ch, "?#"); i >= 0 {
		ch = ch[:i]
	}
	ch = strings.Trim(ch, "/")
	return strings.TrimSpace(ch)
}

func SearchPublic(channels []string, keywords []string, limit int) ([]Hit, error) {
	if limit <= 0 {
		limit = 30
	}
	client := &http.Client{Timeout: 15 * time.Second}
	var hits []Hit
	for _, ch := range channels {
		ch = NormalizeChannel(ch)
		if ch == "" {
			continue
		}
		u := "https://t.me/s/" + ch
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0")
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		html := string(b)
		// crude message split
		parts := strings.Split(html, "tgme_widget_message_text")
		for _, part := range parts[1:] {
			text := stripTags(part)
			urls := reQuark.FindAllString(part, -1)
			if len(urls) == 0 {
				continue
			}
			if len(keywords) > 0 {
				low := strings.ToLower(text)
				ok := false
				for _, k := range keywords {
					k = strings.TrimSpace(strings.ToLower(k))
					if k != "" && strings.Contains(low, k) {
						ok = true
						break
					}
				}
				if !ok {
					continue
				}
			}
			for _, url := range urls {
				hits = append(hits, Hit{URL: url, Text: trimText(text, 200), Channel: ch})
				if len(hits) >= limit {
					return hits, nil
				}
			}
		}
	}
	return hits, nil
}

func stripTags(s string) string {
	re := regexp.MustCompile(`<[^>]+>`)
	s = re.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	return strings.Join(strings.Fields(s), " ")
}

func trimText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}