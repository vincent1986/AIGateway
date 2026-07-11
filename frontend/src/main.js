import "./style.css";
import "./app.css";

import {
  DiscoverToolConfigs,
  PickToolConfig,
  ClearToolConfigPath,
  SetToolModel,
  ApplyToolModel,
  ApplyDeepSeekClaudeCode,
  RevealConfigPath,
  ReadConfigText,
  GetSystemInfo,
  BackupDefaultConfig,
  RestoreDefaultConfig,
  ClearDefaultBackup,
  ListProviders,
  SaveProviders,
  FetchProviderModels,
  TestProviderConnection,
  GetProxyStatus,
  GetProxyConfig,
  SaveProxyConfig,
  StartProxy,
  StopProxy,
  EnsureProxyRouting,
  GetUsageStats,
  ClearUsageStats,
} from "../wailsjs/go/main/App";

/** @typedef {{ id: string, name: string, baseUrl: string, apiKey: string, color: string, models: Model[] }} Provider */
/** @typedef {{ id: string, name: string, enabled: boolean, isDefault: boolean, ownedBy?: string }} Model */
/** @typedef {{ kind: string, name: string, path: string, found: boolean, exists: boolean, model: string, modelProvider: string, searchPaths: string[], candidates: {id:string,name:string,provider:string}[], source: string, message: string, hasDefaultBackup?: boolean, defaultBackupAt?: string }} ToolConfigStatus */

const COLORS = ["#3d8bfd", "#7c5cff", "#3fb950", "#d29922", "#f85149", "#39c5cf", "#e85d9a"];
const PRESETS = [
  { name: "Ollama", baseUrl: "http://127.0.0.1:11434/v1", color: "#c4c4c4", useProxy: false, apiKey: "ollama" },
  { name: "OpenAI", baseUrl: "https://api.openai.com/v1", color: "#3d8bfd", useProxy: true },
  { name: "Anthropic", baseUrl: "https://api.anthropic.com/v1", color: "#d29922", useProxy: true },
  { name: "DeepSeek", baseUrl: "https://api.deepseek.com/v1", color: "#3fb950", useProxy: true },
  { name: "通义千问", baseUrl: "https://dashscope.aliyuncs.com/compatible-mode/v1", color: "#7c5cff", useProxy: true },
  { name: "Moonshot", baseUrl: "https://api.moonshot.cn/v1", color: "#39c5cf", useProxy: true },
  { name: "清华智谱", baseUrl: "https://open.bigmodel.cn/api/paas/v4", color: "#3859ff", useProxy: true },
  { name: "MiniMax", baseUrl: "https://api.minimax.chat/v1", color: "#e85d9a", useProxy: true },
  { name: "自定义", baseUrl: "https://", color: "#8b9cb3", useProxy: true },
];

const STORAGE_KEY = "codex.providers.v1";
const PAGE_KEY = "codex.ui.page";

/** @type {Provider[]} */
let providers = [];
/** @type {string|null} */
let selectedId = null;
let showKey = false;
let fetching = false;
let testing = false;
/** @type {null | { ok: boolean, message: string, endpoint?: string, statusCode?: number, latencyMs?: number, modelCount?: number, sample?: string[], error?: string }} */
let testResult = null;
let modelQuery = "";
let booting = true;
/** @type {'providers'|'configs'|'proxy'|'usage'} */
let page = ["configs", "proxy", "usage"].includes(localStorage.getItem(PAGE_KEY) || "")
  ? localStorage.getItem(PAGE_KEY)
  : "providers";

/** @type {null | { total?: any, byDay?: any[], byModel?: any[], byProvider?: any[], recent?: any[] }} */
let usageStats = null;
let usageBusy = false;

/** @type {null | { running?: boolean, baseUrl?: string, host?: string, port?: number, autoStart?: boolean, listenKey?: string, lastError?: string, logs?: string[] }} */
let proxyStatus = null;
let proxyBusy = false;
let proxyForm = { host: "127.0.0.1", port: 18080, autoStart: false, listenKey: "" };

/** @type {Record<string, ToolConfigStatus>} */
let toolConfigs = {};
/** @type {Record<string, string>} */
let pendingModel = {};
/** @type {Record<string, string>} */
let pendingProvider = {};
/** @type {Record<string, string>} */
let configPreview = {};
let configsLoading = false;
let configsBusy = "";

/** @type {{ os?: string, platformName?: string, revealLabel?: string, homeDir?: string }} */
let systemInfo = {
  os: "darwin",
  platformName: "macOS",
  revealLabel: "在访达中显示",
  homeDir: "",
};

