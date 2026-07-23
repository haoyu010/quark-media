
const $ = (s, r=document) => r.querySelector(s);
const $$ = (s, r=document) => [...r.querySelectorAll(s)];
const TITLES = {
  dashboard: ["总览", "运行状态与关键总览"],
  subs: ["订阅追更", "TMDB 海报墙 · 一键订阅"],
  mysubs: ["我的订阅", "已入库订阅 · 搜链补资源 · 管理"],
  tasks: ["任务", "转存任务（完成后写 STRM）"],
  strm: ["STRM 库", "转存后生成的播放索引"],
  play: ["播放测试", "验证 302 → m3u8 直链"],
  settings: ["设置", "账号 / QAS / 302 / Emby 配置"],
  logs: ["日志", "流水线与 302 运行摘要"],
};

function toast(msg, type = "info") {
  const el = $("#toast");
  if (!el) return;
  el.textContent = msg;
  el.classList.remove("ok", "err", "warn", "info", "show");
  const t = ["ok", "err", "warn", "info"].includes(type) ? type : "info";
  el.classList.add(t, "show");
  clearTimeout(toast._t);
  toast._t = setTimeout(() => el.classList.remove("show"), type === "err" ? 3600 : 2400);
}
function setBusy(on) {
  const bar = $("#top-progress");
  const dot = $("#busy-dot");
  if (bar) bar.hidden = !on;
  if (dot) dot.hidden = !on;
  document.body.classList.toggle("is-busy", !!on);
}
function emptyBox(title, hint = "") {
  return `<div class="empty"><strong>${esc(title)}</strong>${hint ? `<div class="hint">${esc(hint)}</div>` : ""}</div>`;
}
function emptyRow(cols, title, hint = "") {
  return `<tr class="table-empty"><td colspan="${cols}">${emptyBox(title, hint)}</td></tr>`;
}
function esc(s) {
  return String(s ?? "")
    .replaceAll("&","&amp;").replaceAll("<","&lt;")
    .replaceAll(">","&gt;").replaceAll('"',"&quot;");
}
async function api(path, opts) {
  setBusy(true);
  try {
    const r = await fetch(path, opts);
    const text = await r.text();
    let data; try { data = JSON.parse(text); } catch { data = { raw: text }; }
    if (!r.ok) throw new Error(data.error || data.message || text || r.statusText);
    return data;
  } finally {
    clearTimeout(api._busyT);
    api._busyT = setTimeout(() => setBusy(false), 120);
  }
}
function badge(ok, a, b) {
  return `<span class="badge ${ok ? "ok" : "bad"}">${ok ? a : b}</span>`;
}


function initTheme() {
  const saved = localStorage.getItem("qm_theme") || localStorage.getItem("qm-theme");
  let theme = saved;
  if (theme === "" || theme == null) theme = document.documentElement.getAttribute("data-theme") || "dark";
  if (theme !== "light" && theme !== "dark") theme = "dark";
  document.documentElement.setAttribute("data-theme", theme);
  localStorage.setItem("qm_theme", theme);
}
function toggleTheme() {
  const cur = document.documentElement.getAttribute("data-theme") === "light" ? "light" : "dark";
  const next = cur === "dark" ? "light" : "dark";
  document.documentElement.setAttribute("data-theme", next);
  localStorage.setItem("qm_theme", next);
  localStorage.setItem("qm-theme", next === "dark" ? "dark" : "");
  toast(next === "dark" ? "已切换深色主题" : "已切换浅色主题", "ok");
}

function setPage(name) {
  if (!TITLES[name]) name = "dashboard";
  $$(".nav-item").forEach(b => b.classList.toggle("active", b.dataset.page === name));
  $$(".page").forEach(p => p.classList.toggle("show", p.id === `page-${name}`));
  $("#crumb").textContent = TITLES[name][0];
  $("#top-desc").textContent = TITLES[name][1];
  location.hash = name;
  $("#app .sidebar").classList.remove("open");
  $("#scrim").hidden = true;
  if (name === "subs") {
    loadDiscover().catch(() => {});
    loadChannelStatus().catch(() => {});
  }
  if (name === "mysubs") {
    loadSubscriptions().catch(() => {});
    loadChannelStatus().catch(() => {});
  }
  if (name === "settings") {
    loadTgStatus().catch(() => {});
  }
}

function setSettingsSec(sec) {
  $$("#settings-nav button").forEach(b => b.classList.toggle("on", b.dataset.sec === sec));
  $$(".settings-sec").forEach(s => s.classList.toggle("show", s.id === `sec-${sec}`));
  if (sec === "accounts") loadAccounts().catch(() => {});
  if (sec === "emby") loadEmbyFolders().catch(() => {});
  if (sec === "tg") loadTgStatus().catch(() => {});
  if (sec === "mtproto") loadMtprotoStatus().catch(() => {});
}

async function loadStatus() {
  const s = await api("/api/status");
  $("#foot-port").textContent = `:${s.port || 18025}`;
  $("#nav-task-count").textContent = String(s.task_count ?? 0);

  const tg = s.tg_inbox || {};
  const chips = [
    { ok: s.quark_ok, text: s.quark_ok ? "夸克 API 正常" : "夸克未就绪" },
    { ok: s.mparam_ok, text: s.mparam_ok ? "转码签名就绪" : "缺少 m_url 签名" },
    {
      ok: s.emby_configured ? s.emby_ok : false,
      warn: !s.emby_configured || (s.emby_configured && !s.emby_ok),
      text: !s.emby_configured ? "Emby 未配置" : (s.emby_ok ? "Emby 已连接" : "Emby 不可达"),
      none: !s.emby_configured
    },
    {
      ok: !!tg.running,
      warn: !tg.running,
      text: tg.running ? "TG 收链运行中" : (tg.enabled ? "TG 收链未启动" : "TG 收链未开启"),
      none: !tg.enabled && !tg.running
    },
    { ok: true, text: "302 服务运行中" },
    {
      ok: !!(s.mtproto && s.mtproto.running),
      warn: !(s.mtproto && s.mtproto.running),
      text: (s.mtproto && s.mtproto.running) ? "MTProto 监控中" : ((s.mtproto && s.mtproto.enabled) ? "MTProto 未启动" : "MTProto 未开启"),
      none: !(s.mtproto && (s.mtproto.enabled || s.mtproto.running))
    },
  ];
  $("#status-chips").innerHTML = chips.map(c => {
    const cls = c.none ? "warn" : c.ok ? "ok" : c.warn ? "warn" : "bad";
    return `<span class="chip ${cls}">${c.text}</span>`;
  }).join("");

  $("#metric-cards").innerHTML = `
    <div class="metric m1"><div class="label">STRM 总数</div><div class="value">${s.strm_count ?? 0}</div><div class="meta"><span class="pill">仅索引</span><span class="pill">无视频实体</span></div></div>
    <div class="metric m2"><div class="label">任务数</div><div class="value">${s.task_count ?? 0}</div><div class="meta"><span class="pill">QAS + 本地</span><span class="pill">可编辑</span></div></div>
    <div class="metric m3"><div class="label">今日 302</div><div class="value">${s.redirects_today ?? 0}</div><div class="meta"><span class="pill">累计 ${s.redirects_total ?? 0}</span><span class="pill">直连 CDN</span></div></div>
    <div class="metric m4"><div class="label">最近流水线</div><div class="value">${s.last_run_text || "尚未运行"}</div><div class="meta"><span class="pill">视频 ${s.last_videos ?? 0}</span><span class="pill">错误 ${s.redirect_errors ?? 0}</span></div></div>`;

  $("#conn-rows").innerHTML = `
    <div class="row"><span>夸克登录</span>${badge(!!s.quark_ok,"已连接","异常")}</div>
    <div class="row"><span>转码签名</span>${badge(!!s.mparam_ok,"就绪","缺失")}</div>
    <div class="row"><span>302 服务</span>${badge(true,"运行中","未运行")}</div>
    <div class="row"><span>TG 收链</span>${tg.running ? badge(true,"运行中","未运行") : (tg.enabled ? '<span class="badge warn">未启动</span>' : '<span class="badge warn">未开启</span>')}</div>
    <div class="row"><span>活动账号</span><span>${esc((s.active_account && s.active_account.name) || (s.accounts_count ? ("#"+s.active_account_index) : "-"))} / ${s.accounts_count ?? 0}</span></div>
    <div class="row"><span>Emby</span>${s.emby_configured ? badge(!!s.emby_ok,"可访问","不可达") : '<span class="badge warn">未配置</span>'}</div>`;

  $("#stat-rows").innerHTML = `
    <div class="row"><span>任务数</span><span>${s.task_count ?? 0}</span></div>
    <div class="row"><span>STRM 文件</span><span>${s.strm_count ?? 0}</span></div>
    <div class="row"><span>今日 302</span><span>${s.redirects_today ?? 0}</span></div>
    <div class="row"><span>累计 302</span><span>${s.redirects_total ?? 0}</span></div>
    <div class="row"><span>上次执行</span><span>${s.last_run_text || "-"}</span></div>`;

  $("#svc-rows").innerHTML = `
    <div class="row"><span>public_base</span><span>${esc(s.public_base || "-")}</span></div>
    
    <div class="row"><span>端口</span><span>${s.port || "-"}</span></div>
    <div class="row"><span>定时间隔</span><span>${s.interval_seconds || "-"}s</span></div>`;

  $("#redirect-stats").innerHTML = `
    <div class="mini"><div class="n">${s.redirects_today ?? 0}</div><div class="t">今日播放</div></div>
    <div class="mini"><div class="n">${s.redirects_total ?? 0}</div><div class="t">累计跳转</div></div>
    <div class="mini"><div class="n">${s.redirect_errors ?? 0}</div><div class="t">失败</div></div>`;

  if ((s.recent_tasks || []).length) {
    $("#task-preview").className = "preview-list";
    $("#task-preview").innerHTML = s.recent_tasks.map(t =>
      `<div class="preview-item"><div><b>${esc(t.name || "-")}</b><span>${esc(t.save_path || "-")}</span></div><span class="tag-src ${(t.source||"").includes("qas")?"qas":"config"}">${(t.source||"").includes("qas")?"QAS":"本地"}</span></div>`
    ).join("");
  } else {
    $("#task-preview").className = "empty-wrap";
    $("#task-preview").innerHTML = emptyBox("暂无任务", "去「任务」新建，或配置 QAS tasklist");
  }

  if ((s.recent_strm || []).length) {
    $("#recent-strm").className = "preview-list";
    $("#recent-strm").innerHTML = s.recent_strm.slice(0,6).map(x =>
      `<div class="preview-item"><div><b>${esc(x.name || x.rel || "-")}</b><span>${esc(x.rel || "")}</span></div></div>`
    ).join("");
  } else {
    $("#recent-strm").className = "empty-wrap";
    $("#recent-strm").innerHTML = emptyBox("暂无 STRM", "点右上角「立即跑一轮」生成索引");
  }
}

