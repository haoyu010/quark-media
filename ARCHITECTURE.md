# 能力说明（细节见 FRAMEWORK.md）

单服务闭环：转存/追更 → STRM → Emby → 302 播放。

## 五大能力（插件化，慢慢做满）

1. **夸克多账号** — QAS cookie / 目录 / 转存（二维码登录后续）
2. **订阅追更** — `subscriptions[]` + TMDB + 定时扫库
3. **多源搜链** — TG 频道（QAS source.telegram）
4. **整理分类** — `category.yaml`
5. **STRM + 302** — `/play/{fid}` → 转码 m3u8

## 进程

```
python main.py run
  ├─ 定时 pipeline（插件 before/after + 写 STRM）
  ├─ TG inbox 插件
  └─ Web + 302
```

完整目录、钩子、禁止事项、路线图 → **[FRAMEWORK.md](./FRAMEWORK.md)**
