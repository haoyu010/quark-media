package tginbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var quarkLinkRe = regexp.MustCompile(`https?://(?:www\.)?(?:pan\.)?quark\.cn/s/[A-Za-z0-9]+(?:\?[^\s]*)?`)

// Event is a recent inbox event for UI/status.
type Event struct {
	Time    int64  `json:"time"`
	Level   string `json:"level"` // ok|err|info
	Message string `json:"message"`
}

// Handler processes one share found in a message.
// Returns reply text for Telegram user.
type Handler func(shareURL, title, rawText string) (reply string, err error)

// Worker long-polls Telegram Bot API getUpdates.
type Worker struct {
	Token     string
	UserID    string
	APIHost   string
	ProxyURL  string
	OffsetFile string
	Handler   Handler
	Log       func(string)

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	events  []Event
	lastErr string
	offset  int64
}

func New(token, userID, apiHost, proxyURL, dataDir string, h Handler, logfn func(string)) *Worker {
	if apiHost == "" {
		apiHost = "api.telegram.org"
	}
	apiHost = strings.TrimPrefix(strings.TrimPrefix(apiHost, "https://"), "http://")
	apiHost = strings.Trim(apiHost, "/")
	if logfn == nil {
		logfn = func(string) {}
	}
	w := &Worker{
		Token:      strings.TrimSpace(token),
		UserID:     strings.TrimSpace(userID),
		APIHost:    apiHost,
		ProxyURL:   strings.TrimSpace(proxyURL),
		OffsetFile: filepath.Join(dataDir, "tg_inbox_offset.json"),
		Handler:    h,
		Log:        logfn,
		events:     make([]Event, 0, 30),
	}
	w.offset = w.loadOffset()
	return w
}

func (w *Worker) Running() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

func (w *Worker) LastError() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastErr
}

func (w *Worker) Events() []Event {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]Event, len(w.events))
	copy(out, w.events)
	return out
}

func (w *Worker) addEvent(level, msg string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, Event{Time: time.Now().Unix(), Level: level, Message: msg})
	if len(w.events) > 40 {
		w.events = w.events[len(w.events)-40:]
	}
	if level == "err" {
		w.lastErr = msg
	}
}

func (w *Worker) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return nil
	}
	if w.Token == "" {
		return fmt.Errorf("missing bot token")
	}
	if w.UserID == "" {
		return fmt.Errorf("missing allowed user id")
	}
	if w.Handler == nil {
		return fmt.Errorf("missing handler")
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.running = true
	w.lastErr = ""
	go w.loop(ctx)
	w.Log("tg inbox worker started")
	return nil
}

func (w *Worker) Stop() {
	w.mu.Lock()
	cancel := w.cancel
	w.running = false
	w.cancel = nil
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	w.Log("tg inbox worker stopped")
}