let TASKS_CACHE = [];

async function loadTasks() {
  const data = await api("/api/tasks");
  TASKS_CACHE = data.tasks || [];
  const tb = $("#tasks-body");
  if (!TASKS_CACHE.length) {
    tb.innerHTML = emptyRow(7, "暂无任务", "点击「新建任务」开始");
    return;
  }
  tb.innerHTML = TASKS_CACHE.map(t => {
    if (t.source === "error") {
      return `<tr><td colspan="7" style="color:var(--bad)">${esc(t.name)}</td></tr>`;
    }
    const srcClass = (t.source || "").includes("qas") || t.source === "quark-auto-save-x" ? "qas" : "config";
    const srcLabel = srcClass === "qas" ? "QAS" : "本地";
    return `
    <tr data-id="${esc(t.id)}">
      <td><b>${esc(t.name || "-")}</b></td>
      <td>${esc(t.save_path || "-")}</td>
      <td title="${esc(t.share_url || "")}">${t.share_url ? esc(t.share_url.length > 36 ? t.share_url.slice(0,36)+"…" : t.share_url) : "-"}</td>
      <td>${esc(t.pattern || "-")}</td>
      <td>${t.enabled === false ? '<span class="badge bad">停用</span>' : '<span class="badge ok">启用</span>'}</td>
      <td><span class="tag-src ${srcClass}">${srcLabel}</span></td>
      <td>
        <div class="ops">
          <button class="btn sm soft" data-edit="${esc(t.id)}" type="button">编辑</button>
          <button class="btn sm soft" data-del="${esc(t.id)}" type="button">删除</button>
        </div>
      </td>
    </tr>`;
  }).join("");

  tb.querySelectorAll("[data-edit]").forEach(btn => {
    btn.onclick = () => openTaskModal(btn.getAttribute("data-edit"));
  });
  tb.querySelectorAll("[data-del]").forEach(btn => {
    btn.onclick = () => deleteTask(btn.getAttribute("data-del"));
  });
}

function openTaskModal(id) {
  const modal = $("#task-modal");
  const isEdit = !!id;
  $("#task-modal-title").textContent = isEdit ? "编辑任务" : "新建任务";
  $("#task-modal-sub").textContent = isEdit ? `ID: ${id}` : "保存后立即写入配置文件";
  $("#task-id").value = id || "";
  const sourceSel = $("#task-source");
  sourceSel.disabled = isEdit;

  if (isEdit) {
    const t = TASKS_CACHE.find(x => x.id === id);
    if (!t) return toast("任务不存在");
    const isQas = (t.source || "").includes("qas") || t.source === "quark-auto-save-x";
    sourceSel.value = isQas ? "qas" : "config";
    $("#task-name").value = t.name || "";
    $("#task-savepath").value = t.save_path || "";
    $("#task-shareurl").value = t.share_url || "";
    $("#task-passcode").value = t.passcode || "";
    $("#task-strm").value = t.strm_subdir || "";
    $("#task-pattern").value = t.pattern || "";
    $("#task-replace").value = t.replace || "";
    $("#task-enabled").checked = t.enabled !== false;
    $("#task-dosave").checked = t.do_save !== false;
  } else {
    sourceSel.value = "qas";
    $("#task-name").value = "";
    $("#task-savepath").value = "";
    $("#task-shareurl").value = "";
    $("#task-passcode").value = "";
    $("#task-strm").value = "";
    $("#task-pattern").value = "";
    $("#task-replace").value = "";
    $("#task-enabled").checked = true;
    $("#task-dosave").checked = true;
  }
  modal.hidden = false;
}

function closeTaskModal() {
  $("#task-modal").hidden = true;
}

function collectTaskForm() {
  return {
    source: $("#task-source").value,
    name: $("#task-name").value.trim(),
    save_path: $("#task-savepath").value.trim(),
    share_url: $("#task-shareurl").value.trim(),
    passcode: $("#task-passcode").value.trim(),
    strm_subdir: $("#task-strm").value.trim(),
    pattern: $("#task-pattern").value,
    replace: $("#task-replace").value,
    enabled: $("#task-enabled").checked,
    do_save: $("#task-dosave").checked,
  };
}

