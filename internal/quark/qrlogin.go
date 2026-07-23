package quark

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"
)

const (
	qrClientID = "532"
	qrVersion  = "1.2"
	qrTokenAPI = "https://uop.quark.cn/cas/ajax/getTokenForQrcodeLogin"
	qrPollAPI  = "https://uop.quark.cn/cas/ajax/getServiceTicketByQrcodeToken"
	qrAccount  = "https://pan.quark.cn/account/info"
)

// QRSession holds one QR login attempt (in-memory).
type QRSession struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"`
	Content   string    `json:"content"`
	QRImage   string    `json:"qr_image"` // data:image/png;base64,...
	Status    string    `json:"status"`   // pending|scanned|confirmed|expired|error
	Message   string    `json:"message"`
	Cookie    string    `json:"cookie,omitempty"`
	Nickname  string    `json:"nickname,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type qrStore struct {
	mu   sync.Mutex
	sess map[string]*QRSession
}

var globalQR = &qrStore{sess: map[string]*QRSession{}}

func (s *qrStore) put(ss *QRSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, v := range s.sess {
		if now.After(v.ExpiresAt.Add(2 * time.Minute)) {
			delete(s.sess, id)
		}
	}
	s.sess[ss.ID] = ss
}

func (s *qrStore) get(id string) *QRSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss := s.sess[id]
	if ss == nil {
		return nil
	}
	if time.Now().After(ss.ExpiresAt) && ss.Status == "pending" {
		ss.Status = "expired"
		ss.Message = "二维码已过期"
	}
	cp := *ss
	return &cp
}

func (s *qrStore) update(id string, fn func(*QRSession)) *QRSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss := s.sess[id]
	if ss == nil {
		return nil
	}
	fn(ss)
	cp := *ss
	return &cp
}

func (s *qrStore) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sess, id)
}

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	// UUID-like
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func qrHTTP() *http.Client {
	return &http.Client{Timeout: 25 * time.Second}
}

// StartQRLogin creates a token + QR content + PNG data URL.
func StartQRLogin() (*QRSession, error) {
	rid := newRequestID()
	u := fmt.Sprintf("%s?client_id=%s&v=%s&request_id=%s", qrTokenAPI, qrClientID, qrVersion, url.QueryEscape(rid))
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", uaPC)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", "https://pan.quark.cn/")
	req.Header.Set("Origin", "https://pan.quark.cn")
	resp, err := qrHTTP().Do(req)
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var js map[string]any
	if err := json.Unmarshal(body, &js); err != nil {
		return nil, fmt.Errorf("token json: %v raw=%s", err, truncate(string(body), 200))
	}
	status, _ := js["status"].(float64)
	if int(status) != 2000000 {
		return nil, fmt.Errorf("token status=%v msg=%v", js["status"], js["message"])
	}
	data, _ := js["data"].(map[string]any)
	members, _ := data["members"].(map[string]any)
	token, _ := members["token"].(string)
	if token == "" {
		return nil, fmt.Errorf("empty token: %s", truncate(string(body), 200))
	}

	content := "https://su.quark.cn/4_eMHBJ?token=" + url.QueryEscape(token) +
		"&client_id=" + qrClientID +
		"&ssb=weblogin&uc_param_str=&uc_biz_str=" +
		url.QueryEscape("S:custom|OPT:SAREA@0|OPT:IMMERSIVE@1|OPT:BACK_BTN_STYLE@0")

	png, err := qrcode.Encode(content, qrcode.Medium, 256)
	if err != nil {
		return nil, fmt.Errorf("qr encode: %w", err)
	}
	img := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)

	ss := &QRSession{
		ID:        newRequestID(),
		Token:     token,
		Content:   content,
		QRImage:   img,
		Status:    "pending",
		Message:   "请使用夸克 App 扫码",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(3 * time.Minute),
	}
	globalQR.put(ss)
	return ss, nil
}

// PollQRLogin checks scan status; on confirm exchanges service ticket for cookies.
func PollQRLogin(id string) (*QRSession, error) {
	ss := globalQR.get(id)
	if ss == nil {
		return nil, fmt.Errorf("session not found")
	}
	if ss.Status == "confirmed" || ss.Status == "expired" || ss.Status == "error" {
		return ss, nil
	}
	if time.Now().After(ss.ExpiresAt) {
		return globalQR.update(id, func(s *QRSession) {
			s.Status = "expired"
			s.Message = "二维码已过期，请刷新"
		}), nil
	}

	rid := newRequestID()
	u := fmt.Sprintf("%s?client_id=%s&v=%s&token=%s&request_id=%s",
		qrPollAPI, qrClientID, qrVersion, url.QueryEscape(ss.Token), url.QueryEscape(rid))
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", uaPC)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", "https://pan.quark.cn/")
	req.Header.Set("Origin", "https://pan.quark.cn")
	resp, err := qrHTTP().Do(req)
	if err != nil {
		return globalQR.update(id, func(s *QRSession) {
			s.Status = "error"
			s.Message = "轮询失败: " + err.Error()
		}), nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var js map[string]any
	if err := json.Unmarshal(body, &js); err != nil {
		return globalQR.update(id, func(s *QRSession) {
			s.Status = "error"
			s.Message = "轮询解析失败"
		}), nil
	}

	statusF, _ := js["status"].(float64)
	status := int(statusF)
	msg, _ := js["message"].(string)

	if status == 50004001 {
		return globalQR.update(id, func(s *QRSession) {
			s.Status = "pending"
			s.Message = "等待扫码…"
		}), nil
	}
	if status == 50004002 {
		return globalQR.update(id, func(s *QRSession) {
			s.Status = "scanned"
			s.Message = "已扫码，请在手机上确认登录"
		}), nil
	}
	if status == 50004003 || status == 50004004 || status == 50009008 {
		return globalQR.update(id, func(s *QRSession) {
			s.Status = "expired"
			s.Message = firstNonEmpty(msg, "二维码已失效")
		}), nil
	}

	if status != 2000000 {
		if strings.Contains(strings.ToLower(msg), "empty") || strings.Contains(msg, "空") {
			return globalQR.update(id, func(s *QRSession) {
				s.Status = "pending"
				s.Message = "等待扫码…"
			}), nil
		}
		return globalQR.update(id, func(s *QRSession) {
			s.Message = firstNonEmpty(msg, fmt.Sprintf("status=%d", status))
		}), nil
	}

	data, _ := js["data"].(map[string]any)
	members, _ := data["members"].(map[string]any)
	st, _ := members["service_ticket"].(string)
	if st == "" {
		st, _ = data["service_ticket"].(string)
	}
	if st == "" {
		return globalQR.update(id, func(s *QRSession) {
			s.Status = "error"
			s.Message = "无 service_ticket"
		}), nil
	}

	cookie, nick, err := exchangeServiceTicket(st)
	if err != nil {
		return globalQR.update(id, func(s *QRSession) {
			s.Status = "error"
			s.Message = "换取 Cookie 失败: " + err.Error()
		}), nil
	}
	if cookie == "" || len(cookie) < 20 {
		return globalQR.update(id, func(s *QRSession) {
			s.Status = "error"
			s.Message = "Cookie 为空，登录失败"
		}), nil
	}

	return globalQR.update(id, func(s *QRSession) {
		s.Status = "confirmed"
		s.Message = "登录成功"
		s.Cookie = cookie
		s.Nickname = nick
	}), nil
}

// CancelQRLogin drops a session.
func CancelQRLogin(id string) {
	globalQR.delete(id)
}

// GetQRSession returns a session snapshot.
func GetQRSession(id string) *QRSession {
	return globalQR.get(id)
}

func exchangeServiceTicket(st string) (cookie string, nickname string, err error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", "", err
	}
	cli := &http.Client{
		Timeout: 30 * time.Second,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 8 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	u := fmt.Sprintf("%s?st=%s&lw=scan", qrAccount, url.QueryEscape(st))
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", uaPC)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", "https://pan.quark.cn/")
	req.Header.Set("Origin", "https://pan.quark.cn")
	resp, err := cli.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var js map[string]any
	if json.Unmarshal(body, &js) == nil {
		if data, ok := js["data"].(map[string]any); ok {
			for _, k := range []string{"nickname", "nick_name", "user_name", "username", "name"} {
				if v, ok := data[k].(string); ok && v != "" {
					nickname = v
					break
				}
			}
		}
	}

	hosts := []string{
		"https://pan.quark.cn",
		"https://drive-pc.quark.cn",
		"https://drive-m.quark.cn",
		"https://quark.cn",
	}
	seen := map[string]string{}
	for _, h := range hosts {
		pu, _ := url.Parse(h)
		for _, c := range jar.Cookies(pu) {
			if c.Value == "" {
				continue
			}
			seen[c.Name] = c.Value
		}
	}
	for _, c := range resp.Cookies() {
		if c.Value != "" {
			seen[c.Name] = c.Value
		}
	}

	prefer := []string{"__puus", "__pus", "__kp", "__kps", "tfstk", "isg", "__uid"}
	var parts []string
	used := map[string]bool{}
	for _, k := range prefer {
		if v, ok := seen[k]; ok {
			parts = append(parts, k+"="+v)
			used[k] = true
		}
	}
	for k, v := range seen {
		if used[k] {
			continue
		}
		parts = append(parts, k+"="+v)
	}
	cookie = strings.Join(parts, "; ")
	return cookie, nickname, nil
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