function uid() {
  return crypto.randomUUID?.() ?? `id_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
}

function hasBackend() {
  return typeof window.go?.main?.App?.ListProviders === "function";
}

function toast(msg, type = "ok") {
  const host = document.getElementById("toast-host");
  if (!host) return;
  const el = document.createElement("div");
  el.className = `toast ${type}`;
  el.textContent = msg;
  host.appendChild(el);
  setTimeout(() => el.remove(), 3000);
}

function initials(name) {
  return (name || "?").trim().slice(0, 2).toUpperCase();
}

function totalModels() {
  return providers.reduce((n, p) => n + (p.models?.length || 0), 0);
}

function enabledModels() {
  return providers.reduce((n, p) => n + (p.models?.filter((m) => m.enabled).length || 0), 0);
}

function selected() {
  return providers.find((p) => p.id === selectedId) ?? null;
}

function isLocalProviderHint(name, baseUrl) {
  const n = `${name || ""} ${baseUrl || ""}`.toLowerCase();
  return n.includes("ollama") || n.includes("11434") || n.includes("127.0.0.1") || n.includes("localhost");
}

function normalizeProvider(p) {
  const name = p.name || "";
  const baseUrl = (p.baseUrl || p.BaseURL || "").replace(/\/$/, "");
  // useProxy: explicit bool, or auto (local → false, cloud → true)
  let useProxy;
  if (typeof p.useProxy === "boolean") useProxy = p.useProxy;
  else if (typeof p.UseProxy === "boolean") useProxy = p.UseProxy;
  else useProxy = !isLocalProviderHint(name, baseUrl);
  return {
    id: p.id || uid(),
    name,
    baseUrl,
    apiKey: p.apiKey || p.APIKey || "",
    color: p.color || COLORS[0],
    useProxy,
    models: (p.models || []).map((m) => ({
      id: m.id,
      name: m.name || m.id,
      enabled: m.enabled !== false,
      isDefault: !!m.isDefault,
      ownedBy: m.ownedBy || m.owned_by || "",
    })),
  };
}

async function loadProviders() {
  if (hasBackend()) {
    try {
      const list = await ListProviders();
      providers = (list || []).map(normalizeProvider);
      // migrate localStorage once if disk empty
      if (!providers.length) {
        const legacy = loadLocalLegacy();
        if (legacy.length) {
          providers = legacy;
          await persistProviders();
          localStorage.removeItem(STORAGE_KEY);
          toast(`已迁移 ${legacy.length} 个本地厂家到应用存储`);
        }
      }
    } catch (e) {
      console.error(e);
      providers = loadLocalLegacy();
      toast("加载厂家失败，已回退本地缓存", "err");
    }
  } else {
    providers = loadLocalLegacy();
  }
  if (!selectedId || !providers.some((p) => p.id === selectedId)) {
    selectedId = providers[0]?.id ?? null;
  }
}

function loadLocalLegacy() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) return JSON.parse(raw).map(normalizeProvider);
  } catch (_) {}
  return [];
}

async function persistProviders() {
  if (hasBackend()) {
    await SaveProviders(providers);
  } else {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(providers));
  }
}

async function loadSystemInfo() {
  if (!hasBackend() || typeof window.go?.main?.App?.GetSystemInfo !== "function") {
    const ua = navigator.userAgent || "";
    if (/Windows/i.test(ua)) {
      systemInfo = { os: "windows", platformName: "Windows", revealLabel: "在资源管理器中显示" };
    } else if (/Mac/i.test(ua)) {
      systemInfo = { os: "darwin", platformName: "macOS", revealLabel: "在访达中显示" };
    } else {
      systemInfo = { os: "linux", platformName: "Linux", revealLabel: "在文件管理器中显示" };
    }
    return;
  }
  try {
    systemInfo = (await GetSystemInfo()) || systemInfo;
  } catch (_) {}
}

/** enabled models from all providers + config candidates */
function modelChoicesFor(kind) {
  /** @type {{id:string,name:string,provider:string,group:string}[]} */
  const list = [];
  const seen = new Set();
  const st = toolConfigs[kind];
  for (const c of st?.candidates || []) {
    if (!c?.id || seen.has(c.id)) continue;
    seen.add(c.id);
    list.push({ id: c.id, name: c.name || c.id, provider: c.provider || "", group: "配置文件内" });
  }
  for (const p of providers) {
    for (const m of p.models || []) {
      if (!m.enabled || !m.id || seen.has(m.id)) continue;
      seen.add(m.id);
      list.push({ id: m.id, name: m.name || m.id, provider: "", group: p.name });
    }
  }
  if (st?.model && !seen.has(st.model)) {
    list.unshift({ id: st.model, name: st.model, provider: st.modelProvider || "", group: "当前" });
  }
  return list;
}

async function fetchModelsFromApi(provider) {
  if (hasBackend()) {
    return FetchProviderModels(provider.baseUrl, provider.apiKey);
  }
  // browser preview fallback
  await new Promise((r) => setTimeout(r, 400));
  throw new Error("请在 Wails 应用中运行以真实获取模型");
}

function mergeFetchedModels(provider, fetched) {
  const prev = new Map((provider.models || []).map((m) => [m.id, m]));
  const next = (fetched || []).map((m, i) => {
    const id = m.id || m.ID;
    const old = prev.get(id);
    return {
      id,
      name: m.name || m.Name || id,
      ownedBy: m.ownedBy || m.OwnedBy || "",
      enabled: old ? old.enabled : true,
      isDefault: old ? old.isDefault : i === 0 && !provider.models?.some((x) => x.isDefault),
    };
  });
  if (!next.some((m) => m.isDefault) && next.length) next[0].isDefault = true;
  const defaults = next.filter((m) => m.isDefault);
  if (defaults.length > 1) defaults.slice(1).forEach((m) => (m.isDefault = false));
  provider.models = next;
}

// ─── Config backend ───
async function loadToolConfigs(force = false) {
  if (!hasBackend()) {
    toolConfigs = {
      codex: mockTool("codex", "Codex", "~/.codex/config.toml"),
      claude: mockTool("claude", "Claude Code", "~/.claude/settings.json"),
    };
    return;
  }
  configsLoading = true;
  render();
  try {
    const list = await DiscoverToolConfigs();
    toolConfigs = {};
    for (const item of list || []) {
      toolConfigs[item.kind] = item;
      pendingModel[item.kind] = item.model || "";
      pendingProvider[item.kind] = item.modelProvider || "";
      if (item.exists && item.path) {
        try {
          configPreview[item.kind] = await ReadConfigText(item.path);
        } catch {
          configPreview[item.kind] = "";
        }
      } else {
        configPreview[item.kind] = "";
      }
    }
  } catch (e) {
    toast(e?.message || String(e), "err");
  } finally {
    configsLoading = false;
    if (force) toast("已重新搜索配置文件");
    render();
  }
}

function mockTool(kind, name, path) {
  return {
    kind,
    name,
    path,
    found: false,
    exists: false,
    model: "",
    modelProvider: "",
    searchPaths: [path],
    candidates: [],
    source: "auto",
    message: "当前为浏览器预览，请用 wails dev 连接本机",
    hasDefaultBackup: false,
  };
}

async function applyToolStatus(item) {
  if (!item) return;
  toolConfigs[item.kind] = item;
  pendingModel[item.kind] = item.model || pendingModel[item.kind] || "";
  pendingProvider[item.kind] = item.modelProvider || "";
  if (item.exists && item.path) {
    try {
      configPreview[item.kind] = await ReadConfigText(item.path);
    } catch {
      configPreview[item.kind] = "";
    }
  }
}

// ─── Render ───
function render() {
  const app = document.getElementById("app");
  if (booting) {
    app.innerHTML = `
      <div class="main-empty" style="flex:1;min-height:0;width:100%;height:100%">
        <div>
          <div class="empty-icon"><span class="spinner" style="width:22px;height:22px;border-width:3px"></span></div>
          <h3>正在加载…</h3>
          <p>读取厂家与系统信息</p>
        </div>
      </div>`;
    return;
  }

  app.innerHTML = `
    <header class="topbar">
      <div class="topbar-left">
        <div class="brand">
          <div class="brand-mark">AI</div>
          <div>
            <div class="brand-title">AI Switch <span class="brand-sub">模型管理</span></div>
          </div>
        </div>
        <nav class="nav-tabs">
          <button class="nav-tab ${page === "providers" ? "active" : ""}" data-page="providers">厂家模型</button>
          <button class="nav-tab ${page === "configs" ? "active" : ""}" data-page="configs">配置文件</button>
          <button class="nav-tab ${page === "proxy" ? "active" : ""}" data-page="proxy">代理服务</button>
          <button class="nav-tab ${page === "usage" ? "active" : ""}" data-page="usage">Token 统计</button>
        </nav>
      </div>
      <div class="topbar-meta">
        <span class="stat-pill">${escapeHtml(systemInfo.platformName || "")}</span>
        <span class="stat-pill">厂家 <strong>${providers.length}</strong></span>
        <span class="stat-pill">模型 <strong>${totalModels()}</strong></span>
        <span class="stat-pill">已启用 <strong>${enabledModels()}</strong></span>
        ${
          proxyStatus?.running
            ? `<span class="stat-pill"><span class="status-dot ok"></span> 代理</span>`
            : ""
        }
      </div>
    </header>
    ${
      page === "providers"
        ? renderProvidersPage()
        : page === "proxy"
          ? renderProxyPage()
          : page === "usage"
            ? renderUsagePage()
            : renderConfigsPage()
    }
    <div class="toast-host" id="toast-host"></div>
    <div id="modal-root"></div>
  `;
  bindShellEvents();
  if (page === "providers") bindProviderEvents();
  else if (page === "proxy") bindProxyEvents();
  else if (page === "usage") bindUsageEvents();
  else bindConfigEvents();
}

function renderProxyPage() {
  const st = proxyStatus || {};
  const running = !!st.running;
  const base = st.baseUrl || `http://${proxyForm.host}:${proxyForm.port}/v1`;
  const logs = (st.logs || []).slice().reverse();

  return `
    <div class="full-page">
      <div class="config-page">
        <div class="config-hero">
          <div>
            <h2>OpenAI 兼容代理</h2>
            <p>本地统一入口：客户端按标准 OpenAI 协议访问，代理按模型路由到各厂家 API（自动带上对应 Key）。</p>
          </div>
          <div class="actions">
            ${
              running
                ? `<button class="btn btn-danger" id="btn-proxy-stop" ${proxyBusy ? "disabled" : ""}>停止代理</button>`
                : `<button class="btn btn-primary" id="btn-proxy-start" ${proxyBusy ? "disabled" : ""}>启动代理</button>`
            }
            <button class="btn" id="btn-proxy-refresh" ${proxyBusy ? "disabled" : ""}>刷新状态</button>
          </div>
        </div>

        <div class="config-grid">
          <section class="config-card">
            <div class="config-card-head">
              <h3>
                <span class="status-dot ${running ? "ok" : "fail"}"></span>
                服务状态
              </h3>
              <span class="tag ${running ? "ok" : "off"}">${running ? "运行中" : "已停止"}</span>
            </div>
            <div class="config-card-body">
              <div class="kv">
                <label>OpenAI Base URL（给 Codex / 客户端用）</label>
                <div class="input-with-action">
                  <input class="input mono" id="proxy-base-url" readonly value="${escapeAttr(base)}" />
                  <button class="btn btn-sm" id="btn-copy-base">复制</button>
                </div>
              </div>
              <div class="form-grid" style="margin-top:12px">
                <div class="field">
                  <label>监听地址</label>
                  <input class="input mono" id="proxy-host" value="${escapeAttr(proxyForm.host)}" ${running ? "disabled" : ""} />
                </div>
                <div class="field">
                  <label>端口</label>
                  <input class="input mono" id="proxy-port" type="number" min="1" max="65535" value="${escapeAttr(String(proxyForm.port))}" ${running ? "disabled" : ""} />
                </div>
                <div class="field full">
                  <label>接入密钥（可选，客户端 Bearer；空则不校验）</label>
                  <input class="input mono" id="proxy-listen-key" value="${escapeAttr(proxyForm.listenKey || "")}" placeholder="留空=任意 token" />
                </div>
                <div class="field full">
                  <label style="display:flex;align-items:center;gap:8px;cursor:pointer">
                    <input type="checkbox" id="proxy-autostart" ${proxyForm.autoStart ? "checked" : ""} />
                    应用启动时自动开启代理
                  </label>
                </div>
              </div>
              <div class="actions" style="margin-top:14px">
                <button class="btn btn-primary" id="btn-proxy-save" ${proxyBusy ? "disabled" : ""}>保存配置</button>
              </div>
              <div class="hint" style="margin-top:8px">
                在<strong>厂家模型</strong>里选择「走本地代理」后会<strong>自动启动代理</strong>并写入 Codex 的 base_url，无需在此单独设置。
                Windows 系统代理下，本机 127.0.0.1 自动直连，云 API 仍可走系统代理。
              </div>
              ${
                st.lastError
                  ? `<div class="test-result err" style="margin-top:12px"><div class="test-result-title">错误</div><pre class="preview-box">${escapeHtml(st.lastError)}</pre></div>`
                  : ""
              }
              <div class="hint" style="margin-top:10px">
                流程：厂家开启「走本地代理」并保存 → 代理自动就绪 → Codex 请求经上方地址转发到真实 API。
              </div>
            </div>
          </section>

          <section class="config-card">
            <div class="config-card-head">
              <h3>路由说明</h3>
            </div>
            <div class="config-card-body">
              <div class="hint">支持端点（标准 OpenAI）</div>
              <ul class="search-paths">
                <li>GET  {base}/models</li>
                <li>POST {base}/chat/completions（含 stream）</li>
                <li>POST {base}/completions</li>
                <li>POST {base}/embeddings</li>
              </ul>
              <div class="hint" style="margin-top:12px">按 model 匹配厂家</div>
              <div class="model-table-wrap" style="margin-top:8px;max-height:220px;overflow:auto">
                <table class="model-table">
                  <thead><tr><th>厂家</th><th>已启用模型</th></tr></thead>
                  <tbody>
                    ${
                      providers.length
                        ? providers
                            .map((p) => {
                              const ms = (p.models || []).filter((m) => m.enabled).map((m) => m.id);
                              return `<tr>
                                <td>${escapeHtml(p.name)}</td>
                                <td class="model-id">${ms.length ? escapeHtml(ms.join(", ")) : "（无）"}</td>
                              </tr>`;
                            })
                            .join("")
                        : `<tr><td colspan="2">暂无厂家，请先在「厂家模型」添加</td></tr>`
                    }
                  </tbody>
                </table>
              </div>
            </div>
          </section>
        </div>

        <section class="panel" style="margin-top:4px">
          <div class="panel-head">
            <div>
              <h3>运行日志</h3>
              <p class="desc">最近请求与路由记录</p>
            </div>
          </div>
          <div class="panel-body">
            <pre class="preview-box" style="max-height:220px">${
              logs.length ? escapeHtml(logs.join("\n")) : "（暂无日志，启动代理后在此查看）"
            }</pre>
          </div>
        </section>
      </div>
    </div>
  `;
}