async function saveTaskForm(e) {
  if (e) e.preventDefault();
  const id = $("#task-id").value.trim();
  const body = collectTaskForm();
  if (!body.name && !body.save_path && !body.share_url) {
    return toast("请填写名称/路径/分享链接");
  }
  toast(id ? "正在更新…" : "正在创建…");
  if (id) {
    await api("/api/tasks/" + encodeURIComponent(id), {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  } else {
    await api("/api/tasks", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  }
  closeTaskModal();
  await loadTasks();
  await loadStatus();
  toast("任务已保存", "ok");
}

async function deleteTask(id) {
  if (!id) return;
  if (!confirm("确认删除该任务？此操作会写回配置文件。")) return;
  await api("/api/tasks/" + encodeURIComponent(id) + "/delete", { method: "POST" });
  await loadTasks();
  await loadStatus();
  toast("已删除", "ok");
}

async function loadStrm() {
  const data = await api("/api/strm");
  $("#strm-root-info").innerHTML = `<span class="pill-info">数量 <b>${data.items?.length || 0}</b></span>`; $("#strm-root-info").hidden = false;
  const tb = $("#strm-body");
  if (!data.items?.length) {
    tb.innerHTML = `<tr><td colspan="3" style="color:var(--muted)">暂无 STRM</td></tr>`;
    return;
  }
  tb.innerHTML = data.items.map(it => `
    <tr>
      <td>${esc(it.rel)}</td>
      <td>${esc(it.url)}</td>
      <td>
        <button class="btn sm soft" data-copy="${encodeURIComponent(it.url || "")}" type="button">复制</button>
        <a class="btn sm primary" href="${esc(it.url || "#")}" target="_blank" rel="noopener">打开</a>
      </td>
    </tr>`).join("");
  tb.querySelectorAll("[data-copy]").forEach(btn => {
    btn.onclick = async () => {
      await navigator.clipboard.writeText(decodeURIComponent(btn.dataset.copy || ""));
      toast("已复制播放地址");
    };
  });
}

function sanitizePreview(data) {
  const d = JSON.parse(JSON.stringify(data || {}));
  if (d.cookie) d.cookie = d.cookie_masked || "***";
  if (d.m_url) d.m_url = d.m_url_masked || "***";
  if (d.emby?.api_key) d.emby.api_key = d.emby.api_key_masked || "***";
  delete d.cookie_masked; delete d.m_url_masked;
  if (d.emby) delete d.emby.api_key_masked;
  if (d.qas_extras) {
    if (d.qas_extras.tmdb_api_key) d.qas_extras.tmdb_api_key = d.qas_extras.tmdb_api_key_masked || "***";
    if (d.qas_extras.push_config?.TG_BOT_TOKEN) {
      d.qas_extras.push_config.TG_BOT_TOKEN = d.qas_extras.push_config.TG_BOT_TOKEN_masked || "***";
    }
  }
  return d;
}
function fillSettingsForm(d) {
  $("#set-cookie").value = "";
  $("#set-murl").value = "";
  $("#cookie-hint").textContent = d.cookie_set ? `已配置 ${d.cookie_masked || ""}` : "未配置";
  $("#murl-hint").textContent = d.m_url_set ? `已配置 ${d.m_url_masked || ""}` : "未配置";
  $("#set-murl-file").value = d.m_url_file || "";
  $("#set-openlist-db").value = d.openlist_db || "";
  $("#set-use-qas").checked = !!d.use_qas_transfer;
  $("#set-import-qas").checked = !!d.import_qas_tasks;
  $("#set-qas-writeback").checked = !!d.qas_write_back;
  $("#set-qas-root").value = d.qas_root || "";
  $("#set-qas-config").value = d.qas_config || "";
  $("#set-host").value = d.server?.host || "0.0.0.0";
  $("#set-port").value = d.server?.port || 18025;
  $("#set-public-base").value = d.server?.public_base || "";
  $("#set-interval").value = d.interval_seconds || 1800;
    $("#set-video-exts").value = Array.isArray(d.video_exts) ? d.video_exts.join(",") : (d.video_exts || "");
  $("#set-emby-enabled").checked = !!d.emby?.enabled;
  $("#set-emby-url").value = d.emby?.base_url || "";
  $("#set-emby-key").value = "";
  $("#emby-key-hint").textContent = d.emby?.api_key_set ? `已配置 ${d.emby.api_key_masked || ""}` : "未配置";
  $("#set-emby-path").value = d.emby?.path || "";
  $("#set-config-path").value = d.config_path || "";

  // QAS 扩展：TG / TMDB / 收链
  const x = d.qas_extras || {};
  const pc = x.push_config || {};
  const src = x.telegram_source || {};
  const ts = x.task_settings || {};

  const el = (id) => document.getElementById(id);
  if (el("set-tg-enabled")) el("set-tg-enabled").checked = !!pc.TG_ENABLED;
  if (el("set-tg-inbox")) el("set-tg-inbox").checked = !!pc.TG_INBOX_AUTO_CREATE;
  if (el("set-tg-sign")) el("set-tg-sign").checked = !!pc.QUARK_SIGN_NOTIFY;
  if (el("set-tg-token")) el("set-tg-token").value = "";
  if (el("tg-token-hint")) el("tg-token-hint").textContent = pc.TG_BOT_TOKEN_set ? `已配置 ${pc.TG_BOT_TOKEN_masked || ""}` : "未配置";
  if (el("set-tg-userid")) el("set-tg-userid").value = pc.TG_USER_ID || "";
  if (el("set-push-notify-type")) el("set-push-notify-type").value = x.push_notify_type || "full";
  if (el("set-tg-api-host")) el("set-tg-api-host").value = pc.TG_API_HOST || "";
  if (el("set-tg-proxy-host")) el("set-tg-proxy-host").value = pc.TG_PROXY_HOST || "";
  if (el("set-tg-proxy-port")) el("set-tg-proxy-port").value = pc.TG_PROXY_PORT || "";
  if (el("set-tg-proxy-auth")) el("set-tg-proxy-auth").value = pc.TG_PROXY_AUTH || "";
  if (el("set-tg-inbox-root")) el("set-tg-inbox-root").value = ts.telegram_inbox_media_root || "";

  if (el("set-src-enabled")) el("set-src-enabled").checked = src.enabled !== false;
  if (el("set-src-replace")) el("set-src-replace").checked = src.auto_replace !== false;
  if (el("set-src-channels")) el("set-src-channels").value = Array.isArray(src.channels) ? src.channels.join("\n") : (src.channels || "");
  if (el("set-src-keywords")) el("set-src-keywords").value = Array.isArray(src.keywords) ? src.keywords.join(",") : (src.keywords || "");
  if (el("set-src-proxy")) el("set-src-proxy").value = src.proxy || "";
  if (el("set-src-read")) el("set-src-read").value = src.read_limit ?? 99;
  if (el("set-src-deep")) el("set-src-deep").value = src.deep_limit ?? 600;
  if (el("set-src-verify")) el("set-src-verify").value = src.verify_limit ?? 5;

  if (el("set-tmdb-key")) el("set-tmdb-key").value = "";
  if (el("tmdb-hint")) el("tmdb-hint").textContent = x.tmdb_api_key_set ? `已配置 ${x.tmdb_api_key_masked || ""}` : "未配置";

  if (el("set-cookies-text")) el("set-cookies-text").value = x.cookies_text || "";
  if (el("accounts-count")) el("accounts-count").textContent = `${x.accounts_count || 0} 个账号`;
  if (el("set-category-file")) el("set-category-file").value = d.category_file || "";

  // async previews
  api("/api/subscriptions").then(s => {
    const box = null && el("subs-preview");
    if (box) box.textContent = JSON.stringify(s.subscriptions || [], null, 2);
  }).catch(()=>{});
  api("/api/category").then(c => {
    const box = el("category-preview");
    if (box) box.textContent = JSON.stringify(c, null, 2);
  }).catch(()=>{});

  $("#settings-preview").textContent = JSON.stringify(sanitizePreview(d), null, 2);
}
async function loadSettings() {
  const data = await api("/api/settings");
  fillSettingsForm(data);
}
function collectSettingsPatch() {
  const el = (id) => document.getElementById(id);
  const val = (id) => (el(id) ? el(id).value.trim() : "");
  const chk = (id) => !!(el(id) && el(id).checked);

  const patch = {
    cookie: val("set-cookie"),
    m_url: val("set-murl"),
    m_url_file: val("set-murl-file"),
    openlist_db: val("set-openlist-db"),
    use_qas_transfer: chk("set-use-qas"),
    import_qas_tasks: chk("set-import-qas"),
    qas_write_back: chk("set-qas-writeback"),
    qas_root: val("set-qas-root"),
    qas_config: val("set-qas-config"),
    interval_seconds: Number(val("set-interval") || 1800),
    video_exts: val("set-video-exts"),
    server: {
      host: val("set-host"),
      port: Number(val("set-port") || 18025),
      public_base: val("set-public-base"),
    },
    emby: {
      enabled: chk("set-emby-enabled"),
      base_url: val("set-emby-url"),
      api_key: val("set-emby-key"),
      path: val("set-emby-path"),
    },
    category_file: val("set-category-file"),
    qas_extras: {
      cookies_text: val("set-cookies-text"),
      tmdb_api_key: val("set-tmdb-key"),
      push_notify_type: val("set-push-notify-type") || "full",
      push_config: {
        TG_ENABLED: chk("set-tg-enabled"),
        TG_BOT_TOKEN: val("set-tg-token"),
        TG_USER_ID: val("set-tg-userid"),
        TG_API_HOST: val("set-tg-api-host"),
        TG_PROXY_HOST: val("set-tg-proxy-host"),
        TG_PROXY_PORT: val("set-tg-proxy-port"),
        TG_PROXY_AUTH: val("set-tg-proxy-auth"),
        TG_INBOX_AUTO_CREATE: chk("set-tg-inbox"),
        QUARK_SIGN_NOTIFY: chk("set-tg-sign"),
      },
      telegram_source: {
        enabled: chk("set-src-enabled"),
        auto_replace: chk("set-src-replace"),
        channels: val("set-src-channels"),
        keywords: val("set-src-keywords"),
        proxy: val("set-src-proxy"),
        read_limit: Number(val("set-src-read") || 99),
        deep_limit: Number(val("set-src-deep") || 600),
        verify_limit: Number(val("set-src-verify") || 5),
      },
      task_settings: {
        telegram_inbox_media_root: val("set-tg-inbox-root"),
      },
    },
  };
  return patch;
}
async function saveSettings() {
  const patch = collectSettingsPatch();
  if (!patch.cookie) delete patch.cookie;
  if (!patch.m_url) delete patch.m_url;
  if (!patch.emby.api_key) delete patch.emby.api_key;
  if (patch.qas_extras) {
    if (!patch.qas_extras.tmdb_api_key) delete patch.qas_extras.tmdb_api_key;
    if (patch.qas_extras.push_config && !patch.qas_extras.push_config.TG_BOT_TOKEN) {
      delete patch.qas_extras.push_config.TG_BOT_TOKEN;
    }
  }
  toast("正在保存…");
  const res = await api("/api/settings", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(patch),
  });
  fillSettingsForm(res.settings || {});
  toast("设置已保存（含 TG/TMDB/收链）", "ok");
  await loadStatus();
}

async function loadLogs() {
  const data = await api("/api/logs");
  $("#logs-view").textContent = (data.logs || []).join("\n") || "暂无日志";
}
async function runOnce() {
  toast("流水线执行中…");
  const res = await api("/api/pipeline/run", { method: "POST" });
  toast(`转存+STRM 完成：${res.total_videos || 0} 个`);
  await refreshAll();
}
async function testPlay() {
  const fid = $("#play-fid").value.trim();
  if (!fid) return toast("请输入 fid");
  $("#play-result").textContent = "请求中…";
  try {
    const res = await api("/api/test-play?fid=" + encodeURIComponent(fid));
    $("#play-result").textContent = JSON.stringify(res, null, 2);
    toast(res.ok ? "直链获取成功" : "失败");
  } catch (e) {
    $("#play-result").textContent = String(e);
    toast("测试失败");
  }
}
async function refreshAll() {
  try {
    await loadStatus();
    await Promise.all([loadTasks(), loadStrm(), loadSettings(), loadLogs(), loadSubscriptions().catch(() => {})]);
  } catch (e) {
    toast("刷新失败: " + e.message, "err");
  }
}


async function loadTgStatus() {
  const s = await api("/api/tg-inbox");
  const box = document.getElementById("tg-status-rows");
  if (!box) return s;
  const miss = (s.missing || []).join(", ") || "-";
  box.innerHTML = `
    <div class="row"><span>收链开关</span>${s.enabled ? badge(true,"条件满足","未满足") : '<span class="badge warn">未满足</span>'}</div>
    <div class="row"><span>Worker</span>${s.running ? badge(true,"运行中","已停止") : badge(false,"运行中","已停止")}</div>
    <div class="row"><span>Token</span>${s.has_token ? badge(true,"已配置","缺失") : badge(false,"已配置","缺失")}</div>
    <div class="row"><span>User ID</span>${s.has_user ? badge(true,"已配置","缺失") : badge(false,"已配置","缺失")}</div>
    <div class="row"><span>收链根目录</span><span>${esc(s.inbox_root || "(空=库根)")}</span></div>
    <div class="row"><span>缺少项</span><span>${esc(miss)}</span></div>
    <div class="row"><span>最近事件</span><span>${esc(s.last_event || "-")}</span></div>
    <div class="row"><span>最近错误</span><span>${esc(s.last_error || "-")}</span></div>`;
  return s;
}

async function tgAction(kind) {
  const path = kind === "test" ? "/api/tg-inbox/test" : `/api/tg-inbox/${kind}`;
  const data = await api(path, { method: "POST" });
  if (kind === "test") {
    if (data.ok) toast(`Bot 正常 @${data.bot_username || data.bot_name || ""}`);
    else toast(data.error || "Bot 测试失败");
  } else {
    toast(data.message || (data.ok ? "完成" : (data.error || "完成")));
  }
  await loadTgStatus();
  await loadStatus().catch(() => {});
  return data;
}



let SUBS_CACHE = [];

async function loadSubscriptions() {
  const data = await api("/api/subscriptions");
  SUBS_CACHE = data.subscriptions || [];
  const subCount = document.getElementById("nav-sub-count");
  if (subCount) subCount.textContent = String(SUBS_CACHE.length);
  const tb = document.getElementById("subs-body");
  if (!tb) return SUBS_CACHE;
  if (!SUBS_CACHE.length) {
    tb.innerHTML = emptyRow(8, "暂无订阅", "去「订阅追更」海报墙一键订阅，或点「新建订阅」");
    return SUBS_CACHE;
  }
  tb.innerHTML = SUBS_CACHE.map(s => {
    const share = s.share_url || s.shareurl || "";
    const shareShort = share ? (share.length > 28 ? share.slice(0, 28) + "…" : share) : "未配置";
    const poster = s.poster_url
      ? `<img class="sub-thumb" src="${esc(s.poster_url)}" alt="" loading="lazy" />`
      : `<div class="sub-thumb ph">无图</div>`;
    return `
    <tr data-id="${esc(s.id)}">
      <td>${poster}</td>
      <td><b>${esc(s.name || "-")}</b></td>
      <td>${esc(s.content_type || "-")}</td>
      <td title="${esc(s.save_path || "")}">${esc(s.save_path || "-")}</td>
      <td title="${esc(share)}"><span class="${share ? "" : "muted"}">${esc(shareShort)}</span></td>
      <td>${esc(s.tmdb_id || "-")}</td>
      <td>${s.enabled === false ? '<span class="badge bad">停用</span>' : '<span class="badge ok">启用</span>'}</td>
      <td>
        <div class="ops">
          <button class="btn sm soft" type="button" data-sub-search="${esc(s.id)}">搜链</button>
          <button class="btn sm soft" type="button" data-sub-edit="${esc(s.id)}">编辑</button>
          <button class="btn sm soft" type="button" data-sub-del="${esc(s.id)}">删除</button>
        </div>
      </td>
    </tr>`;
  }).join("");

  tb.querySelectorAll("[data-sub-edit]").forEach(btn => {
    btn.onclick = () => openSubModal(btn.getAttribute("data-sub-edit"));
  });
  tb.querySelectorAll("[data-sub-del]").forEach(btn => {
    btn.onclick = () => deleteSubscription(btn.getAttribute("data-sub-del"));
  });
  tb.querySelectorAll("[data-sub-search]").forEach(btn => {
    btn.onclick = () => openSubSearch(btn.getAttribute("data-sub-search")).catch(e => toast(e.message, "err"));
  });
  return SUBS_CACHE;
}


function openSubModal(id) {
  const modal = $("#sub-modal");
  if (!modal) return;
  const isEdit = !!id;
  $("#sub-modal-title").textContent = isEdit ? "编辑订阅" : "新建订阅";
  $("#sub-id").value = id || "";
  let s = { enabled: true, content_type: "tv", sources: ["telegram", "qas"] };
  if (isEdit) {
    s = SUBS_CACHE.find(x => x.id === id) || s;
  }
  $("#sub-name").value = s.name || "";
  $("#sub-type").value = s.content_type || "tv";
  $("#sub-savepath").value = s.save_path || "";
  $("#sub-shareurl").value = s.share_url || "";
  $("#sub-tmdb").value = s.tmdb_id || "";
  $("#sub-strm").value = s.strm_subdir || "";
  $("#sub-keywords").value = Array.isArray(s.keywords) ? s.keywords.join(",") : (s.keywords || "");
  $("#sub-sources").value = Array.isArray(s.sources) ? s.sources.join(",") : (s.sources || "telegram,qas");
  $("#sub-enabled").checked = s.enabled !== false;
  modal.hidden = false;
}

function closeSubModal() {
  const modal = $("#sub-modal");
  if (modal) modal.hidden = true;
}

function collectSubForm() {
  const form = document.getElementById("sub-form");
  return {
    name: $("#sub-name").value.trim(),
    content_type: $("#sub-type").value,
    save_path: $("#sub-savepath").value.trim(),
    share_url: $("#sub-shareurl").value.trim(),
    tmdb_id: $("#sub-tmdb").value.trim(),
    strm_subdir: $("#sub-strm").value.trim(),
    keywords: $("#sub-keywords").value.trim(),
    sources: $("#sub-sources").value.trim(),
    enabled: $("#sub-enabled").checked,
    poster_url: (form && form.dataset.posterUrl) || "",
  };
}

async function saveSubForm(e) {
  if (e) e.preventDefault();
  const id = $("#sub-id").value.trim();
  const body = collectSubForm();
  if (!body.name && !body.save_path && !body.share_url) {
    return toast("请填写名称/路径/分享链接");
  }
  let res = null;
  if (id) {
    res = await api("/api/subscriptions/" + encodeURIComponent(id), {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    closeSubModal();
    await loadSubscriptions();
    toast("订阅已保存", "ok");
  } else {
    res = await api("/api/subscriptions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    closeSubModal();
    await loadSubscriptions();
    const cs = (res && res.channel_search) || null;
    if (cs && cs.applied) toast("订阅已保存 · 已自动搜到分享链", "ok");
    else if (cs && !cs.skipped && (cs.count === 0 || cs.error)) toast("订阅已保存 · 频道暂无匹配/搜链失败", "warn");
    else toast("订阅已保存", "ok");
  }
}

async function deleteSubscription(id) {
  if (!id) return;
  if (!confirm("确认删除该订阅？")) return;
  await api("/api/subscriptions/" + encodeURIComponent(id) + "/delete", { method: "POST" });
  await loadSubscriptions();
  toast("已删除", "ok");
}



function _setChannelStatusHtml(text, isHtml = false) {
  for (const id of ["channel-status-box", "channel-status-box-mysubs"]) {
    const box = document.getElementById(id);
    if (!box) continue;
    if (isHtml) box.innerHTML = text;
    else box.textContent = text;
  }
}
async function loadChannelStatus() {
  const s = await api("/api/channel/status");
  if (!s.ok) {
    _setChannelStatusHtml("频道源不可用: " + (s.error || "unknown"), false);
    return s;
  }
  const st = s.stats || {};
  _setChannelStatusHtml(
    `频道源 <b>${s.enabled ? "启用" : "关闭"}</b> · 频道 ${esc((s.channels||[]).join(", ") || "-")} · 缓存 ${st.count ?? st.total ?? "-"} 条 · TTL ${s.cache_ttl_seconds || "-"}s`,
    true
  );
  return s;
}

async function openSubSearch(id) {
  toast("正在搜链…");
  const data = await api("/api/subscriptions/" + encodeURIComponent(id) + "/search");
  const modal = document.getElementById("sub-search-modal");
  const body = document.getElementById("sub-search-body");
  const sub = SUBS_CACHE.find(x => x.id === id) || {};
  document.getElementById("sub-search-title").textContent = `搜链 · ${sub.name || id}`;
  document.getElementById("sub-search-sub").textContent = `关键词: ${(data.queries || []).join(" / ") || "-"} · 命中 ${data.count || 0}`;
  const items = data.items || [];
  if (!items.length) {
    body.innerHTML = `<div class="empty">未找到结果。可先点「索引频道」刷新缓存，或检查关键词/频道配置。</div>`;
  } else {
    body.innerHTML = items.map((it, idx) => `
      <div class="preview-item" style="flex-direction:column;align-items:stretch;gap:8px">
        <div style="display:flex;justify-content:space-between;gap:10px;align-items:flex-start">
          <div>
            <b>${esc(it.title || it.share_url || ("结果"+(idx+1)))}</b>
            <div class="help">${esc(it.channel || "")} ${esc(it.publish_date || "")} · ${esc(it.matched_query || "")}</div>
            <div class="help" style="word-break:break-all">${esc(it.share_url || "")}</div>
            <div class="help">${esc((it.content || "").slice(0,160))}</div>
          </div>
          <button class="btn primary" type="button" data-apply-share="${esc(id)}" data-share="${encodeURIComponent(it.share_url || "")}">应用</button>
        </div>
      </div>`).join("");
    body.querySelectorAll("[data-apply-share]").forEach(btn => {
      btn.onclick = async () => {
        const sid = btn.getAttribute("data-apply-share");
        const share = decodeURIComponent(btn.getAttribute("data-share") || "");
        await api("/api/subscriptions/" + encodeURIComponent(sid) + "/apply", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ share_url: share, also_qas_task: true }),
        });
        toast("已写入订阅并同步 QAS 任务", "ok");
        document.getElementById("sub-search-modal").hidden = true;
        await loadSubscriptions();
      };
    });
  }
  modal.hidden = false;
}



async function loadAccounts() {
  const data = await api("/api/accounts");
  const box = document.getElementById("accounts-list");
  const cnt = document.getElementById("accounts-count");
  if (cnt) cnt.textContent = `${data.count || 0} 个账号`;
  if (!box) return data;
  const accs = data.accounts || [];
  if (!accs.length) {
    box.innerHTML = emptyBox("暂无账号 Cookie", "在下方粘贴完整 Cookie 后保存");
    return data;
  }
  box.innerHTML = accs.map(a => `
    <div class="preview-item" style="align-items:center">
      <div>
        <b>${esc(a.name)} ${a.active ? "· 活动" : ""}</b>
        <div class="help">${esc(a.cookie_masked)} · len ${a.cookie_len} · ${esc(a.role)}</div>
      </div>
      <div class="ops">
        <button class="btn soft" type="button" data-acc-test="${a.index}">测试</button>
        <button class="btn ${a.active ? "primary" : "soft"}" type="button" data-acc-use="${a.index}" ${a.active ? "disabled" : ""}>${a.active ? "使用中" : "切换"}</button>
      </div>
    </div>`).join("");
  box.querySelectorAll("[data-acc-use]").forEach(btn => {
    btn.onclick = async () => {
      const idx = Number(btn.getAttribute("data-acc-use"));
      await api("/api/accounts/active", {
        method: "POST",
        headers: {"Content-Type":"application/json"},
        body: JSON.stringify({index: idx}),
      });
      toast(`已切换到账号 ${idx}`);
      await loadAccounts();
      await loadStatus().catch(()=>{});
    };
  });
  box.querySelectorAll("[data-acc-test]").forEach(btn => {
    btn.onclick = async () => {
      const idx = Number(btn.getAttribute("data-acc-test"));
      const r = await api("/api/accounts/test", {
        method: "POST",
        headers: {"Content-Type":"application/json"},
        body: JSON.stringify({index: idx}),
      });
      toast(r.ok || r.quark_ok ? `账号 ${idx} 可用` : `账号 ${idx} 异常`);
    };
  });
  return data;
}

async function loadEmbyFolders() {
  const box = document.getElementById("emby-folders-box");
  try {
    const data = await api("/api/emby/folders");
    if (!box) return data;
    if (!data.ok) {
      box.textContent = "拉取失败: " + (data.error || data.status || "");
      return data;
    }
    const folders = data.folders || [];
    if (!folders.length) {
      box.textContent = "无 VirtualFolders（检查 Emby 地址/密钥）";
      return data;
    }
    box.innerHTML = folders.map(f => {
      const locs = (f.locations || []).join(" | ");
      return `<div class="row"><span>${esc(f.name)} <small style="color:var(--muted)">${esc(f.collection_type||"")}</small></span>
        <span class="ops">
          <button class="btn soft" type="button" data-emby-item="${esc(f.item_id)}">刷此库</button>
        </span></div>
        <div class="help" style="padding:0 8px 8px">${esc(locs || f.item_id || "")}</div>`;
    }).join("");
    box.querySelectorAll("[data-emby-item]").forEach(btn => {
      btn.onclick = async () => {
        const id = btn.getAttribute("data-emby-item");
        const r = await api("/api/emby/refresh", {
          method: "POST",
          headers: {"Content-Type":"application/json"},
          body: JSON.stringify({item_id: id}),
        });
        toast(r.ok ? `已触发库刷新 ${id}` : (r.error || "失败"));
      };
    });
    return data;
  } catch (e) {
    if (box) box.textContent = e.message;
    throw e;
  }
}

async function refreshEmbyNow() {
  const path = (document.getElementById("set-emby-path")?.value || "").trim();
  const r = await api("/api/emby/refresh", {
    method: "POST",
    headers: {"Content-Type":"application/json"},
    body: JSON.stringify({path}),
  });
  const mode = r.result?.mode || "";
  toast(r.ok ? `Emby 刷库已触发 (${mode})` : (r.error || "失败"));
  return r;
}



/* ===== TMDB Discover poster wall ===== */
const DISCOVER = {
  tab: "hot_movie",
  page: 1,
  totalPages: 1,
  mode: "discover", // discover | search
  query: "",
  items: [],
};
const TAB_LABELS = {
  hot_movie: "热门电影",
  hot_tv: "热门剧集",
  trend_movie: "趋势电影",
  trend_tv: "趋势剧集",
};

function renderDiscoverGrid(items) {
  const grid = document.getElementById("discover-grid");
  if (!grid) return;
  if (!items || !items.length) {
    grid.innerHTML = emptyBox("暂无结果", "换个分类，或用上方搜索框查询");
    return;
  }
  grid.innerHTML = items.map((it, idx) => {
    const rating = Number(it.vote_average || 0).toFixed(1);
    const poster = it.poster_url
      ? `<img src="${esc(it.poster_url)}" alt="${esc(it.title || "")}" loading="lazy" />`
      : `<div class="ph">${esc(it.title || "无海报")}</div>`;
    const subBadge = it.subscribed ? `<span class="sub-badge">已订阅</span>` : "";
    const year = it.year || "";
    const typeLabel = it.content_type === "movie" ? "电影" : "剧集";
    return `
    <article class="poster-card ${it.subscribed ? "subscribed" : ""}" data-idx="${idx}">
      <div class="poster">
        ${subBadge}
        <span class="rating">★ ${rating}</span>
        ${poster}
      </div>
      <div class="title">${esc(it.title || "-")}</div>
      <div class="meta">${esc(typeLabel)}${year ? " · " + esc(year) : ""}</div>
      <div class="actions">
        <button class="btn sm ${it.subscribed ? "soft" : "primary"}" type="button" data-sub-one="${idx}">
          ${it.subscribed ? "已订阅" : "订阅"}
        </button>
        <button class="btn sm soft" type="button" data-sub-fill="${idx}">详情</button>
      </div>
    </article>`;
  }).join("");

  grid.querySelectorAll("[data-sub-one]").forEach(btn => {
    btn.onclick = (e) => {
      e.stopPropagation();
      const it = DISCOVER.items[Number(btn.getAttribute("data-sub-one"))];
      if (it) subscribeFromDiscover(it).catch(err => toast(err.message, "err"));
    };
  });
  grid.querySelectorAll("[data-sub-fill]").forEach(btn => {
    btn.onclick = (e) => {
      e.stopPropagation();
      const it = DISCOVER.items[Number(btn.getAttribute("data-sub-fill"))];
      if (it) openSubFromDiscover(it);
    };
  });

  // side stack
  const stack = document.getElementById("discover-stack");
  if (stack) {
    const tops = items.filter(x => x.poster_url).slice(0, 3);
    stack.innerHTML = tops.map(x => `<img src="${esc(x.poster_url)}" alt="" loading="lazy" />`).join("");
  }
  const cnt = document.getElementById("discover-count");
  if (cnt) cnt.textContent = String(items.length);
}

async function loadDiscover() {
  const titleEl = document.getElementById("discover-title");
  const metaEl = document.getElementById("discover-meta");
  const pageEl = document.getElementById("discover-page-label");
  const grid = document.getElementById("discover-grid");
  if (!grid) return;

  try {
    let data;
    if (DISCOVER.mode === "search" && DISCOVER.query) {
      if (titleEl) titleEl.textContent = `搜索：${DISCOVER.query}`;
      data = await api(`/api/tmdb/search?q=${encodeURIComponent(DISCOVER.query)}&type=multi&page=${DISCOVER.page}`);
    } else {
      DISCOVER.mode = "discover";
      if (titleEl) titleEl.textContent = TAB_LABELS[DISCOVER.tab] || "发现";
      data = await api(`/api/tmdb/discover?tab=${encodeURIComponent(DISCOVER.tab)}&page=${DISCOVER.page}`);
    }
    if (!data.ok) {
      DISCOVER.items = [];
      if (grid) {
        if (data.configured === false) {
          grid.innerHTML = `<div class="empty"><strong>尚未配置 TMDB API Key</strong><div class="hint">去「设置 → TMDB」填入密钥后即可显示海报墙</div><div style="margin-top:10px"><button class="btn primary" type="button" id="btn-goto-tmdb">去配置 TMDB</button></div></div>`;
          const b = document.getElementById("btn-goto-tmdb");
          if (b) b.onclick = () => { setPage("settings"); setSettingsSec("tmdb"); };
        } else {
          grid.innerHTML = emptyBox("加载失败", data.error || "TMDB 请求失败");
        }
      }
      if (metaEl) metaEl.textContent = data.error || "加载失败";
      if (data.configured !== false) toast(data.error || "TMDB 加载失败", "err");
      return data;
    }
    DISCOVER.items = data.items || [];
    DISCOVER.totalPages = data.total_pages || 1;
    DISCOVER.page = data.page || DISCOVER.page;
    renderDiscoverGrid(DISCOVER.items);
    if (metaEl) {
      metaEl.textContent = `TMDB · 当前显示 ${DISCOVER.items.length} / ${data.total_results || DISCOVER.items.length} 条`;
    }
    if (pageEl) pageEl.textContent = `${DISCOVER.page} / ${DISCOVER.totalPages}`;
    // sync tab UI
    document.querySelectorAll("#discover-tabs .chip-btn").forEach(b => {
      b.classList.toggle("on", b.getAttribute("data-tab") === DISCOVER.tab && DISCOVER.mode === "discover");
    });
    return data;
  } catch (e) {
    if (metaEl) metaEl.textContent = e.message;
    grid.innerHTML = emptyBox("加载失败", e.message);
    throw e;
  }
}

function openSubFromDiscover(it) {
  openSubModal("");
  $("#sub-name").value = it.title || "";
  $("#sub-type").value = it.content_type === "movie" ? "movie" : "tv";
  $("#sub-tmdb").value = it.tmdb_id || it.id || "";
  $("#sub-keywords").value = it.title || "";
  const year = it.year || "";
  const name = year ? `${it.title} (${year})` : (it.title || "");
  $("#sub-savepath").value = it.content_type === "movie" ? `电影/${name}` : `电视剧/${name}`;
  $("#sub-strm").value = it.content_type === "movie" ? "movies" : "tv";
  $("#sub-modal-title").textContent = "从海报订阅";
  // stash poster on form via dataset
  const form = document.getElementById("sub-form");
  if (form) form.dataset.posterUrl = it.poster_url || "";
}

function _toastChannelSearch(r, title) {
  const cs = r && r.channel_search;
  if (!cs) {
    toast(title || "完成", "ok");
    return;
  }
  if (cs.applied && cs.share_url) {
    toast(`${title || "已订阅"} · 已搜到分享链`, "ok");
    return;
  }
  if (cs.skipped) {
    toast(`${title || "已订阅"} · ${cs.reason === "already has share_url" ? "已有分享链" : "频道搜链跳过"}`, "ok");
    return;
  }
  if (cs.count > 0 && !cs.applied) {
    toast(`${title || "已订阅"} · 找到 ${cs.count} 条，未自动应用`, "warn");
    return;
  }
  if (cs.error) {
    toast(`${title || "已订阅"} · 搜链失败：${cs.error}`, "warn");
    return;
  }
  toast(`${title || "已订阅"} · 频道暂无匹配`, "warn");
}

async function subscribeFromDiscover(it) {
  toast(it.subscribed ? "正在搜链…" : "正在订阅并搜链…");
  const r = await api("/api/tmdb/subscribe", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      title: it.title,
      tmdb_id: it.tmdb_id || it.id,
      content_type: it.content_type || it.media_type,
      year: it.year,
      poster_url: it.poster_url || "",
      keywords: it.title,
    }),
  });
  const name = it.title || "订阅";
  if (r.already) _toastChannelSearch(r, `已订阅：${name}`);
  else _toastChannelSearch(r, `已订阅：${name}`);
  await loadSubscriptions();
  await loadDiscover();
  return r;
}


