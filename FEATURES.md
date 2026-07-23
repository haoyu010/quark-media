# Quark Media 功能细化清单

> 原则：UI 后做；先把功能链路跑通、可配置、可验收。  
> 对照：115 / Casby 式一体闭环。

---

## 0. 系统总览（用户感知）

```
资源进来（分享链 / TG 转发 / 订阅）
   → 转存到夸克目录
   → 按规则整理路径
   → 生成 STRM
   → Emby/Jellyfin 入库
   → 播放时 302 到夸克转码 m3u8（视频流量不经 NAS）
```

**服务边界**
| 进程/配置 | 职责 |
|-----------|------|
| `quark-media` | 流水线、STRM、302、Web 控制台、插件调度 |
| `quark-auto-save-x`（源码 bridge） | 转存引擎、多账号 cookie、tasklist、TG 推送/频道、TMDB |
| `category.yaml` | 整理分类规则 |

---

## 1. 功能模块拆解

### F1. 夸克账号与鉴权
| 子功能 | 说明 | 验收标准 | 现状 |
|--------|------|----------|------|
| F1.1 Cookie 登录 | 主账号 cookie 用于 API | `quark_ok=true` | ✅ 可用（openlist/QAS/配置） |
| F1.2 转码签名 m_url | play 接口 kps/sign/vcode | `mparam_ok=true`，test-play 出 m3u8 | ✅ 可用 |
| F1.3 多账号 Cookie | QAS cookie 多行/列表；第一账号转存 | 设置可读写多账号；转存用主号 | ⚠️ 读写代码有，需重启服务验收 |
| F1.4 二维码登录 | 扫码取 cookie | 扫码后 cookie 入库 | ❌ 未做（后续） |

### F2. 任务与转存
| 子功能 | 说明 | 验收标准 | 现状 |
|--------|------|----------|------|
| F2.1 任务列表 | 合并 config.tasks + QAS tasklist | 列表有稳定 `id`，可区分来源 | ⚠️ 代码有 id，**当前旧进程返回 id=null** |
| F2.2 新建任务 | 分享链 + 保存路径 + 规则 | POST 后落盘（config 或 QAS） | ⚠️ 代码有，需验收 |
| F2.3 编辑/删除任务 | 改路径/开关/pattern | PUT/DELETE 生效 | ⚠️ 同上 |
| F2.4 执行转存 | 调 QAS `do_save` | `once` 后文件出现在夸克目录 | ✅ bridge 有 |
| F2.5 仅转存命令 | `python main.py qas-save` | 不写 STRM 也能转存 | ✅ |
| F2.6 防双跑 | 插件转存后 pipeline 不再二次 | 一轮只转存一次 | ✅ `_qas_done_by_plugin` |

### F3. TG 通知与收链
| 子功能 | 说明 | 验收标准 | 现状 |
|--------|------|----------|------|
| F3.1 TG 推送配置 | Bot Token / UserId / 代理 | 设置页读写 QAS `push_config` | ⚠️ 代码有，API 需新进程 |
| F3.2 转存结果通知 | 成功/失败推 TG | 跑任务后收到消息 | 依赖 QAS 推送 |
| F3.3 转发自动收链 | 转发夸克链到 Bot → 建任务 → 转存 | 转发后自动进 tasklist 并转存 | ⚠️ `tg_inbox` 有，需配置后验收 |
| F3.4 自动收链根目录 | `telegram_inbox_media_root` | 无路径时落到根目录+分类 | ⚠️ 设置读写有 |
| F3.5 收链后整理 | TMDB 识剧名 + category 规则出路径 | savepath 类似 `电视剧/xxx/Season 01` | ⚠️ 代码有，依赖 TMDB key |

### F4. 多源搜链 / 频道
| 子功能 | 说明 | 验收标准 | 现状 |
|--------|------|----------|------|
| F4.1 TG 频道源配置 | channels / keywords / proxy | 读写 `source.telegram` | ⚠️ 设置层有 |
| F4.2 失效换源 | `auto_replace` | 分享失效时换新链 | 依赖 QAS 原逻辑 |
| F4.3 订阅触发搜链 | 订阅关键词 + 频道扫描 | 发现新资源自动建任务 | ❌ 仅骨架，未闭环 |

