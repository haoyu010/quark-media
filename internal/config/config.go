package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

type ServerCfg struct {
	Host       string `yaml:"host" json:"host"`
	Port       int    `yaml:"port" json:"port"`
	PublicBase string `yaml:"public_base" json:"public_base"`
}

type EmbyCfg struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	BaseURL string `yaml:"base_url" json:"base_url"`
	APIKey  string `yaml:"api_key" json:"api_key"`
	Path    string `yaml:"path" json:"path"`
}

type NamingTemplate struct {
	Movie          string `yaml:"movie" json:"movie"`
	TVShowDir      string `yaml:"tv_show_dir" json:"tv_show_dir"`
	TVSeasonDir    string `yaml:"tv_season_dir" json:"tv_season_dir"`
	TVFile         string `yaml:"tv_file" json:"tv_file"`
	TVCombo        string `yaml:"tv_combo" json:"tv_combo"`
	EnableCategory bool   `yaml:"enable_category" json:"enable_category"`
}

type OrganizeCfg struct {
	AutoOrganize   bool           `yaml:"auto_organize" json:"auto_organize"`
	ScrapeEnabled  bool           `yaml:"scrape_enabled" json:"scrape_enabled"`
	EnableCategory bool           `yaml:"enable_category" json:"enable_category"`
	CategoryFile   string         `yaml:"category_file" json:"category_file"`
	NamingTemplate NamingTemplate `yaml:"naming_template" json:"naming_template"`
}

type SubSearchCfg struct {
	Enabled          bool `yaml:"enabled" json:"enabled"`
	OnlyMissingShare bool `yaml:"only_missing_share" json:"only_missing_share"`
	ApplyBest        bool `yaml:"apply_best" json:"apply_best"`
}

type MtprotoCfg struct {
	Enabled             bool     `yaml:"enabled" json:"enabled"`
	APIID               string   `yaml:"api_id" json:"api_id"`
	APIHash             string   `yaml:"api_hash" json:"api_hash"`
	Phone               string   `yaml:"phone" json:"phone"`
	SessionPath         string   `yaml:"session_path" json:"session_path"`
	Channels            []string `yaml:"channels" json:"channels"`
	AutoApply           bool     `yaml:"auto_apply" json:"auto_apply"`
	AlsoQASTask         bool     `yaml:"also_qas_task" json:"also_qas_task"`
	UpdateExistingShare bool     `yaml:"update_existing_share" json:"update_existing_share"`
}

type Subscription struct {
	Name        string   `yaml:"name" json:"name"`
	TMDBID      string   `yaml:"tmdb_id" json:"tmdb_id"`
	ContentType string   `yaml:"content_type" json:"content_type"`
	Keywords    []string `yaml:"keywords" json:"keywords"`
	SavePath    string   `yaml:"save_path" json:"save_path"`
	ShareURL    string   `yaml:"share_url" json:"share_url"`
	Sources     []string `yaml:"sources" json:"sources"`
	Enabled     *bool    `yaml:"enabled" json:"enabled"`
	StrmSubdir  string   `yaml:"strm_subdir" json:"strm_subdir"`
	PosterURL   string   `yaml:"poster_url" json:"poster_url"`
}

type Task struct {
	Name       string `yaml:"name" json:"name"`
	SavePath   string `yaml:"save_path" json:"save_path"`
	ShareURL   string `yaml:"share_url" json:"share_url"`
	Passcode   string `yaml:"passcode" json:"passcode"`
	Pattern    string `yaml:"pattern" json:"pattern"`
	Replace    string `yaml:"replace" json:"replace"`
	StrmSubdir string `yaml:"strm_subdir" json:"strm_subdir"`
	Enabled    *bool  `yaml:"enabled" json:"enabled"`
	DoSave     *bool  `yaml:"do_save" json:"do_save"`
}