/* ===== MTProto watch ===== */
async function loadMtprotoStatus() {
  const s = await api("/api/mtproto/status");
  const box = document.getElementById("mp-status-rows");
  if (box) {
    const rows = [
      ["运行", s.running ? "监控中" : "未运行"],
      ["登录", s.authorized ? (`已登录 ${s.user || ""}`) : (s.login_stage || "未登录")],
      ["Telethon", s.telethon_ok ? "可用" : "未安装"],
      ["启用配置", s.enabled ? "是" : "否"],
      ["监听频道", (s.joined && s.joined.length) ? s.joined.join(", ") : ((s.channels || []).join(", ") || "-")],
      ["命中次数", String(s.hits_total ?? 0)],
      ["最近事件", s.last_event || "-"],
      ["错误", s.last_error || "-"],
    ];
    box.innerHTML = rows.map(([k,v]) => `<div class="row"><span>${esc(k)}</span><b>${esc(v)}</b></div>`).join("");
  }
  const hits = document.getElementById("mp-hits");
  if (hits) {
    const list = s.recent_hits || [];
    if (!list.length) hits.innerHTML = emptyBox("暂无命中", "有匹配订阅的新夸克链会出现在这里");
    else hits.innerHTML = list.map(h => `
      <div class="preview-item">
        <div>
          <b>${esc(h.sub_name || h.sub_id || "-")}</b>
          <span>${esc(h.time || "")} · ${esc(h.channel || "")} · ${esc(h.matched || "")}</span>
          <span>${esc(h.share_url || "")}</span>
        </div>
      </div>`).join("");
  }
  // form hints
  const hh = document.getElementById("mp-hash-hint");
  if (hh) hh.textContent = s.api_hash_set ? (s.api_hash_masked || "已配置") : "未配置";
  const en = document.getElementById("set-mp-enabled");
  if (en && document.activeElement !== en) en.checked = !!s.enabled;
  const aa = document.getElementById("set-mp-auto-apply");
  if (aa && document.activeElement !== aa) aa.checked = s.auto_apply !== false;
  const qas = document.getElementById("set-mp-qas");
  if (qas && document.activeElement !== qas) qas.checked = s.also_qas_task !== false;
  const up = document.getElementById("set-mp-update");
  if (up && document.activeElement !== up) up.checked = !!s.update_existing_share;
  const apiId = document.getElementById("set-mp-api-id");
  if (apiId && !apiId.value) apiId.value = s.api_id || "";
  const phone = document.getElementById("set-mp-phone");
  if (phone && !phone.value) phone.value = s.phone || "";
  const sess = document.getElementById("set-mp-session");
  if (sess && !sess.value) sess.value = s.session_path || "";
  const ch = document.getElementById("set-mp-channels");
  if (ch && !ch.value) ch.value = (s.channels || []).join("\n");
  return s;
}

