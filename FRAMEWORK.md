# Quark Media 框架说明

> 原则：**先框架，后功能**。主链路保持薄；能力用插件慢慢加。

## 一句话

```
转存/追更 → 扫库写 STRM → Emby 入库 → 播放走 302（夸克转码 m3u8）
```

不依赖 SmartStrm。转存引擎复用 `quark-auto-save-x` 源码，不重复造。

## 目录结构

```
quark-media/
  main.py                 # 薄入口：CLI 分发
  config.yaml             # 本服务配置
  category.yaml           # 整理分类规则
  FRAMEWORK.md            # 本文件
  app/
    bootstrap.py          # 组装 cfg + client + 插件
    config.py             # 读配置 / cookie / m_url
    quark.py              # 夸克 API（列目录 / play / 简易转存）
    pipeline.py           # 主流水线：钩子 + 扫库写 STRM
    strm.py               # 写 .strm
    server_302.py         # Web 控制台 + API + /play 302
    qas_bridge.py         # 调用 QAS 源码
    qas_settings.py       # QAS 设置读写
    tasks_store.py        # 任务增删改
    tg_inbox.py           # TG 收链 worker
    category.py           # category.yaml
    subscriptions.py      # 订阅骨架
    emby.py               # Emby 刷库
    core/
      registry.py         # Plugin / AppContext / PluginRegistry
    plugins/
      builtin.py          # 内置插件注册
  web/                    # 前端静态资源
  strm/                   # 生成的 STRM
  data/                   # 运行状态
```

## 主链路（不可再乱）

```
config.yaml
    │
    ▼
bootstrap()  ──► load_config + QuarkClient + PluginRegistry
    │
    ├─ once / run 定时
    │     │
    │     ▼
    │  plugin.on_pipeline_before   # 默认：qas_transfer
    │     │
    │     ▼
    │  pipeline 扫库 + 写 STRM      # 唯一「写媒体库文件」的地方
    │     │
    │     ▼
    │  plugin.on_pipeline_after    # 默认：emby
    │
    └─ serve / run
          │
          ▼
       plugin.on_startup           # 默认：tg_inbox
          │
          ▼
       server_302                  # Web + /api/* + /play/{fid} → 302 m3u8
```

### 302 播放

```
Emby 读 xxx.strm
  → http://你的服务/play/{fid}/name.mp4
  → 本服务取夸克转码 play 地址
  → HTTP 302 到 m3u8
  → 播放器直连夸克 CDN（视频流量不经 NAS）
```

注意：原画 download 要 Cookie，**不能**纯 302；当前走转码 m3u8。

## 插件清单

| name | 开关 | 钩子 | 作用 |
|------|------|------|------|
| `category` | 始终可装 | on_register | 加载 category.yaml |
| `subscriptions` | 始终可装 | on_register | 读 subscriptions 骨架 |
| `qas_transfer` | `use_qas_transfer` | on_pipeline_before | QAS do_save |
| `tg_inbox` | QAS 内 TG 开关 | on_startup | 转发收链+整理 |
| `emby` | `emby.enabled` | on_pipeline_after | 刷库 |

查看状态：

```bash
python main.py plugins
```

### 以后加功能怎么做

1. 在 `app/plugins/` 写一个 `Plugin` 子类  
2. 在 `create_builtin_registry()` 注册  
3. 需要配置就加 `config.yaml` 字段 + `enabled_key`  
4. **不要**往 `main.py` / `pipeline.py` 塞大段业务  

## CLI

```bash
cd quark-media
python main.py plugins          # 插件状态
python main.py test-play <fid>  # 测 302 源
python main.py ls [path]        # 列视频
python main.py qas-tasks        # QAS 任务
python main.py qas-save         # 只转存
python main.py once             # 一轮流水线
python main.py serve            # Web+302+后台插件
python main.py run              # 常驻：定时流水线+Web+302
```

浏览器：`http://127.0.0.1:18025/`

## 配置边界

| 配置 | 管什么 |
|------|--------|
| `quark-media/config.yaml` | 端口、public_base、strm_root、本机 tasks、subscriptions、emby、QAS 路径 |
| `quark-auto-save-x/config/quark_config.json` | Cookie 多账号、tasklist、TG 推送/收链、频道源、TMDB、收链根目录 |
| `category.yaml` | 整理分类规则 |

## 功能路线图（慢慢加，别一次堆）

- **P0 框架**（本文件）：registry + bootstrap + 薄入口 ✅  
- **P1 主链路稳**：once / serve / 302 / STRM ✅  
- **P2 转存连贯**：QAS bridge + 任务 Web + 设置 ✅（继续打磨）  
- **P3 订阅可视化**：subscriptions 前端 CRUD  
- **P4 多账号 UI / 原画 proxy**（可选，非纯 302）  
- **P5 单容器打包**：QAS + quark-media 一体镜像  

## 禁止事项（防再乱）

1. 不要在根目录堆 `_w_*.py` / `_fix_*.py` 临时脚本写生产文件  
2. 不要复制一份 QAS 转存逻辑；只 bridge  
3. 不要让 pipeline 同时「自己转存」又「插件转存」两遍（用 `_qas_done_by_plugin`）  
4. 不要把 SmartStrm 塞进依赖  
5. 端口 18025 被旧 `quark_transcode_302.py` 占用时先杀掉  

## 开发约定

- 中文注释 OK；用户面中文  
- 插件失败只 print，不拖垮 serve  
- 改 Web 静态资源后浏览器 **Ctrl+F5**  
- 改 server 需重启 `python main.py run`