function renderProvidersPage() {
  const p = selected();
  return `
    <div class="layout">
      <aside class="sidebar">
        <div class="sidebar-head">
          <h2>厂家</h2>
          <button class="btn btn-primary btn-sm" id="btn-add-provider">+ 添加</button>
        </div>
        <div class="provider-list">
          ${
            providers.length
              ? providers
                  .map(
                    (item) => `
            <button class="provider-item ${item.id === selectedId ? "active" : ""}" data-id="${item.id}">
              <div class="provider-avatar" style="background:${item.color}">${initials(item.name)}</div>
              <div class="provider-meta">
                <div class="provider-name">${escapeHtml(item.name || "未命名")}</div>
                <div class="provider-hint">${escapeHtml(item.baseUrl || "未配置 API")}</div>
              </div>
              <span class="provider-badge" title="${item.useProxy ? "走代理" : "直连"}">${item.useProxy ? "代" : "直"} ${item.models?.length || 0}</span>
            </button>`
                  )
                  .join("")
              : `<div class="empty-side">还没有厂家<br/>点击「添加」开始</div>`
          }
        </div>
      </aside>
      <main class="main">
        ${p ? renderDetail(p) : renderEmpty()}
      </main>
    </div>
  `;
}

function renderConfigsPage() {
  const codex = toolConfigs.codex;
  const claude = toolConfigs.claude;
  return `
    <div class="full-page">
      <div class="config-page">
        <div class="config-hero">
          <div>
            <h2>配置文件管理</h2>
            <p>管理 Codex 与 Claude Code（${escapeHtml(
              systemInfo.platformName || "macOS / Windows"
            )}）：自动搜索 · 手动选择 · 备份还原 · 切换模型。</p>
          </div>
          <div class="actions">
            <button class="btn btn-primary" id="btn-rescan" ${configsLoading ? "disabled" : ""}>
              ${configsLoading ? `<span class="spinner"></span> 搜索中…` : "重新自动搜索"}
            </button>
          </div>
        </div>
        <div class="config-grid">
          ${renderToolCard(codex || placeholder("codex", "Codex"))}
          ${renderToolCard(claude || placeholder("claude", "Claude Code"))}
        </div>
      </div>
    </div>
  `;
}

function placeholder(kind, name) {
  return {
    kind,
    name,
    path: "",
    found: false,
    exists: false,
    model: "",
    modelProvider: "",
    searchPaths: [],
    candidates: [],
    source: "auto",
    message: configsLoading ? "正在搜索…" : "尚未加载",
    hasDefaultBackup: false,
  };
}