function collectMtprotoPatch() {
  const patch = {
    enabled: !!document.getElementById("set-mp-enabled")?.checked,
    auto_apply: !!document.getElementById("set-mp-auto-apply")?.checked,
    also_qas_task: !!document.getElementById("set-mp-qas")?.checked,
    update_existing_share: !!document.getElementById("set-mp-update")?.checked,
    api_id: document.getElementById("set-mp-api-id")?.value.trim() || "",
    phone: document.getElementById("set-mp-phone")?.value.trim() || "",
    session_path: document.getElementById("set-mp-session")?.value.trim() || "",
    channels: document.getElementById("set-mp-channels")?.value || "",
  };
  const hash = document.getElementById("set-mp-api-hash")?.value.trim() || "";
  if (hash) patch.api_hash = hash;
  return patch;
}

async function saveMtprotoConfig() {
  const patch = collectMtprotoPatch();
  toast("正在保存 MTProto…");
  const r = await api("/api/mtproto/config", {
    method: "POST",
    headers: {"Content-Type":"application/json"},
    body: JSON.stringify(patch),
  });
  toast("MTProto 配置已保存", "ok");
  document.getElementById("set-mp-api-hash").value = "";
  await loadMtprotoStatus();
  return r;
}

async function mpAction(kind) {
  if (kind === "send-code") {
    const phone = document.getElementById("set-mp-phone")?.value.trim() || "";
    // save first
    await saveMtprotoConfig().catch(() => {});
    const r = await api("/api/mtproto/send-code", {
      method: "POST", headers: {"Content-Type":"application/json"},
      body: JSON.stringify({phone}),
    });
    if (r.ok) toast(r.already_authorized ? `已登录 ${r.user||""}` : "验证码已发送", "ok");
    else toast(r.error || "发送失败", "err");
    await loadMtprotoStatus();
    return r;
  }
  if (kind === "sign-in") {
    const code = document.getElementById("set-mp-code")?.value.trim() || "";
    const password = document.getElementById("set-mp-password")?.value || "";
    const r = await api("/api/mtproto/sign-in", {
      method: "POST", headers: {"Content-Type":"application/json"},
      body: JSON.stringify({code, password}),
    });
    if (r.ok) toast(`登录成功 ${r.user||""}`, "ok");
    else if (r.need_password) toast("需要二步验证密码", "warn");
    else toast(r.error || "登录失败", "err");
    await loadMtprotoStatus();
    return r;
  }
  if (kind === "logout") {
    const r = await api("/api/mtproto/logout", { method: "POST" });
    toast("已退出登录", "ok");
    await loadMtprotoStatus();
    return r;
  }
  const r = await api(`/api/mtproto/${kind}`, { method: "POST" });
  toast(r.message || (r.ok ? "完成" : (r.error || "完成")), r.ok ? "ok" : "err");
  await loadMtprotoStatus();
  return r;
}

