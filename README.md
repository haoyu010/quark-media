# Quark Media (Go)

一体媒体中台：订阅/转存 → STRM → Emby → **夸克 302 m3u8 直链播放**（视频流量不经 NAS）。

## 快速启动（飞牛）
```bash
git clone https://github.com/haoyu010/quark-media.git
cd quark-media
cp .env.example .env
docker compose up -d --build
# Web: http://192.168.10.14:18025
```

## 挂载持久化
- `./config` → 主配置
- `./data` → 订阅/QAS/MTProto session
- `./strm` → STRM 给 Emby 扫描

## 本地
```bash
go run ./cmd/quark-media -c config/config.yaml serve
```

推送 main 自动构建 `ghcr.io/haoyu010/quark-media` 并升小版本。