func (w *Worker) loop(ctx context.Context) {
	// drop pending webhook if any
	_, _ = w.api("deleteWebhook", url.Values{"drop_pending_updates": {"false"}})
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		params := url.Values{}
		params.Set("timeout", "20")
		params.Set("allowed_updates", `["message","channel_post"]`)
		if w.offset > 0 {
			params.Set("offset", fmt.Sprintf("%d", w.offset))
		}
		body, err := w.api("getUpdates", params)
		if err != nil {
			w.addEvent("err", "getUpdates: "+err.Error())
			w.Log("tg getUpdates: " + err.Error())
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		var resp struct {
			OK     bool `json:"ok"`
			Result []struct {
				UpdateID int64 `json:"update_id"`
				Message  *struct {
					MessageID int64          `json:"message_id"`
					Text      string         `json:"text"`
					Caption   string         `json:"caption"`
					Chat      map[string]any `json:"chat"`
					From      map[string]any `json:"from"`
				} `json:"message"`
				ChannelPost *struct {
					MessageID int64          `json:"message_id"`
					Text      string         `json:"text"`
					Caption   string         `json:"caption"`
					Chat      map[string]any `json:"chat"`
					From      map[string]any `json:"from"`
				} `json:"channel_post"`
			} `json:"result"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			w.addEvent("err", "decode updates: "+err.Error())
			continue
		}
		if !resp.OK {
			w.addEvent("err", "getUpdates not ok: "+resp.Description)
			time.Sleep(3 * time.Second)
			continue
		}
		for _, u := range resp.Result {
			if u.UpdateID+1 > w.offset {
				w.offset = u.UpdateID + 1
				w.saveOffset()
			}
			var text string
			var chatID, fromID string
			if u.Message != nil {
				text = u.Message.Text
				if text == "" {
					text = u.Message.Caption
				}
				chatID = fmt.Sprint(u.Message.Chat["id"])
				if u.Message.From != nil {
					fromID = fmt.Sprint(u.Message.From["id"])
				}
			} else if u.ChannelPost != nil {
				text = u.ChannelPost.Text
				if text == "" {
					text = u.ChannelPost.Caption
				}
				chatID = fmt.Sprint(u.ChannelPost.Chat["id"])
			}
			if text == "" {
				continue
			}
			if !w.authorized(chatID, fromID) {
				w.addEvent("info", "ignore unauthorized msg from chat="+chatID+" from="+fromID)
				continue
			}
			links := ExtractQuarkLinks(text)
			if len(links) == 0 {
				// QAS: no_link - ignore non-link chatter without reply noise
				continue
			}
			// QAS handle_message uses only the first share link
			link := links[0]
			title := ExtractTitle(text)
			w.addEvent("info", "收到链接: "+link)
			w.Log("tg inbox link: " + link)
			reply, err := w.Handler(link, title, text)
			if err != nil {
				msg := "处理失败: " + err.Error()
				w.addEvent("err", msg)
				_ = w.send(chatID, "处理失败: "+msg)
				continue
			}
			if reply == "" {
				reply = "已处理: " + link
			}
			w.addEvent("ok", reply)
			_ = w.send(chatID, reply)
		}
	}
}

func (w *Worker) authorized(chatID, fromID string) bool {
	allow := strings.TrimSpace(w.UserID)
	if allow == "" {
		return false
	}
	// support comma-separated ids
	for _, p := range strings.FieldsFunc(allow, func(r rune) bool { return r == ',' || r == ' ' || r == ';' || r == '\n' }) {
		p = strings.TrimSpace(p)
		if p != "" && (p == chatID || p == fromID) {
			return true
		}
	}
	return false
}

func (w *Worker) maskErr(err error) error {
	if err == nil {
		return nil
	}
	s := err.Error()
	if w.Token != "" {
		s = strings.ReplaceAll(s, w.Token, "***")
		s = strings.ReplaceAll(s, "bot"+w.Token, "bot***")
	}
	return fmt.Errorf("%s", s)
}

func (w *Worker) httpClient(timeout time.Duration) *http.Client {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}
	if w.ProxyURL != "" {
		if u, err := url.Parse(w.ProxyURL); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Timeout: timeout, Transport: tr}
}

func (w *Worker) api(method string, params url.Values) ([]byte, error) {
	u := fmt.Sprintf("https://%s/bot%s/%s", w.APIHost, w.Token, method)
	// getUpdates long-poll needs longer timeout than other calls
	timeout := 35 * time.Second
	if method == "getUpdates" {
		timeout = 40 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, w.maskErr(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := w.httpClient(timeout).Do(req)
	if err != nil {
		return nil, w.maskErr(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, w.maskErr(err)
	}
	if resp.StatusCode >= 300 {
		return b, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return b, nil
}

func (w *Worker) send(chatID, text string) error {
	if chatID == "" {
		chatID = w.UserID
	}
	_, err := w.api("sendMessage", url.Values{
		"chat_id": {chatID},
		"text":    {text},
	})
	return err
}

func (w *Worker) loadOffset() int64 {
	b, err := os.ReadFile(w.OffsetFile)
	if err != nil {
		return 0
	}
	var raw map[string]any
	if json.Unmarshal(b, &raw) != nil {
		return 0
	}
	switch v := raw["offset"].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case json.Number:
		n, _ := v.Int64()
		return n
	}
	return 0
}

func (w *Worker) saveOffset() {
	_ = os.MkdirAll(filepath.Dir(w.OffsetFile), 0o755)
	b, _ := json.MarshalIndent(map[string]any{"offset": w.offset, "updated": time.Now().Unix()}, "", "  ")
	_ = os.WriteFile(w.OffsetFile, b, 0o664)
}

// ExtractQuarkLinks finds pan.quark.cn share URLs.
func ExtractQuarkLinks(text string) []string {
	found := quarkLinkRe.FindAllString(text, -1)
	seen := map[string]bool{}
	var out []string
	for _, u := range found {
		// strip trailing punctuation
		u = strings.TrimRight(u, ")。.,，、；;！!？?\"'`")
		// normalize host
		u = strings.Replace(u, "://quark.cn/s/", "://pan.quark.cn/s/", 1)
		if seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// ExtractTitle picks a human title seed from message text.
func ExtractTitle(text string) string {
	content := text
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	// literal backslash-n from JSON bridges
	content = strings.ReplaceAll(content, `\n`, "\n")
	for _, link := range ExtractQuarkLinks(content) {
		content = strings.ReplaceAll(content, link, " ")
	}
	content = regexp.MustCompile(`https?://\S+`).ReplaceAllString(content, " ")
	lines := strings.Split(content, "\n")
	if len(lines) <= 1 {
		tmp := content
		for _, sep := range []string{"夸克：", "夸克:", "链接：", "链接:", "大小：", "大小:"} {
			tmp = strings.ReplaceAll(tmp, sep, "\n")
		}
		lines = strings.Split(tmp, "\n")
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.Trim(line, " -_|:：，,。;；")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "大小") || strings.HasPrefix(line, "标签") || strings.HasPrefix(line, "夸克") || strings.HasPrefix(line, "链接") {
			continue
		}
		line = regexp.MustCompile(`^(资源|片名|剧名|名称|标题)[:：]\s*`).ReplaceAllString(line, "")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len([]rune(line)) > 80 {
			line = string([]rune(line)[:80])
		}
		return line
	}
	return "TG收链资源"
}