function bind() {

  const btnMpRefresh = document.getElementById("btn-mp-refresh");
  if (btnMpRefresh) btnMpRefresh.onclick = () => loadMtprotoStatus().catch(e => toast(e.message, "err"));
  const btnMpStart = document.getElementById("btn-mp-start");
  if (btnMpStart) btnMpStart.onclick = () => mpAction("start").catch(e => toast(e.message, "err"));
  const btnMpStop = document.getElementById("btn-mp-stop");
  if (btnMpStop) btnMpStop.onclick = () => mpAction("stop").catch(e => toast(e.message, "err"));
  const btnMpRestart = document.getElementById("btn-mp-restart");
  if (btnMpRestart) btnMpRestart.onclick = () => mpAction("restart").catch(e => toast(e.message, "err"));
  const btnMpSend = document.getElementById("btn-mp-send-code");
  if (btnMpSend) btnMpSend.onclick = () => mpAction("send-code").catch(e => toast(e.message, "err"));
  const btnMpSign = document.getElementById("btn-mp-sign-in");
  if (btnMpSign) btnMpSign.onclick = () => mpAction("sign-in").catch(e => toast(e.message, "err"));
  const btnMpLogout = document.getElementById("btn-mp-logout");
  if (btnMpLogout) btnMpLogout.onclick = () => mpAction("logout").catch(e => toast(e.message, "err"));
  const btnMpSave = document.getElementById("btn-mp-save");
  if (btnMpSave) btnMpSave.onclick = () => saveMtprotoConfig().catch(e => toast(e.message, "err"));

  initTheme();

  const btnAccReload = document.getElementById("btn-acc-reload");
  if (btnAccReload) btnAccReload.onclick = () => loadAccounts().then(() => toast("账号已刷新")).catch(e => toast(e.message));
  const btnEmbyFolders = document.getElementById("btn-emby-folders");
  if (btnEmbyFolders) btnEmbyFolders.onclick = () => loadEmbyFolders().then(() => toast("媒体库已拉取")).catch(e => toast(e.message));
  const btnEmbyRefresh = document.getElementById("btn-emby-refresh");
  if (btnEmbyRefresh) btnEmbyRefresh.onclick = () => refreshEmbyNow().catch(e => toast(e.message));
  // 兼容旧 data-settings-sec（已不再使用）
  document.querySelectorAll("[data-settings-sec]").forEach(btn => {
    btn.addEventListener("click", (e) => {
      e.stopPropagation();
      const sec = btn.getAttribute("data-settings-sec");
      if (sec === "subs") setPage("subs");
      else { setPage("settings"); if (sec) setSettingsSec(sec); }
    });
  });


  const btnChStatus = document.getElementById("btn-channel-status");
  if (btnChStatus) btnChStatus.onclick = () => loadChannelStatus().then(() => toast("已刷新频道状态")).catch(e => toast(e.message));
  const btnChIndex = document.getElementById("btn-channel-index");
  if (btnChIndex) btnChIndex.onclick = async () => {
    toast("正在索引频道（可能较慢）…");
    try {
      const r = await api("/api/channel/index", { method: "POST", headers: {"Content-Type":"application/json"}, body: "{}" });
      toast(r.ok ? "索引完成" : (r.error || "索引失败"));
      await loadChannelStatus();
    } catch (e) { toast(e.message); }
  };
  
  const btnChStatus2 = document.getElementById("btn-channel-status-2");
  if (btnChStatus2) btnChStatus2.onclick = () => loadChannelStatus().then(() => toast("已刷新频道状态", "ok")).catch(e => toast(e.message, "err"));
  const btnSearchAll2 = document.getElementById("btn-subs-search-all-2");
  if (btnSearchAll2) btnSearchAll2.onclick = () => document.getElementById("btn-subs-search-all")?.click();

  const btnSearchAll = document.getElementById("btn-subs-search-all");
  if (btnSearchAll) btnSearchAll.onclick = async () => {
    toast("批量搜链中…");
    try {
      const r = await api("/api/subscriptions/refresh-channels", {
        method: "POST",
        headers: {"Content-Type":"application/json"},
        body: JSON.stringify({ only_missing_share: true, apply_best: true }),
      });
      toast(r.skipped ? "频道源未启用/跳过" : `完成：应用 ${r.applied || 0}/${r.total || 0}`);
      await loadSubscriptions();
      await loadChannelStatus();
    } catch (e) { toast(e.message); }
  };
  const subSearchClose = document.getElementById("sub-search-close");
  if (subSearchClose) subSearchClose.onclick = () => { const m = document.getElementById("sub-search-modal"); if (m) m.hidden = true; };
  const subSearchModal = document.getElementById("sub-search-modal");
  if (subSearchModal) subSearchModal.addEventListener("click", (e) => { if (e.target === subSearchModal) subSearchModal.hidden = true; });


  const btnSubsAdd = document.getElementById("btn-subs-add");
  if (btnSubsAdd) btnSubsAdd.onclick = () => openSubModal("");
  document.querySelectorAll('[data-action="sub-add"]').forEach(b => {
    b.onclick = () => openSubModal("");
  });
  const btnSubsReload = document.getElementById("btn-subs-reload");
  if (btnSubsReload) btnSubsReload.onclick = () => loadSubscriptions().then(() => toast("已刷新")).catch(e => toast(e.message));
  const subForm = document.getElementById("sub-form");
  if (subForm) subForm.addEventListener("submit", (e) => saveSubForm(e).catch(err => toast(err.message)));
  const subClose = document.getElementById("sub-modal-close");
  if (subClose) subClose.onclick = closeSubModal;
  const subCancel = document.getElementById("sub-cancel");
  if (subCancel) subCancel.onclick = closeSubModal;
  const subModal = document.getElementById("sub-modal");
  if (subModal) subModal.addEventListener("click", (e) => { if (e.target === subModal) closeSubModal(); });


  const tgRefresh = document.getElementById("btn-tg-refresh");
  if (tgRefresh) tgRefresh.onclick = () => loadTgStatus().catch(e => toast(e.message));
  const tgTest = document.getElementById("btn-tg-test");
  if (tgTest) tgTest.onclick = () => tgAction("test").catch(e => toast(e.message));
  const tgStart = document.getElementById("btn-tg-start");
  if (tgStart) tgStart.onclick = () => tgAction("start").catch(e => toast(e.message));
  const tgStop = document.getElementById("btn-tg-stop");
  if (tgStop) tgStop.onclick = () => tgAction("stop").catch(e => toast(e.message));
  const tgRestart = document.getElementById("btn-tg-restart");
  if (tgRestart) tgRestart.onclick = () => tgAction("restart").catch(e => toast(e.message));

  $$(".nav-item").forEach(b => b.addEventListener("click", () => setPage(b.dataset.page)));
  $$("[data-goto]").forEach(b => b.addEventListener("click", () => setPage(b.dataset.goto)));
  $$("#settings-nav button").forEach(b => b.addEventListener("click", () => setSettingsSec(b.dataset.sec)));
  const _bt = $("#btn-theme"); if (_bt) _bt.onclick = toggleTheme;
  $("#btn-refresh").onclick = () => refreshAll();
  $("#btn-run-once").onclick = () => runOnce().catch(e => toast(e.message));
  $("#btn-run-tasks").onclick = () => runOnce().catch(e => toast(e.message));
  $("#btn-reload-tasks").onclick = () => loadTasks().catch(e => toast(e.message));
  const btnAdd = $("#btn-add-task");
  if (btnAdd) btnAdd.onclick = () => openTaskModal("");
  const taskForm = $("#task-form");
  if (taskForm) taskForm.addEventListener("submit", (e) => saveTaskForm(e).catch(err => toast(err.message)));
  const taskClose = $("#task-modal-close");
  if (taskClose) taskClose.onclick = closeTaskModal;
  const taskCancel = $("#task-cancel");
  if (taskCancel) taskCancel.onclick = closeTaskModal;
  const taskModal = $("#task-modal");
  if (taskModal) taskModal.addEventListener("click", (e) => { if (e.target === taskModal) closeTaskModal(); });
  $("#btn-test-play").onclick = () => testPlay();
  $("#btn-clear-logs").onclick = async () => {
    await api("/api/logs/clear", { method: "POST" });
    await loadLogs();
    toast("日志已清空", "ok");
  };
  $("#btn-settings-save").onclick = () => saveSettings().catch(e => toast(e.message));
  $("#btn-settings-reload").onclick = () => loadSettings().then(() => toast("已重新加载")).catch(e => toast(e.message));
  $("#settings-form").addEventListener("submit", e => {
    e.preventDefault();
    saveSettings().catch(err => toast(err.message));
  });
  $("#btn-menu").onclick = () => {
    $(".sidebar").classList.add("open");
    $("#scrim").hidden = false;
  };
  $("#scrim").onclick = () => {
    $(".sidebar").classList.remove("open");
    $("#scrim").hidden = true;
  };

  // TMDB discover wall
  document.querySelectorAll("#discover-tabs .chip-btn").forEach(btn => {
    btn.addEventListener("click", () => {
      DISCOVER.tab = btn.getAttribute("data-tab") || "hot_movie";
      DISCOVER.mode = "discover";
      DISCOVER.page = 1;
      DISCOVER.query = "";
      const q = document.getElementById("discover-q");
      if (q) q.value = "";
      loadDiscover().catch(e => toast(e.message, "err"));
    });
  });
  const btnDiscSearch = document.getElementById("btn-discover-search");
  if (btnDiscSearch) btnDiscSearch.onclick = () => {
    const q = (document.getElementById("discover-q")?.value || "").trim();
    if (!q) return toast("请输入搜索关键词", "warn");
    DISCOVER.mode = "search";
    DISCOVER.query = q;
    DISCOVER.page = 1;
    loadDiscover().catch(e => toast(e.message, "err"));
  };
  const btnDiscReload = document.getElementById("btn-discover-reload");
  if (btnDiscReload) btnDiscReload.onclick = () => loadDiscover().then(() => toast("已刷新", "ok")).catch(e => toast(e.message, "err"));
  const btnPrev = document.getElementById("btn-discover-prev");
  if (btnPrev) btnPrev.onclick = () => {
    if (DISCOVER.page <= 1) return;
    DISCOVER.page -= 1;
    loadDiscover().catch(e => toast(e.message, "err"));
  };
  const btnNext = document.getElementById("btn-discover-next");
  if (btnNext) btnNext.onclick = () => {
    if (DISCOVER.page >= (DISCOVER.totalPages || 1)) return;
    DISCOVER.page += 1;
    loadDiscover().catch(e => toast(e.message, "err"));
  };
  const qInput = document.getElementById("discover-q");
  if (qInput) {
    let t = null;
    qInput.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        document.getElementById("btn-discover-search")?.click();
      }
    });
    qInput.addEventListener("input", () => {
      clearTimeout(t);
      t = setTimeout(() => {
        const q = qInput.value.trim();
        if (!q) {
          DISCOVER.mode = "discover";
          DISCOVER.query = "";
          DISCOVER.page = 1;
          loadDiscover().catch(() => {});
          return;
        }
        if (q.length < 2) return;
        DISCOVER.mode = "search";
        DISCOVER.query = q;
        DISCOVER.page = 1;
        loadDiscover().catch(() => {});
      }, 450);
    });
  }

const hash = (location.hash || "#dashboard").slice(1);
  setPage(TITLES[hash] ? hash : "dashboard");
  setSettingsSec("account");
}

bind();
refreshAll();
setInterval(() => loadStatus().catch(() => {}), 15000);