function renderToolCard(st) {
  const kind = st.kind;
  const ok = !!st.found && !!st.exists;
  const choices = modelChoicesFor(kind);
  const selectedModel = pendingModel[kind] ?? st.model ?? "";
  const busy = configsBusy === kind;
  const msgClass = ok ? "ok" : st.message ? "warn" : "";

  const groups = new Map();
  for (const c of choices) {
    const g = c.group || "模型";
    if (!groups.has(g)) groups.set(g, []);
    groups.get(g).push(c);
  }

  return `
    <section class="config-card" data-kind="${escapeAttr(kind)}">
      <div class="config-card-head">
        <h3>
          <span class="status-dot ${ok ? "ok" : "fail"}"></span>
          ${escapeHtml(st.name)}
        </h3>
        <div class="meta-row">
          <span class="tag ${ok ? "ok" : "off"}">${ok ? "已定位" : "未找到"}</span>
          <span class="tag">${escapeHtml(st.source || "auto")}</span>
        </div>
      </div>
      <div class="config-card-body">
        <div class="path-box">
          <label style="font-size:12px;color:var(--text-secondary);font-weight:500">配置文件路径</label>
          <div class="path-value ${ok ? "" : "missing"}">${escapeHtml(st.path || "—")}</div>
        </div>

        <div class="actions">
          <button class="btn btn-sm" data-act="scan" data-kind="${kind}" ${busy ? "disabled" : ""}>自动搜索</button>
          <button class="btn btn-sm btn-primary" data-act="pick" data-kind="${kind}" ${busy ? "disabled" : ""}>手动选择</button>
          <button class="btn btn-sm" data-act="reveal" data-kind="${kind}" ${!st.path ? "disabled" : ""}>${escapeHtml(
            systemInfo.revealLabel || "在文件管理器中显示"
          )}</button>
          <button class="btn btn-sm btn-ghost" data-act="clear" data-kind="${kind}" ${st.source !== "override" ? "disabled" : ""}>清除手动路径</button>
        </div>

        <div class="actions" style="margin-top:2px">
          <button class="btn btn-sm" data-act="backup-default" data-kind="${kind}" ${busy || !ok ? "disabled" : ""}>
            ${st.hasDefaultBackup ? "更新默认备份" : "备份为默认"}
          </button>
          <button class="btn btn-sm" data-act="restore-default" data-kind="${kind}" ${
            busy || !st.hasDefaultBackup ? "disabled" : ""
          }>还原默认</button>
          <button class="btn btn-sm btn-ghost" data-act="clear-backup" data-kind="${kind}" ${
            busy || !st.hasDefaultBackup ? "disabled" : ""
          }>清除备份</button>
          ${st.hasDefaultBackup ? `<span class="tag ok">已有默认备份</span>` : `<span class="tag off">尚未备份</span>`}
        </div>
        ${
          st.hasDefaultBackup
            ? `<div class="hint">默认备份：${escapeHtml(formatBackupAt(st.defaultBackupAt))} · 首次修改前自动备份</div>`
            : `<div class="hint">切换模型前会自动备份当前配置，之后可一键还原</div>`
        }

        <div class="config-msg ${msgClass}">${escapeHtml(st.message || (ok ? "就绪" : "等待搜索"))}</div>

        <div class="form-grid" style="grid-template-columns:1fr 1fr">
          <div class="kv">
            <label>当前模型</label>
            <div class="value">${escapeHtml(st.model || "（未设置）")}</div>
          </div>
          <div class="kv">
            <label>${kind === "codex" ? "model_provider" : "备注"}</label>
            <div class="value">${escapeHtml(
              kind === "codex"
                ? st.modelProvider || "—"
                : st.modelProvider
                  ? `BASE ${st.modelProvider}`
                  : "model + env (教程兼容)"
            )}</div>
          </div>
        </div>

        <div class="field full">
          <label>切换模型</label>
          <div class="input-with-action">
            <select class="select" data-act="model-select" data-kind="${kind}">
              <option value="">选择模型…</option>
              ${[...groups.entries()]
                .map(
                  ([g, items]) => `
                <optgroup label="${escapeAttr(g)}">
                  ${items
                    .map(
                      (c) =>
                        `<option value="${escapeAttr(c.id)}" data-provider="${escapeAttr(c.provider || "")}" ${
                          c.id === selectedModel ? "selected" : ""
                        }>${escapeHtml(c.name)}${c.id !== c.name ? ` (${c.id})` : ""}</option>`
                    )
                    .join("")}
                </optgroup>`
                )
                .join("")}
            </select>
            <button class="btn btn-primary btn-sm" data-act="apply-model" data-kind="${kind}" ${
              busy || !selectedModel ? "disabled" : ""
            }>
              ${busy ? `<span class="spinner"></span>` : "应用"}
            </button>
          </div>
          <span class="hint">也可直接输入自定义模型 ID</span>
          <div class="input-with-action" style="margin-top:8px">
            <input class="input mono" data-act="model-input" data-kind="${kind}" value="${escapeAttr(
              selectedModel
            )}" placeholder="model id" />
            ${
              kind === "codex"
                ? `<input class="input mono" style="max-width:160px" data-act="provider-input" data-kind="${kind}" value="${escapeAttr(
                    pendingProvider[kind] || st.modelProvider || ""
                  )}" placeholder="provider" />`
                : ""
            }
          </div>
          ${
            kind === "claude"
              ? `<div class="actions" style="margin-top:10px">
                  <button class="btn btn-sm btn-primary" data-act="deepseek-claude" ${busy ? "disabled" : ""} title="按 DeepSeek 官方文档写入 ANTHROPIC_* 环境变量与 settings.json">
                    一键迁移 DeepSeek → Claude Code
                  </button>
                </div>
                <div class="hint">官方路径：ANTHROPIC_BASE_URL=https://api.deepseek.com/anthropic · 主模型 deepseek-v4-pro[1m] · 子代理 deepseek-v4-flash</div>`
              : ""
          }
        </div>

        ${
          st.searchPaths?.length
            ? `<div>
                <label style="font-size:12px;color:var(--text-secondary)">自动搜索路径</label>
                <ul class="search-paths">
                  ${(st.searchPaths || []).map((p) => `<li>${escapeHtml(p)}</li>`).join("")}
                </ul>
              </div>`
            : ""
        }

        ${
          configPreview[kind]
            ? `<div>
                <label style="font-size:12px;color:var(--text-secondary)">文件预览</label>
                <pre class="preview-box">${escapeHtml((configPreview[kind] || "").slice(0, 1200))}</pre>
              </div>`
            : ""
        }
      </div>
    </section>
  `;
}

function renderEmpty() {
  return `
    <div class="main-empty">
      <div>
        <div class="empty-icon">◎</div>
        <h3>选择或添加厂家</h3>
        <p>配置厂家名称、API Base URL 与 API Key，即可自动获取可用模型，并应用到 Codex / Claude Code。</p>
        <button class="btn btn-primary" id="btn-add-provider-empty">添加厂家</button>
      </div>
    </div>
  `;
}

function renderDetail(p) {
  const models = (p.models || []).filter((m) => {
    if (!modelQuery.trim()) return true;
    const q = modelQuery.trim().toLowerCase();
    return m.id.toLowerCase().includes(q) || (m.name || "").toLowerCase().includes(q);
  });

  return `
    <div class="main-scroll">
      <section class="panel">
        <div class="panel-head">
          <div>
            <h3>厂家配置</h3>
            <p class="desc">名称 · API Base URL · API Key · 持久化到本机</p>
          </div>
          <div class="actions">
            <button class="btn btn-danger btn-sm" id="btn-delete-provider">删除厂家</button>
          </div>
        </div>
        <div class="panel-body">
          <div class="form-grid">
            <div class="field">
              <label>厂家名称 <span class="req">*</span></label>
              <input class="input" id="f-name" value="${escapeAttr(p.name)}" placeholder="例如 OpenAI" />
            </div>
            <div class="field">
              <label>主题色</label>
              <input class="input" id="f-color" type="color" value="${escapeAttr(p.color || "#3d8bfd")}" style="padding:4px;height:36px" />
            </div>
            <div class="field full">
              <label>API Base URL <span class="req">*</span></label>
              <input class="input mono" id="f-base" value="${escapeAttr(p.baseUrl)}" placeholder="https://api.openai.com/v1" />
              <span class="hint">OpenAI 兼容接口：自动请求 {base}/models</span>
            </div>
            <div class="field full">
              <label>API Key ${isLocalProviderHint(p.name, p.baseUrl) ? "" : `<span class="req">*</span>`}</label>
              <div class="input-with-action">
                <input class="input mono" id="f-key" type="${showKey ? "text" : "password"}"
                  value="${escapeAttr(p.apiKey)}" placeholder="${isLocalProviderHint(p.name, p.baseUrl) ? "Ollama 可填 ollama 或留空" : "sk-..."}" autocomplete="off" />
                <button class="btn btn-sm" id="btn-toggle-key" type="button">${showKey ? "隐藏" : "显示"}</button>
              </div>
              <span class="hint">密钥保存在 ~/.codex-manager/providers.json（仅本机）</span>
            </div>
            <div class="field full">
              <label>访问方式</label>
              <div class="actions" style="margin-top:4px">
                <label class="stat-pill" style="cursor:pointer;gap:8px">
                  <input type="radio" name="f-proxy-mode" id="f-proxy-on" value="1" ${p.useProxy ? "checked" : ""} />
                  走本地代理
                </label>
                <label class="stat-pill" style="cursor:pointer;gap:8px">
                  <input type="radio" name="f-proxy-mode" id="f-proxy-off" value="0" ${!p.useProxy ? "checked" : ""} />
                  直连（不代理）
                </label>
              </div>
              <span class="hint">
                ${
                  p.useProxy
                    ? "保存后自动启动代理，并把该厂家在 Codex 中的 base_url 改为本地代理地址"
                    : "保存后使用上方 Base URL 直连（适合 Ollama 等本机服务）"
                }
              </span>
            </div>
          </div>
          <div class="actions" style="margin-top:16px">
            <button class="btn btn-primary" id="btn-save">保存配置</button>
            <button class="btn" id="btn-test" ${testing || fetching ? "disabled" : ""}>
              ${testing ? `<span class="spinner"></span> 测试中…` : "测试连接"}
            </button>
            <button class="btn" id="btn-fetch" ${fetching || testing ? "disabled" : ""}>
              ${fetching ? `<span class="spinner"></span> 获取中…` : "自动获取模型"}
            </button>
          </div>
          ${renderTestResultPanel()}
        </div>
      </section>

      <section class="panel">
        <div class="panel-head">
          <div>
            <h3>模型列表</h3>
            <p class="desc">可启用/禁用、设默认，并一键写入 Codex / Claude Code 配置</p>
          </div>
          <div class="actions">
            <button class="btn btn-sm" id="btn-refresh-models" ${fetching ? "disabled" : ""}>刷新获取</button>
          </div>
        </div>
        <div class="panel-body">
          <div class="models-toolbar">
            <input class="input search" id="f-search" placeholder="搜索模型 ID / 名称" value="${escapeAttr(modelQuery)}" />
            <span class="loading-inline">${models.length} / ${(p.models || []).length} 个模型</span>
          </div>
          <div class="model-table-wrap">
            ${
              models.length
                ? `<table class="model-table">
              <thead>
                <tr>
                  <th style="width:44px">启用</th>
                  <th>模型 ID</th>
                  <th>名称</th>
                  <th>状态</th>
                  <th style="width:280px;text-align:right">操作</th>
                </tr>
              </thead>
              <tbody>
                ${models
                  .map(
                    (m) => `
                  <tr data-model="${escapeAttr(m.id)}">
                    <td><button class="toggle ${m.enabled ? "on" : ""}" data-act="toggle" title="启用/禁用"></button></td>
                    <td><span class="model-id">${escapeHtml(m.id)}</span></td>
                    <td><span class="model-name">${escapeHtml(m.name || m.id)}</span></td>
                    <td>
                      ${m.isDefault ? `<span class="tag default">默认</span>` : ""}
                      ${m.enabled ? `<span class="tag ok">启用</span>` : `<span class="tag off">禁用</span>`}
                    </td>
                    <td>
                      <div class="row-actions">
                        <button class="btn btn-sm" data-act="default" ${m.isDefault || !m.enabled ? "disabled" : ""}>设为默认</button>
                        <button class="btn btn-sm" data-act="to-codex" title="写入 Codex 配置">→ Codex</button>
                        <button class="btn btn-sm" data-act="to-claude" title="写入 Claude Code 配置（DeepSeek 自动用官方 anthropic 端点）">→ Claude</button>
                        <button class="btn btn-sm btn-ghost" data-act="remove">移除</button>
                      </div>
                    </td>
                  </tr>`
                  )
                  .join("")}
              </tbody>
            </table>`
                : `<div class="empty-models">
                  <strong>${(p.models || []).length ? "没有匹配的模型" : "暂无模型"}</strong>
                  ${(p.models || []).length ? "试试其他搜索词" : "填写 API 与 Key 后点击「自动获取模型」"}
                </div>`
            }
          </div>
        </div>
      </section>
    </div>
  `;
}

function formatBackupAt(iso) {
  if (!iso) return "—";
  try {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleString();
  } catch {
    return iso;
  }
}

function renderTestResultPanel() {
  if (testing) {
    return `
      <div class="test-result loading" style="margin-top:14px">
        <div class="test-result-title"><span class="spinner"></span> 正在测试连接…</div>
        <div class="hint">请求 {baseUrl}/models，最长约 30 秒</div>
      </div>`;
  }
  if (!testResult) return "";

  const ok = !!testResult.ok;
  const samples = (testResult.sample || []).filter(Boolean);
  return `
    <div class="test-result ${ok ? "ok" : "err"}" style="margin-top:14px">
      <div class="test-result-title">
        <span class="status-dot ${ok ? "ok" : "fail"}"></span>
        ${escapeHtml(testResult.message || (ok ? "连接成功" : "连接失败"))}
      </div>
      <div class="test-result-grid">
        <div class="kv">
          <label>请求地址</label>
          <div class="value mono-sm">${escapeHtml(testResult.endpoint || "—")}</div>
        </div>
        <div class="kv">
          <label>HTTP 状态</label>
          <div class="value">${testResult.statusCode ? escapeHtml(String(testResult.statusCode)) : "—"}</div>
        </div>
        <div class="kv">
          <label>耗时</label>
          <div class="value">${testResult.latencyMs != null ? `${testResult.latencyMs} ms` : "—"}</div>
        </div>
        <div class="kv">
          <label>模型数量</label>
          <div class="value">${ok ? escapeHtml(String(testResult.modelCount ?? 0)) : "—"}</div>
        </div>
      </div>
      ${
        samples.length
          ? `<div class="test-sample">
              <label>示例模型</label>
              <div class="sample-tags">
                ${samples.map((s) => `<span class="tag">${escapeHtml(s)}</span>`).join("")}
                ${(testResult.modelCount || 0) > samples.length ? `<span class="tag off">+${(testResult.modelCount || 0) - samples.length} 更多</span>` : ""}
              </div>
            </div>`
          : ""
      }
      ${
        testResult.error
          ? `<div class="test-error"><label>错误详情</label><pre class="preview-box">${escapeHtml(testResult.error)}</pre></div>`
          : ""
      }
    </div>
  `;
}

function escapeHtml(s) {
  return String(s ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function escapeAttr(s) {
  return escapeHtml(s).replaceAll("'", "&#39;");
}

function bindShellEvents() {
  document.querySelectorAll(".nav-tab").forEach((el) => {
    el.addEventListener("click", () => {
      const p = el.dataset.page;
      page = ["configs", "proxy", "usage"].includes(p) ? p : "providers";
      localStorage.setItem(PAGE_KEY, page);
      render();
      if (page === "configs" && !Object.keys(toolConfigs).length) {
        loadToolConfigs(false);
      }
      if (page === "proxy") {
        refreshProxyStatus();
      }
      if (page === "usage") {
        loadUsageStats().then(() => render());
      }
    });
  });
}

async function refreshProxyStatus() {
  if (!hasBackend() || typeof window.go?.main?.App?.GetProxyStatus !== "function") {
    proxyStatus = { running: false, lastError: "请在 Wails 应用中运行", logs: [] };
    return;
  }
  try {
    const st = await GetProxyStatus();
    proxyStatus = normalizeProxyStatus(st);
    const cfg = await GetProxyConfig();
    if (cfg) {
      proxyForm = {
        host: cfg.host || cfg.Host || "127.0.0.1",
        port: cfg.port || cfg.Port || 18080,
        autoStart: !!(cfg.autoStart ?? cfg.AutoStart),
        listenKey: cfg.listenKey || cfg.ListenKey || "",
      };
    }
  } catch (e) {
    proxyStatus = { running: false, lastError: e?.message || String(e), logs: [] };
  }
}

function normalizeProxyStatus(st) {
  if (!st) return { running: false, logs: [] };
  return {
    running: !!(st.running ?? st.Running),
    baseUrl: st.baseUrl || st.BaseURL || "",
    host: st.host || st.Host || "",
    port: st.port || st.Port || 18080,
    autoStart: !!(st.autoStart ?? st.AutoStart),
    listenKey: st.listenKey || st.ListenKey || "",
    lastError: st.lastError || st.LastError || "",
    logs: st.logs || st.Logs || [],
  };
}

function readProxyForm() {
  const host = document.getElementById("proxy-host")?.value?.trim() || "127.0.0.1";
  const port = Number(document.getElementById("proxy-port")?.value || 18080);
  const listenKey = document.getElementById("proxy-listen-key")?.value?.trim() || "";
  const autoStart = !!document.getElementById("proxy-autostart")?.checked;
  proxyForm = { host, port, listenKey, autoStart };
  return {
    enabled: !!proxyStatus?.running,
    host,
    port,
    autoStart,
    listenKey,
  };
}

function bindProxyEvents() {
  document.getElementById("btn-proxy-refresh")?.addEventListener("click", async () => {
    proxyBusy = true;
    render();
    await refreshProxyStatus();
    proxyBusy = false;
    render();
  });

  document.getElementById("btn-proxy-start")?.addEventListener("click", async () => {
    proxyBusy = true;
    render();
    try {
      if (!hasBackend()) throw new Error("请在 Wails 应用中运行");
      const cfg = readProxyForm();
      await SaveProxyConfig(cfg);
      const st = await StartProxy();
      proxyStatus = normalizeProxyStatus(st);
      toast(proxyStatus.running ? "代理已启动" : "启动失败", proxyStatus.running ? "ok" : "err");
    } catch (e) {
      toast(e?.message || String(e), "err");
      await refreshProxyStatus();
    } finally {
      proxyBusy = false;
      render();
    }
  });

  document.getElementById("btn-proxy-stop")?.addEventListener("click", async () => {
    proxyBusy = true;
    render();
    try {
      const st = await StopProxy();
      proxyStatus = normalizeProxyStatus(st);
      toast("代理已停止");
    } catch (e) {
      toast(e?.message || String(e), "err");
    } finally {
      proxyBusy = false;
      render();
    }
  });

  document.getElementById("btn-proxy-save")?.addEventListener("click", async () => {
    proxyBusy = true;
    render();
    try {
      const cfg = readProxyForm();
      const st = await SaveProxyConfig(cfg);
      proxyStatus = normalizeProxyStatus(st);
      toast("代理配置已保存");
    } catch (e) {
      toast(e?.message || String(e), "err");
    } finally {
      proxyBusy = false;
      render();
    }
  });

  document.getElementById("btn-copy-base")?.addEventListener("click", async () => {
    const v = document.getElementById("proxy-base-url")?.value || "";
    try {
      await navigator.clipboard.writeText(v);
      toast("已复制 Base URL");
    } catch {
      toast("复制失败，请手动选择", "err");
    }
  });
}

function renderUsagePage() {
  const t = usageStats?.total || usageStats?.Total || {};
  const calls = t.calls ?? t.Calls ?? 0;
  const input = t.inputTokens ?? t.InputTokens ?? 0;
  const output = t.outputTokens ?? t.OutputTokens ?? 0;
  const total = t.totalTokens ?? t.TotalTokens ?? 0;
  const byModel = usageStats?.byModel || usageStats?.ByModel || [];
  const byProvider = usageStats?.byProvider || usageStats?.ByProvider || [];
  const byDay = usageStats?.byDay || usageStats?.ByDay || [];
  const recent = usageStats?.recent || usageStats?.Recent || [];

  const rowBucket = (b) => {
    const key = b.key ?? b.Key ?? "";
    const c = b.calls ?? b.Calls ?? 0;
    const i = b.inputTokens ?? b.InputTokens ?? 0;
    const o = b.outputTokens ?? b.OutputTokens ?? 0;
    const tot = b.totalTokens ?? b.TotalTokens ?? 0;
    return `<tr>
      <td class="model-id">${escapeHtml(key)}</td>
      <td>${c}</td>
      <td>${i.toLocaleString()}</td>
      <td>${o.toLocaleString()}</td>
      <td><strong>${tot.toLocaleString()}</strong></td>
    </tr>`;
  };

  return `
    <div class="full-page">
      <div class="config-page">
        <div class="config-hero">
          <div>
            <h2>Token 使用统计</h2>
            <p>统计经本地代理转发的请求用量（输入 / 输出 / 合计）。仅记录代理链路，直连 Ollama 不经过代理则不计入。</p>
          </div>
          <div class="actions">
            <button class="btn" id="btn-usage-refresh" ${usageBusy ? "disabled" : ""}>刷新</button>
            <button class="btn btn-ghost" id="btn-usage-clear" ${usageBusy ? "disabled" : ""}>清空统计</button>
          </div>
        </div>

        <div class="config-grid" style="grid-template-columns:repeat(4,1fr)">
          <section class="config-card" style="min-height:auto">
            <div class="config-card-body">
              <div class="kv"><label>请求次数</label><div class="value" style="font-size:22px;font-weight:700">${calls}</div></div>
            </div>
          </section>
          <section class="config-card" style="min-height:auto">
            <div class="config-card-body">
              <div class="kv"><label>输入 Tokens</label><div class="value" style="font-size:22px;font-weight:700">${Number(input).toLocaleString()}</div></div>
            </div>
          </section>
          <section class="config-card" style="min-height:auto">
            <div class="config-card-body">
              <div class="kv"><label>输出 Tokens</label><div class="value" style="font-size:22px;font-weight:700">${Number(output).toLocaleString()}</div></div>
            </div>
          </section>
          <section class="config-card" style="min-height:auto">
            <div class="config-card-body">
              <div class="kv"><label>合计 Tokens</label><div class="value" style="font-size:22px;font-weight:700;color:var(--accent)">${Number(total).toLocaleString()}</div></div>
            </div>
          </section>
        </div>

        <div class="config-grid">
          <section class="config-card">
            <div class="config-card-head"><h3>按模型</h3></div>
            <div class="config-card-body">
              <div class="model-table-wrap">
                <table class="model-table">
                  <thead><tr><th>模型</th><th>次数</th><th>输入</th><th>输出</th><th>合计</th></tr></thead>
                  <tbody>${byModel.length ? byModel.map(rowBucket).join("") : `<tr><td colspan="5">暂无数据</td></tr>`}</tbody>
                </table>
              </div>
            </div>
          </section>
          <section class="config-card">
            <div class="config-card-head"><h3>按厂家</h3></div>
            <div class="config-card-body">
              <div class="model-table-wrap">
                <table class="model-table">
                  <thead><tr><th>厂家</th><th>次数</th><th>输入</th><th>输出</th><th>合计</th></tr></thead>
                  <tbody>${byProvider.length ? byProvider.map(rowBucket).join("") : `<tr><td colspan="5">暂无数据</td></tr>`}</tbody>
                </table>
              </div>
            </div>
          </section>
        </div>

        <section class="panel" style="margin-top:4px">
          <div class="panel-head"><div><h3>按日（近 30 天）</h3></div></div>
          <div class="panel-body">
            <div class="model-table-wrap">
              <table class="model-table">
                <thead><tr><th>日期</th><th>次数</th><th>输入</th><th>输出</th><th>合计</th></tr></thead>
                <tbody>${byDay.length ? byDay.map(rowBucket).join("") : `<tr><td colspan="5">暂无数据</td></tr>`}</tbody>
              </table>
            </div>
          </div>
        </section>

        <section class="panel" style="margin-top:4px">
          <div class="panel-head"><div><h3>最近请求</h3><p class="desc">最多 50 条</p></div></div>
          <div class="panel-body">
            <div class="model-table-wrap" style="max-height:280px;overflow:auto">
              <table class="model-table">
                <thead><tr><th>时间</th><th>厂家</th><th>模型</th><th>端点</th><th>输入</th><th>输出</th><th>合计</th></tr></thead>
                <tbody>
                  ${
                    recent.length
                      ? recent
                          .map((e) => {
                            const time = (e.time || e.Time || "").replace("T", " ").slice(0, 19);
                            return `<tr>
                              <td class="model-id">${escapeHtml(time)}</td>
                              <td>${escapeHtml(e.provider || e.Provider || "")}</td>
                              <td class="model-id">${escapeHtml(e.model || e.Model || "")}</td>
                              <td>${escapeHtml(e.endpoint || e.Endpoint || "")}</td>
                              <td>${e.inputTokens ?? e.InputTokens ?? 0}</td>
                              <td>${e.outputTokens ?? e.OutputTokens ?? 0}</td>
                              <td><strong>${e.totalTokens ?? e.TotalTokens ?? 0}</strong></td>
                            </tr>`;
                          })
                          .join("")
                      : `<tr><td colspan="7">暂无记录。请启动代理并用 Codex 经代理访问后刷新。</td></tr>`
                  }
                </tbody>
              </table>
            </div>
          </div>
        </section>
      </div>
    </div>
  `;
}

async function loadUsageStats() {
  if (!hasBackend() || typeof GetUsageStats !== "function") {
    usageStats = { total: { calls: 0, inputTokens: 0, outputTokens: 0, totalTokens: 0 }, byDay: [], byModel: [], byProvider: [], recent: [] };
    return;
  }
  usageBusy = true;
  try {
    usageStats = await GetUsageStats();
  } catch (e) {
    toast(e?.message || String(e), "err");
  } finally {
    usageBusy = false;
  }
}

function bindUsageEvents() {
  document.getElementById("btn-usage-refresh")?.addEventListener("click", async () => {
    await loadUsageStats();
    render();
    toast("统计已刷新");
  });
  document.getElementById("btn-usage-clear")?.addEventListener("click", async () => {
    if (!confirm("确定清空全部 Token 统计？此操作不可恢复。")) return;
    try {
      if (!hasBackend()) throw new Error("请在 Wails 应用中运行");
      usageStats = await ClearUsageStats();
      toast("已清空统计");
      render();
    } catch (e) {
      toast(e?.message || String(e), "err");
    }
  });
}

function bindConfigEvents() {
  document.getElementById("btn-rescan")?.addEventListener("click", () => loadToolConfigs(true));

  document.querySelectorAll("[data-act='scan']").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const kind = btn.dataset.kind;
      configsBusy = kind;
      render();
      try {
        if (!hasBackend()) throw new Error("请在 Wails 应用中运行");
        const st = await ClearToolConfigPath(kind);
        await applyToolStatus(st);
        if (!st.found) toast(`${st.name} 自动搜索失败，请手动选择`, "err");
        else toast(`${st.name} 已定位`);
      } catch (e) {
        toast(e?.message || String(e), "err");
      } finally {
        configsBusy = "";
        render();
      }
    });
  });

  document.querySelectorAll("[data-act='pick']").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const kind = btn.dataset.kind;
      configsBusy = kind;
      render();
      try {
        if (!hasBackend()) throw new Error("请在 Wails 应用中运行");
        const st = await PickToolConfig(kind);
        await applyToolStatus(st);
        if (st.found) toast(`已选择 ${st.path}`);
      } catch (e) {
        toast(e?.message || String(e), "err");
      } finally {
        configsBusy = "";
        render();
      }
    });
  });

  document.querySelectorAll("[data-act='clear']").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const kind = btn.dataset.kind;
      try {
        const st = await ClearToolConfigPath(kind);
        await applyToolStatus(st);
        toast("已清除手动路径并重新搜索");
        render();
      } catch (e) {
        toast(e?.message || String(e), "err");
      }
    });
  });

  document.querySelectorAll("[data-act='reveal']").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const kind = btn.dataset.kind;
      try {
        await RevealConfigPath(toolConfigs[kind]?.path || "");
      } catch (e) {
        toast(e?.message || String(e), "err");
      }
    });
  });

  document.querySelectorAll("[data-act='backup-default']").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const kind = btn.dataset.kind;
      configsBusy = kind;
      render();
      try {
        if (!hasBackend()) throw new Error("请在 Wails 应用中运行");
        const st = await BackupDefaultConfig(kind, toolConfigs[kind]?.path || "");
        await applyToolStatus(st);
        toast(st.message || "已备份为默认配置");
      } catch (e) {
        toast(e?.message || String(e), "err");
      } finally {
        configsBusy = "";
        render();
      }
    });
  });

  document.querySelectorAll("[data-act='deepseek-claude']").forEach((btn) => {
    btn.addEventListener("click", async () => {
      configsBusy = "claude";
      render();
      try {
        if (!hasBackend()) throw new Error("请在 Wails 应用中运行");
        // prefer DeepSeek provider key from app storage
        const ds =
          providers.find((p) => {
            const n = (p.name || "").toLowerCase();
            const u = (p.baseUrl || "").toLowerCase();
            return n.includes("deepseek") || u.includes("deepseek");
          }) || null;
        const mainFromUI =
          document.querySelector(`[data-act='model-input'][data-kind='claude']`)?.value?.trim() ||
          pendingModel.claude ||
          "";
        const st = await ApplyDeepSeekClaudeCode({
          apiKey: ds?.apiKey || "",
          path: toolConfigs.claude?.path || "",
          mainModel: mainFromUI || "deepseek-v4-pro[1m]",
          haikuModel: "deepseek-v4-flash",
          effortLevel: "max",
          setSystemEnv: true,
        });
        await applyToolStatus(st);
        toast(st.message || "已迁移 Claude Code → DeepSeek");
      } catch (e) {
        toast(e?.message || String(e), "err");
      } finally {
        configsBusy = "";
        render();
      }
    });
  });

  document.querySelectorAll("[data-act='restore-default']").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const kind = btn.dataset.kind;
      const name = toolConfigs[kind]?.name || kind;
      if (!confirm(`确定将「${name}」还原为默认备份？\n当前配置会先写入历史快照。`)) return;
      configsBusy = kind;
      render();
      try {
        if (!hasBackend()) throw new Error("请在 Wails 应用中运行");
        const st = await RestoreDefaultConfig(kind);
        await applyToolStatus(st);
        toast(st.message || "已还原默认配置");
      } catch (e) {
        toast(e?.message || String(e), "err");
      } finally {
        configsBusy = "";
        render();
      }
    });
  });

  document.querySelectorAll("[data-act='clear-backup']").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const kind = btn.dataset.kind;
      if (!confirm("清除默认备份后，将无法还原。确定？")) return;
      configsBusy = kind;
      render();
      try {
        if (!hasBackend()) throw new Error("请在 Wails 应用中运行");
        const st = await ClearDefaultBackup(kind);
        await applyToolStatus(st);
        toast(st.message || "已清除默认备份");
      } catch (e) {
        toast(e?.message || String(e), "err");
      } finally {
        configsBusy = "";
        render();
      }
    });
  });

  document.querySelectorAll("[data-act='model-select']").forEach((sel) => {
    sel.addEventListener("change", () => {
      const kind = sel.dataset.kind;
      pendingModel[kind] = sel.value;
      const opt = sel.selectedOptions?.[0];
      if (opt?.dataset?.provider) pendingProvider[kind] = opt.dataset.provider;
      const input = document.querySelector(`[data-act='model-input'][data-kind='${kind}']`);
      if (input) input.value = sel.value;
      const prov = document.querySelector(`[data-act='provider-input'][data-kind='${kind}']`);
      if (prov && opt?.dataset?.provider) prov.value = opt.dataset.provider;
      const applyBtn = document.querySelector(`[data-act='apply-model'][data-kind='${kind}']`);
      if (applyBtn) applyBtn.disabled = !sel.value;
    });
  });

  document.querySelectorAll("[data-act='model-input']").forEach((input) => {
    input.addEventListener("input", () => {
      const kind = input.dataset.kind;
      pendingModel[kind] = input.value.trim();
      const applyBtn = document.querySelector(`[data-act='apply-model'][data-kind='${kind}']`);
      if (applyBtn) applyBtn.disabled = !pendingModel[kind];
    });
  });

  document.querySelectorAll("[data-act='provider-input']").forEach((input) => {
    input.addEventListener("input", () => {
      pendingProvider[input.dataset.kind] = input.value.trim();
    });
  });

  document.querySelectorAll("[data-act='apply-model']").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const kind = btn.dataset.kind;
      const model =
        document.querySelector(`[data-act='model-input'][data-kind='${kind}']`)?.value?.trim() ||
        pendingModel[kind] ||
        "";
      const provider =
        document.querySelector(`[data-act='provider-input'][data-kind='${kind}']`)?.value?.trim() ||
        pendingProvider[kind] ||
        "";
      if (!model) return toast("请选择或输入模型", "err");
      let usePath = toolConfigs[kind]?.path || "";
      configsBusy = kind;
      render();
      try {
        if (!hasBackend()) throw new Error("请在 Wails 应用中运行");
        if (!toolConfigs[kind]?.exists) {
          toast("配置文件不存在，请先手动选择路径", "err");
          const st = await PickToolConfig(kind);
          await applyToolStatus(st);
          if (!st.path) throw new Error("未选择路径");
          usePath = st.path;
        }
        // try match provider from our saved vendors
        const matched = findProviderForModel(model);
        const st = await ApplyToolModel({
          kind,
          path: usePath,
          model,
          provider: provider || matched?.providerId || "",
          baseUrl: matched?.baseUrl || "",
          apiKey: matched?.apiKey || "",
          name: matched?.name || model,
        });
        await applyToolStatus(st);
        toast(st.message || `已切换为 ${model}`);
      } catch (e) {
        toast(e?.message || String(e), "err");
      } finally {
        configsBusy = "";
        render();
      }
    });
  });
}

/** Find app vendor that owns this model id for baseUrl/apiKey injection */
function findProviderForModel(modelId) {
  for (const p of providers) {
    const m = (p.models || []).find((x) => x.id === modelId);
    if (m) {
      return {
        baseUrl: p.baseUrl || "",
        apiKey: p.apiKey || "",
        name: m.name || modelId,
        providerId: slugFromProvider(p),
      };
    }
  }
  return null;
}

function slugFromProvider(p) {
  const name = (p.name || "").toLowerCase();
  const url = (p.baseUrl || "").toLowerCase();
  if (name.includes("deepseek") || url.includes("deepseek")) return "deepseek";
  if (name.includes("openai") || url.includes("openai.com")) return "openai";
  if (name.includes("anthropic") || url.includes("anthropic")) return "anthropic";
  if (name.includes("moonshot") || url.includes("moonshot")) return "moonshot";
  if (name.includes("通义") || name.includes("qwen") || url.includes("dashscope")) return "qwen";
  if (name.includes("智谱") || name.includes("zhipu") || url.includes("bigmodel")) return "zhipu";
  if (name.includes("minimax") || url.includes("minimax")) return "minimax";
  // safe toml table key
  return (p.name || "custom")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_|_$/g, "") || "custom";
}

function readFormInto(p) {
  const name = document.getElementById("f-name");
  const base = document.getElementById("f-base");
  const key = document.getElementById("f-key");
  const color = document.getElementById("f-color");
  if (name) p.name = name.value;
  if (base) p.baseUrl = base.value.trim().replace(/\/$/, "");
  if (key) p.apiKey = key.value;
  if (color) p.color = color.value;
  const proxyOn = document.getElementById("f-proxy-on");
  if (proxyOn) p.useProxy = !!proxyOn.checked;
}

async function applyModelToTool(kind, modelId) {
  if (!hasBackend()) throw new Error("请在 Wails 应用中运行");
  // ensure configs loaded
  if (!toolConfigs[kind]) {
    await loadToolConfigs(false);
  }
  let st = toolConfigs[kind];
  if (!st?.exists) {
    toast(`${kind === "codex" ? "Codex" : "Claude"} 配置未找到，请手动选择`, "err");
    page = "configs";
    localStorage.setItem(PAGE_KEY, page);
    render();
    await loadToolConfigs(false);
    st = await PickToolConfig(kind);
    await applyToolStatus(st);
    if (!st?.path) throw new Error("未选择配置文件");
  }
  const path = toolConfigs[kind]?.path || st.path;
  const p = selected();
  const m = p?.models?.find((x) => x.id === modelId);
  const matched = findProviderForModel(modelId);
  const providerId = matched?.providerId || slugFromProvider(p || { name: "custom", baseUrl: "" });
  const baseUrl = p?.baseUrl || matched?.baseUrl || "";
  const apiKey = p?.apiKey || matched?.apiKey || "";

  // Claude + DeepSeek → official migration (anthropic endpoint + multi-model env)
  if (kind === "claude" && isDeepSeekVendor(p, providerId, baseUrl, modelId)) {
    const result = await ApplyDeepSeekClaudeCode({
      apiKey,
      path,
      mainModel: modelId,
      haikuModel: "deepseek-v4-flash",
      effortLevel: "max",
      setSystemEnv: true,
    });
    await applyToolStatus(result);
    return result;
  }

  const result = await ApplyToolModel({
    kind,
    path,
    model: modelId,
    provider: providerId,
    baseUrl,
    apiKey,
    name: m?.name || matched?.name || modelId,
  });
  await applyToolStatus(result);
  return result;
}

function isDeepSeekVendor(p, providerId, baseUrl, modelId) {
  const n = `${p?.name || ""} ${providerId || ""} ${baseUrl || ""} ${modelId || ""}`.toLowerCase();
  return n.includes("deepseek");
}

function bindProviderEvents() {
  document.getElementById("btn-add-provider")?.addEventListener("click", openAddModal);
  document.getElementById("btn-add-provider-empty")?.addEventListener("click", openAddModal);

  document.querySelectorAll(".provider-item").forEach((el) => {
    el.addEventListener("click", () => {
      selectedId = el.dataset.id;
      showKey = false;
      modelQuery = "";
      testResult = null;
      render();
    });
  });

  const p = selected();
  if (!p) return;

  document.getElementById("btn-toggle-key")?.addEventListener("click", () => {
    readFormInto(p);
    showKey = !showKey;
    render();
  });

  document.getElementById("btn-save")?.addEventListener("click", async () => {
    readFormInto(p);
    if (!p.name.trim()) return toast("请填写厂家名称", "err");
    if (!p.baseUrl.trim()) return toast("请填写 API Base URL", "err");
    try {
      await persistProviders();
      // SaveProviders backend already EnsureProxyRouting; refresh proxy status for UI
      if (p.useProxy && hasBackend() && typeof EnsureProxyRouting === "function") {
        try {
          const st = await EnsureProxyRouting();
          proxyStatus = normalizeProxyStatus(st);
        } catch (_) {}
      }
      toast(p.useProxy ? "已保存，并自动配置本地代理" : "已保存（直连模式）");
      render();
    } catch (e) {
      toast(e?.message || String(e), "err");
    }
  });

  document.getElementById("f-proxy-on")?.addEventListener("change", () => {
    readFormInto(p);
    render();
  });
  document.getElementById("f-proxy-off")?.addEventListener("change", () => {
    readFormInto(p);
    render();
  });

  document.getElementById("btn-test")?.addEventListener("click", async () => {
    readFormInto(p);
    const local = isLocalProviderHint(p.name, p.baseUrl);
    if (!p.baseUrl.trim() || (!local && !p.apiKey.trim())) {
      testResult = {
        ok: false,
        message: "测试失败",
        error: local ? "请先填写 API Base URL" : "请先填写 API Base URL 与 API Key",
      };
      toast(testResult.error, "err");
      render();
      return;
    }
    testing = true;
    testResult = null;
    render();
    try {
      if (!hasBackend()) {
        testResult = {
          ok: false,
          message: "无法测试",
          error: "请在 Wails 应用中运行（wails dev）",
          endpoint: `${p.baseUrl.replace(/\/$/, "")}/models`,
        };
        toast(testResult.error, "err");
        return;
      }
      const res = await TestProviderConnection(p.baseUrl, p.apiKey);
      // Wails may return struct as-is
      testResult = {
        ok: !!(res?.ok ?? res?.OK),
        message: res?.message || res?.Message || "",
        endpoint: res?.endpoint || res?.Endpoint || "",
        statusCode: res?.statusCode ?? res?.StatusCode ?? 0,
        latencyMs: res?.latencyMs ?? res?.LatencyMs ?? null,
        modelCount: res?.modelCount ?? res?.ModelCount ?? 0,
        sample: res?.sample || res?.Sample || [],
        error: res?.error || res?.Error || "",
      };
      toast(testResult.message || (testResult.ok ? "连接成功" : "连接失败"), testResult.ok ? "ok" : "err");
    } catch (e) {
      testResult = {
        ok: false,
        message: "测试异常",
        error: e?.message || String(e),
        endpoint: `${p.baseUrl.replace(/\/$/, "")}/models`,
      };
      toast(testResult.error, "err");
    } finally {
      testing = false;
      render();
    }
  });

  const doFetch = async () => {
    readFormInto(p);
    if (!p.name.trim() || !p.baseUrl.trim()) return toast("请先完善名称与 API", "err");
    if (!p.apiKey.trim()) return toast("请先填写 API Key", "err");
    fetching = true;
    render();
    try {
      const list = await fetchModelsFromApi(p);
      mergeFetchedModels(p, list);
      await persistProviders();
      toast(`已获取 ${list.length} 个模型`);
    } catch (e) {
      toast(e?.message || String(e), "err");
    } finally {
      fetching = false;
      render();
    }
  };

  document.getElementById("btn-fetch")?.addEventListener("click", doFetch);
  document.getElementById("btn-refresh-models")?.addEventListener("click", doFetch);

  document.getElementById("btn-delete-provider")?.addEventListener("click", async () => {
    if (!confirm(`确定删除厂家「${p.name}」及其模型？`)) return;
    providers = providers.filter((x) => x.id !== p.id);
    selectedId = providers[0]?.id ?? null;
    try {
      await persistProviders();
      toast("已删除厂家");
      render();
    } catch (e) {
      toast(e?.message || String(e), "err");
    }
  });

  const search = document.getElementById("f-search");
  search?.addEventListener("input", () => {
    modelQuery = search.value;
    readFormInto(p);
    render();
    const again = document.getElementById("f-search");
    if (again) {
      again.focus();
      again.setSelectionRange(modelQuery.length, modelQuery.length);
    }
  });

  document.querySelectorAll("tr[data-model]").forEach((row) => {
    const mid = row.dataset.model;
    row.querySelector('[data-act="toggle"]')?.addEventListener("click", async () => {
      const m = p.models.find((x) => x.id === mid);
      if (!m) return;
      m.enabled = !m.enabled;
      if (!m.enabled && m.isDefault) m.isDefault = false;
      try {
        await persistProviders();
        render();
      } catch (e) {
        toast(e?.message || String(e), "err");
      }
    });
    row.querySelector('[data-act="default"]')?.addEventListener("click", async () => {
      p.models.forEach((x) => (x.isDefault = x.id === mid));
      const m = p.models.find((x) => x.id === mid);
      if (m) m.enabled = true;
      try {
        await persistProviders();
        toast(`默认模型：${mid}`);
        render();
      } catch (e) {
        toast(e?.message || String(e), "err");
      }
    });
    row.querySelector('[data-act="remove"]')?.addEventListener("click", async () => {
      p.models = p.models.filter((x) => x.id !== mid);
      try {
        await persistProviders();
        render();
      } catch (e) {
        toast(e?.message || String(e), "err");
      }
    });
    row.querySelector('[data-act="to-codex"]')?.addEventListener("click", async () => {
      try {
        const st = await applyModelToTool("codex", mid);
        toast(st.message || `已写入 Codex：${mid}`);
      } catch (e) {
        toast(e?.message || String(e), "err");
      }
    });
    row.querySelector('[data-act="to-claude"]')?.addEventListener("click", async () => {
      try {
        const st = await applyModelToTool("claude", mid);
        toast(st.message || `已写入 Claude：${mid}`);
      } catch (e) {
        toast(e?.message || String(e), "err");
      }
    });
  });
}

function openAddModal() {
  const root = document.getElementById("modal-root");
  root.innerHTML = `
    <div class="modal-backdrop" id="modal-backdrop">
      <div class="modal" role="dialog" aria-modal="true">
        <div class="modal-head">
          <h3>添加厂家</h3>
          <button class="btn btn-sm btn-ghost" id="modal-close">关闭</button>
        </div>
        <div class="modal-body">
          <div class="field">
            <label>快速预设</label>
            <select class="select" id="m-preset">
              ${PRESETS.map((x, i) => `<option value="${i}">${escapeHtml(x.name)}</option>`).join("")}
            </select>
          </div>
          <div class="field">
            <label>厂家名称 <span class="req">*</span></label>
            <input class="input" id="m-name" value="${escapeAttr(PRESETS[0].name)}" />
          </div>
          <div class="field">
            <label>API Base URL <span class="req">*</span></label>
            <input class="input mono" id="m-base" value="${escapeAttr(PRESETS[0].baseUrl)}" />
          </div>
          <div class="field">
            <label>API Key</label>
            <input class="input mono" id="m-key" type="password" placeholder="Ollama 可填 ollama" value="${escapeAttr(PRESETS[0].apiKey || "")}" />
          </div>
          <div class="field">
            <label>访问方式</label>
            <select class="select" id="m-proxy">
              <option value="0" ${PRESETS[0].useProxy === false ? "selected" : ""}>直连（不代理）</option>
              <option value="1" ${PRESETS[0].useProxy !== false ? "selected" : ""}>走本地代理</option>
            </select>
          </div>
        </div>
        <div class="modal-foot">
          <button class="btn" id="modal-cancel">取消</button>
          <button class="btn btn-primary" id="modal-ok">添加</button>
        </div>
      </div>
    </div>
  `;

  const close = () => {
    root.innerHTML = "";
  };

  document.getElementById("m-preset")?.addEventListener("change", (e) => {
    const preset = PRESETS[Number(e.target.value)];
    if (!preset) return;
    document.getElementById("m-name").value = preset.name === "自定义" ? "" : preset.name;
    document.getElementById("m-base").value = preset.baseUrl === "https://" ? "" : preset.baseUrl;
    const keyEl = document.getElementById("m-key");
    if (keyEl) keyEl.value = preset.apiKey || "";
    const proxyEl = document.getElementById("m-proxy");
    if (proxyEl) proxyEl.value = preset.useProxy === false ? "0" : "1";
  });

  document.getElementById("modal-close")?.addEventListener("click", close);
  document.getElementById("modal-cancel")?.addEventListener("click", close);
  document.getElementById("modal-backdrop")?.addEventListener("click", (e) => {
    if (e.target.id === "modal-backdrop") close();
  });

  document.getElementById("modal-ok")?.addEventListener("click", async () => {
    const name = document.getElementById("m-name").value.trim();
    const baseUrl = document.getElementById("m-base").value.trim().replace(/\/$/, "");
    let apiKey = document.getElementById("m-key").value.trim();
    const presetIdx = Number(document.getElementById("m-preset").value);
    const color = PRESETS[presetIdx]?.color || COLORS[providers.length % COLORS.length];
    const useProxy = document.getElementById("m-proxy")?.value === "1";
    if (!apiKey && isLocalProviderHint(name, baseUrl)) apiKey = "ollama";

    if (!name) return toast("请填写厂家名称", "err");
    if (!baseUrl) return toast("请填写 API Base URL", "err");

    const item = {
      id: name.toLowerCase() === "ollama" ? "ollama" : uid(),
      name,
      baseUrl,
      apiKey,
      color,
      useProxy,
      models: [],
    };
    providers.push(item);
    selectedId = item.id;
    showKey = false;
    modelQuery = "";
    try {
      await persistProviders();
      close();
      toast(`已添加 ${name}`);
      render();
    } catch (e) {
      providers = providers.filter((x) => x.id !== item.id);
      toast(e?.message || String(e), "err");
    }
  });
}

// boot
(async () => {
  booting = true;
  render();
  await loadSystemInfo();
  await loadProviders();
  await refreshProxyStatus();
  booting = false;
  render();
  if (page === "configs") {
    loadToolConfigs(false);
  }
  if (page === "proxy") {
    // already refreshed
  }
})();
