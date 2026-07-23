package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"quark-media/internal/config"
	"quark-media/internal/qas"
	"quark-media/internal/quark"
	"quark-media/internal/server"
)

func main() {
	cfgPath := flag.String("c", envOr("QM_CONFIG", "config/config.yaml"), "config path")
	flag.Parse()
	args := flag.Args()
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}

	abs, _ := filepath.Abs(*cfgPath)
	if _, err := os.Stat(abs); os.IsNotExist(err) {
		seedConfig(abs)
	}
	cfg, err := config.Load(abs)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	_ = os.MkdirAll(cfg.StrmRoot, 0o755)
	_ = os.MkdirAll(filepath.Dir(cfg.QASConfig), 0o755)


	// Hydrate cookie from persistent QAS json if yaml empty (recover after partial saves).
	hydrateFromData(cfg)
	log.Printf("persist config=%s qas=%s strm=%s", cfg.Path, cfg.QASConfig, cfg.StrmRoot)

	client := quark.New(cfg.Cookie, cfg.MURL)

	switch cmd {
	case "serve", "run":
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		log.Printf("Quark Media (Go) listen %s public_base=%s", addr, cfg.Server.PublicBase)
		if err := server.Listen(addr, cfg, client); err != nil {
			log.Fatal(err)
		}
	case "once":
		n, err := server.RunPipeline(cfg, client)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("pipeline ok strm_videos=%d\n", n)
	case "version":
		b, _ := os.ReadFile("VERSION")
		fmt.Print(string(b))
	default:
		log.Fatalf("unknown cmd %s (serve|run|once|version)", cmd)
	}
}

func seedConfig(abs string) {
	cands := []string{
		"config.example.yaml",
		"/app/config.example.yaml",
		filepath.Join(filepath.Dir(abs), "..", "config.example.yaml"),
	}
	var src string
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			src = c
			break
		}
	}
	if src == "" {
		_ = os.MkdirAll(filepath.Dir(abs), 0o755)
		cfg := config.Default()
		cfg.Path = abs
		_ = cfg.Save()
		log.Printf("seeded default config %s", abs)
		return
	}
	_ = os.MkdirAll(filepath.Dir(abs), 0o755)
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.Create(abs)
	if err != nil {
		return
	}
	defer out.Close()
	_, _ = io.Copy(out, in)
	log.Printf("seeded config from %s -> %s", src, abs)
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func hydrateFromData(cfg *config.Config) {
	ex := qas.LoadExtras(cfg.QASConfig)
	if strings.TrimSpace(cfg.Cookie) == "" && len(ex.Cookies) > 0 {
		cfg.Cookie = ex.Cookies[0]
		log.Printf("hydrate cookie from %s (len=%d)", cfg.QASConfig, len(cfg.Cookie))
	}
	if len(cfg.Accounts) == 0 && len(ex.Cookies) > 0 {
		cfg.Accounts = append([]string{}, ex.Cookies...)
	}
	if strings.TrimSpace(cfg.TMDBAPIKey) == "" && strings.TrimSpace(ex.TMDBAPIKey) != "" {
		cfg.TMDBAPIKey = ex.TMDBAPIKey
	}
	// ensure qas path always under /app/data when running in docker
	if strings.TrimSpace(cfg.QASConfig) == "" {
		cfg.QASConfig = "/app/data/quark_config.json"
	}
}