type Config struct {
	Cookie          string         `yaml:"cookie" json:"cookie"`
	MURL            string         `yaml:"m_url" json:"m_url"`
	MURLFile        string         `yaml:"m_url_file" json:"m_url_file"`
	OpenListDB      string         `yaml:"openlist_db" json:"openlist_db"`
	UseQASTransfer  bool           `yaml:"use_qas_transfer" json:"use_qas_transfer"`
	QASRoot         string         `yaml:"qas_root" json:"qas_root"`
	QASConfig       string         `yaml:"qas_config" json:"qas_config"`
	ImportQASTasks  bool           `yaml:"import_qas_tasks" json:"import_qas_tasks"`
	QASWriteBack    bool           `yaml:"qas_write_back" json:"qas_write_back"`
	Server          ServerCfg      `yaml:"server" json:"server"`
	StrmRoot        string         `yaml:"strm_root" json:"strm_root"`
	VideoExts       []string       `yaml:"video_exts" json:"video_exts"`
	Emby            EmbyCfg        `yaml:"emby" json:"emby"`
	Interval        int            `yaml:"interval_seconds" json:"interval_seconds"`
	Tasks           []Task         `yaml:"tasks" json:"tasks"`
	Subscriptions   []Subscription `yaml:"subscriptions" json:"subscriptions"`
	CategoryFile    string         `yaml:"category_file" json:"category_file"`
	Organize        OrganizeCfg    `yaml:"organize" json:"organize"`
	SubSearch       SubSearchCfg   `yaml:"subscription_search" json:"subscription_search"`
	Mtproto         MtprotoCfg     `yaml:"mtproto" json:"mtproto"`
	Accounts        []string       `yaml:"accounts" json:"accounts"`
	TMDBAPIKey      string         `yaml:"tmdb_api_key" json:"tmdb_api_key"`

	Path string     `yaml:"-" json:"-"`
	mu   sync.Mutex `yaml:"-" json:"-"`
}

func Default() *Config {
	return &Config{
		UseQASTransfer: true,
		QASRoot:        "/app/third_party/quark-auto-save-x",
		QASConfig:      "/app/data/quark_config.json",
		ImportQASTasks: true,
		Server:         ServerCfg{Host: "0.0.0.0", Port: 18025, PublicBase: "http://192.168.10.14:18025"},
		StrmRoot:       "/app/strm",
		VideoExts:      []string{".mp4", ".mkv", ".ts", ".mov", ".m4v", ".avi", ".webm", ".flv"},
		Interval:       1800,
		CategoryFile:   "/app/data/category.yaml",
		Organize: OrganizeCfg{
			AutoOrganize: true, EnableCategory: true, CategoryFile: "/app/data/category.yaml",
			NamingTemplate: NamingTemplate{
				Movie:          `{{title}} ({{year}})/{{title}} ({{year}}) - {{resolution}}{{videoCodec}}{{audioCodec}}{{ext}}`,
				TVShowDir:      `{{title}} ({{year}})`,
				TVSeasonDir:    `Season {{seasonPad}}`,
				TVFile:         `{{title}} - S{{seasonPad}}E{{episodePad}} - {{resolution}}{{videoCodec}}{{audioCodec}}{{ext}}`,
				TVCombo:        `{{title}} ({{year}})/Season {{seasonPad}}/{{title}} - S{{seasonPad}}E{{episodePad}} - {{resolution}}{{videoCodec}}{{audioCodec}}{{ext}}`,
				EnableCategory: true,
			},
		},
		SubSearch:     SubSearchCfg{Enabled: true, OnlyMissingShare: true, ApplyBest: true},
		Mtproto:       MtprotoCfg{SessionPath: "/app/data/mtproto", AutoApply: true, AlsoQASTask: true},
		Tasks:         []Task{},
		Subscriptions: []Subscription{},
		Accounts:      []string{},
	}
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := Default()
	if err := yaml.Unmarshal(b, cfg); err != nil {
		return nil, err
	}
	cfg.Path = path
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 18025
	}
	if cfg.Server.PublicBase == "" {
		cfg.Server.PublicBase = fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port)
	}
	if cfg.StrmRoot == "" {
		cfg.StrmRoot = filepath.Join(filepath.Dir(path), "..", "strm")
	}
	if cfg.CategoryFile == "" {
		cfg.CategoryFile = filepath.Join(filepath.Dir(path), "..", "data", "category.yaml")
	}
	if cfg.QASConfig == "" {
		cfg.QASConfig = filepath.Join(filepath.Dir(path), "..", "data", "quark_config.json")
	}
	if cfg.MURL == "" && cfg.MURLFile != "" {
		if raw, err := os.ReadFile(cfg.MURLFile); err == nil {
			cfg.MURL = string(raw)
		}
	}
	if v := os.Getenv("QM_PUBLIC_BASE"); v != "" {
		cfg.Server.PublicBase = v
	}
	if v := os.Getenv("EMBY_BASE_URL"); v != "" {
		cfg.Emby.BaseURL = v
	}
	if v := os.Getenv("EMBY_API_KEY"); v != "" {
		cfg.Emby.APIKey = v
	}
	if os.Getenv("EMBY_ENABLED") == "true" {
		cfg.Emby.Enabled = true
	}
	if v := os.Getenv("QM_STRM_ROOT"); v != "" {
		cfg.StrmRoot = v
	}
	if cfg.StrmRoot == "" {
		cfg.StrmRoot = "/app/strm"
	}
	return cfg, nil
}