### F5. 订阅追更
| 子功能 | 说明 | 验收标准 | 现状 |
|--------|------|----------|------|
| F5.1 订阅配置 | tmdb_id / keywords / save_path | config.subscriptions 可读 | ⚠️ 骨架 |
| F5.2 定时扫订阅目录 | 有文件则写 STRM | once 覆盖 subscription 路径 | ⚠️ 扫库有，搜新资源无 |
| F5.3 TMDB 元数据 | 剧名/季/类型 | key 可配，识片可用 | ⚠️ key 读写有，业务浅 |
| F5.4 订阅 CRUD API/UI | 增删改订阅 | 后续 | ❌ |

### F6. 整理分类
| 子功能 | 说明 | 验收标准 | 现状 |
|--------|------|----------|------|
| F6.1 category.yaml | 关键词/类型 → 路径模板 | 规则文件生效 | ✅ 5 条规则已加载 |
| F6.2 分类应用到任务 | 无 savepath 时补路径 | TG 收链路径正确 | ⚠️ 需联调 |
| F6.3 分类预览 API | GET `/api/category` | 返回 rules | ⚠️ 代码有，旧进程 404 |

### F7. STRM 生成
| 子功能 | 说明 | 验收标准 | 现状 |
|--------|------|----------|------|
| F7.1 扫夸克目录 | walk 视频扩展名 | ls 列出 fid/path | ✅ |
| F7.2 写 .strm | 内容 = `public_base/play/{fid}/name` | strm 目录出现文件 | ✅ 已有 21 个 |
| F7.3 增量更新 | 内容不变则 skip | 统计 created/updated/skipped | ✅ |
| F7.4 子目录映射 | strm_subdir / 相对路径 | Emby 目录结构合理 | ✅ |

### F8. 302 播放
| 子功能 | 说明 | 验收标准 | 现状 |
|--------|------|----------|------|
| F8.1 `/play/{fid}/...` | 302 → 夸克 m3u8 | curl -I 返回 302 Location | ✅ 逻辑有 |
| F8.2 纯直链 | 浏览器/播放器可播 | 不经 NAS 视频流量 | ✅ 转码链路 |
| F8.3 原画 | 带 cookie 下载 | 非纯 302 | ❌ 明确不做纯 302；后续可选 proxy |
| F8.4 播放测试 | 控制台 test-play | 出 url + 可打开 | ✅ |

### F9. Emby / Jellyfin
| 子功能 | 说明 | 验收标准 | 现状 |
|--------|------|----------|------|
| F9.1 刷库 | Library/Refresh | emby.enabled + api_key | ⚠️ 插件有，默认 OFF |
| F9.2 配置 | base_url / api_key / path | 设置可存 | ✅ 配置字段有 |
| F9.3 指定库路径刷新 | 只刷 STRM 目录 | 减少全库刷新 | ❌ 当前全库 refresh |

### F10. 流水线与常驻
| 子功能 | 说明 | 验收标准 | 现状 |
|--------|------|----------|------|
| F10.1 once | 转存→STRM→Emby | 一条命令跑通 | ✅ |
| F10.2 run | 定时 + Web + 302 | 常驻 | ✅ |
| F10.3 serve | 只 Web/302/后台插件 | 不跑定时 | ✅ |
| F10.4 插件开关 | use_qas / emby.enabled | `python main.py plugins` | ✅ |
| F10.5 手动触发 API | POST `/api/pipeline/run` | 前端「立即跑一轮」 | ✅ |

### F11. 控制台（功能面，非视觉）
| 子功能 | 说明 | 优先级 |
|--------|------|--------|
| F11.1 总览状态 | 夸克/签名/任务数/302 计数 | P1 |
| F11.2 任务 CRUD | 必须有 id，能改能删 | **P0** |
| F11.3 设置读写 | cookie/m_url/QAS/TG/TMDB/收链根/Emby | **P0** |
| F11.4 STRM 列表 | 查看生成结果 | P1 |
| F11.5 日志 | 流水线/302 摘要 | P1 |
| F11.6 UI 美化 | 后置 | P9 |

---

## 2. 当前最大卡点（先修）

1. **运行中的是旧进程**  
   - 现象：`/api/tg-inbox`、`/api/subscriptions`、`/api/category` → 404  
   - `/api/tasks` 的 `id` 为 `null`  
   - `/api/settings` 无 `qas_extras`（TG/TMDB/收链根读不出来）  
   - **处理：杀旧进程，用当前代码 `python main.py run` 重启**

2. **设置与 QAS 双配置**  
   - 本服务：`config.yaml`  
   - 转存/TG/TMDB：`quark_config.json`  
   - 必须保证设置页一次保存写对两边

3. **订阅/搜链未闭环**  
   - 有配置骨架，没有「发现新资源 → 建任务」自动环

---

## 3. 推荐实施顺序（功能优先）

### 阶段 A — 止血可验收（1 个会话内）
1. 重启服务，确认新 API 全绿  
2. 任务：列表有 id → 编辑 → 删除 → 新建 → 落盘  
3. 设置：cookie / m_url / TG / TMDB / 收链根目录 读写回显  
4. `once`：转存（可跳过）+ STRM + 统计正确  
5. `test-play` + `/play/{fid}` 302 验证  

### 阶段 B — 收链连贯（对标 115 日常）
1. TG Bot 配置 + inbox worker 启动状态可见  
2. 转发一条夸克链 → 自动整理路径 → 转存 → 可写 STRM  
3. category 规则联调（电影/剧集路径）  

### 阶段 C — 追更闭环
1. 订阅 CRUD（API + 设置页表格） ✅  
2. 定时：订阅路径扫库写 STRM ✅（run/once pipeline）  
3. 可选：TG 频道搜新链挂到订阅 — 待做  

### 阶段 D — 增强
1. 多账号 UI/切换  
2. Emby 指定路径刷库  
3. 原画 proxy（可选）  
4. 单容器打包  
5. UI 精修  

---

## 4. 每条功能的「最小验收用例」

| 编号 | 操作 | 期望 |
|------|------|------|
| A1 | `python main.py plugins` | 5 插件状态正确 |
| A2 | GET `/api/tasks` | 每条有 `id` 如 `config:0` / `qas:1` |
| A3 | PUT `/api/tasks/config:0` 改名 | 刷新后名称变 |
| A4 | GET `/api/settings` | 含 `qas_extras.push_config` / `tmdb_api_key_set` / `task_settings` |
| A5 | POST settings 写 `telegram_inbox_media_root` | QAS json 落盘 |
| A6 | `python main.py test-play <fid>` | 打印 m3u8 URL |
| A7 | curl -I `/play/<fid>/x.mp4` | 302 + Location 含 m3u8 |
| A8 | `python main.py once` | 返回 total_videos / strm 统计 |
| B1 | TG 转发分享链 | tasklist 新增 + 可选立即转存 |
| B2 | GET `/api/tg-inbox` | `running/enabled/last_event` |

---

## 5. 明确「不做 / 后做」

| 项 | 决定 |
|----|------|
| SmartStrm 依赖 | 不做 |
| 原画纯 302 | 不做（要 Cookie） |
| 转码改原画封装 | 不做；当前就是 m3u8 转码播 |
| UI 大改 | 后置到 D |
| 二维码登录 | 后置 |
| 重写一套转存引擎 | 不做；只 bridge QAS |

---

## 6. 建议你下一句直接点名

回复下面之一即可开工：

1. **`A 止血`** — 重启 + 任务/设置/302/once 全验收修好  
2. **`B 收链`** — TG 转发自动整理转存打通  
3. **`C 订阅`** — 订阅追更闭环  
4. **`全 A→B`** — 按顺序一口气做  

默认推荐：**先 A 止血**（否则网页上很多功能「像没做」其实是旧进程）。