func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Path == "" {
		return fmt.Errorf("config path empty")
	}
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
		return err
	}
	// avoid marshaling zero mutex issues — copy public fields via yaml tags on same struct
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	tmp := c.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.Path)
}

func (c *Config) DataDir() string {
	if c.Path != "" {
		return filepath.Clean(filepath.Join(filepath.Dir(c.Path), "..", "data"))
	}
	return "data"
}

func MaskSecret(s string, keep int) string {
	s = trimSpace(s)
	if s == "" {
		return ""
	}
	if keep < 0 {
		keep = 0
	}
	if len(s) <= keep*2 {
		return "****"
	}
	return s[:keep] + "****" + s[len(s)-keep:]
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 {
		c := s[len(s)-1]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			s = s[:len(s)-1]
			continue
		}
		break
	}
	return s
}

func (c *Config) SettingsPublic() map[string]any {
	return map[string]any{
		"cookie":           "",
		"cookie_masked":    MaskSecret(c.Cookie, 8),
		"cookie_set":       c.Cookie != "",
		"m_url":            "",
		"m_url_masked":     MaskSecret(c.MURL, 24),
		"m_url_set":        c.MURL != "",
		"m_url_file":       c.MURLFile,
		"openlist_db":      c.OpenListDB,
		"qas_root":         c.QASRoot,
		"qas_config":       c.QASConfig,
		"use_qas_transfer": c.UseQASTransfer,
		"import_qas_tasks": c.ImportQASTasks,
		"qas_write_back":   c.QASWriteBack,
		"server": map[string]any{
			"host":        c.Server.Host,
			"port":        c.Server.Port,
			"public_base": c.Server.PublicBase,
		},
		"video_exts":       c.VideoExts,
		"interval_seconds": c.Interval,
		"emby": map[string]any{
			"enabled":       c.Emby.Enabled,
			"base_url":      c.Emby.BaseURL,
			"api_key":       "",
			"api_key_masked": MaskSecret(c.Emby.APIKey, 4),
			"api_key_set":   c.Emby.APIKey != "",
			"path":          c.Emby.Path,
		},
		"config_path":   c.Path,
		"category_file": c.CategoryFile,
		"subscriptions": c.Subscriptions,
		"tmdb_api_key":  "",
		"tmdb_api_key_masked": MaskSecret(c.TMDBAPIKey, 4),
		"tmdb_set":      c.TMDBAPIKey != "",
		"organize":      c.Organize,
		"subscription_search": c.SubSearch,
		"mtproto": map[string]any{
			"enabled":      c.Mtproto.Enabled,
			"api_id":       c.Mtproto.APIID,
			"api_hash_set": c.Mtproto.APIHash != "",
			"phone":        c.Mtproto.Phone,
			"session_path": c.Mtproto.SessionPath,
			"channels":     c.Mtproto.Channels,
			"auto_apply":   c.Mtproto.AutoApply,
			"also_qas_task": c.Mtproto.AlsoQASTask,
			"update_existing_share": c.Mtproto.UpdateExistingShare,
		},
	}
}
