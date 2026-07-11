import "./style.css";
import "./app.css";
import { t, getLocale, setLocale, getLocaleMeta, LOCALES, revealLabelForOs, tb, localeBcp47, usesChineseUnits } from "./i18n.js";

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
  GetProviderPackageStatuses,
  ListModelGroups,
  SetModelGroupRoutePriority,
  SetModelGroupRouteEnabled,
  ReorderModelGroupRoutes,
  InjectGateway,
  RollbackGateway,
} from "../wailsjs/go/main/App";

/** @typedef {{ id: string, name: string, baseUrl: string, apiKey: string, color: string, models: Model[] }} Provider */
/** @typedef {{ id: string, name: string, enabled: boolean, isDefault: boolean, ownedBy?: string }} Model */
/** @typedef {{ kind: string, name: string, path: string, found: boolean, exists: boolean, model: string, modelProvider: string, searchPaths: string[], candidates: {id:string,name:string,provider:string}[], source: string, message: string, hasDefaultBackup?: boolean, defaultBackupAt?: string }} ToolConfigStatus */

const COLORS = ["#3d8bfd", "#7c5cff", "#3fb950", "#d29922", "#f85149", "#39c5cf", "#e85d9a", "#3859ff", "#a371f7"];

/**
 * Built-in provider preset library (PRD 3.2).
 * User picks a card → only fills API Key for most cloud vendors.
 * @type {Array<{
 *   id: string, name: string, nameKey?: string|null, baseUrl: string, color: string,
 *   useProxy: boolean, formatStandard?: string, apiKey?: string, keyRequired?: boolean,
 *   local?: boolean, region?: string, blurbKey?: string
 * }>}
 */
const PRESETS = [
  { id: "ollama", name: "Ollama", baseUrl: "http://127.0.0.1:11434/v1", color: "#c4c4c4", useProxy: false, formatStandard: "openai", apiKey: "ollama", keyRequired: false, local: true, region: "local", blurbKey: "preset.blurb.local" },
  { id: "deepseek", name: "DeepSeek", baseUrl: "https://api.deepseek.com/v1", color: "#3fb950", useProxy: true, formatStandard: "openai", keyRequired: true, region: "cn", blurbKey: "preset.blurb.deepseek" },
  { id: "siliconflow", nameKey: "preset.siliconflow", name: "硅基流动", baseUrl: "https://api.siliconflow.cn/v1", color: "#7c5cff", useProxy: true, formatStandard: "openai", keyRequired: true, region: "cn", blurbKey: "preset.blurb.silicon" },
  { id: "openai", name: "OpenAI", baseUrl: "https://api.openai.com/v1", color: "#3d8bfd", useProxy: true, formatStandard: "openai", keyRequired: true, region: "global" },
  { id: "anthropic", name: "Anthropic", baseUrl: "https://api.anthropic.com/v1", color: "#d29922", useProxy: true, formatStandard: "openai", keyRequired: true, region: "global" },
  { id: "qwen", nameKey: "preset.qwen", name: "通义千问", baseUrl: "https://dashscope.aliyuncs.com/compatible-mode/v1", color: "#7c5cff", useProxy: true, formatStandard: "openai", keyRequired: true, region: "cn" },
  { id: "moonshot", name: "Moonshot", baseUrl: "https://api.moonshot.cn/v1", color: "#39c5cf", useProxy: true, formatStandard: "openai", keyRequired: true, region: "cn" },
  { id: "zhipu", nameKey: "preset.zhipu", name: "智谱", baseUrl: "https://open.bigmodel.cn/api/paas/v4", color: "#3859ff", useProxy: true, formatStandard: "openai", keyRequired: true, region: "cn" },
  { id: "minimax", name: "MiniMax", baseUrl: "https://api.minimax.chat/v1", color: "#e85d9a", useProxy: true, formatStandard: "openai", keyRequired: true, region: "cn" },
  { id: "doubao", nameKey: "preset.doubao", name: "豆包/火山", baseUrl: "https://ark.cn-beijing.volces.com/api/v3", color: "#3d8bfd", useProxy: true, formatStandard: "openai", keyRequired: true, region: "cn" },
  { id: "yi", nameKey: "preset.yi", name: "零一万物", baseUrl: "https://api.lingyiwanwu.com/v1", color: "#a371f7", useProxy: true, formatStandard: "openai", keyRequired: true, region: "cn" },
  { id: "groq", name: "Groq", baseUrl: "https://api.groq.com/openai/v1", color: "#f85149", useProxy: true, formatStandard: "openai", keyRequired: true, region: "global" },
  { id: "openrouter", name: "OpenRouter", baseUrl: "https://openrouter.ai/api/v1", color: "#7c5cff", useProxy: true, formatStandard: "openai", keyRequired: true, region: "global" },
  { id: "xai", name: "xAI Grok", baseUrl: "https://api.x.ai/v1", color: "#e6edf3", useProxy: true, formatStandard: "openai", keyRequired: true, region: "global" },
  { id: "together", name: "Together", baseUrl: "https://api.together.xyz/v1", color: "#3fb950", useProxy: true, formatStandard: "openai", keyRequired: true, region: "global" },
  { id: "fireworks", name: "Fireworks", baseUrl: "https://api.fireworks.ai/inference/v1", color: "#d29922", useProxy: true, formatStandard: "openai", keyRequired: true, region: "global" },
  { id: "custom", nameKey: "preset.custom", name: "自定义", baseUrl: "", color: "#8b9cb3", useProxy: true, formatStandard: "openai", keyRequired: true, region: "custom", blurbKey: "preset.blurb.custom" },
];

function presetDisplayName(p) {
  return p.nameKey ? t(p.nameKey) : p.name;
}

function isCustomPreset(p) {
  return p.id === "custom" || p.nameKey === "preset.custom";
}

function presetStableId(p) {
  return p.id || slugifyClient(presetDisplayName(p));
}

function slugifyClient(s) {
  return String(s || "custom")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_|_$/g, "") || "custom";
}

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
/** @type {'providers'|'models'|'apps'|'configs'|'proxy'|'usage'} */
let page = (() => {
  const raw = localStorage.getItem(PAGE_KEY) || "";
  if (raw === "configs") return "apps";
  if (["models", "apps", "proxy", "usage"].includes(raw)) return raw;
  return "providers";
})();

/** @type {any[]} */
let modelGroups = [];
let modelsLoading = false;
let modelsBusy = "";

/** @type {null | { total?: any, byDay?: any[], byModel?: any[], byProvider?: any[], recent?: any[] }} */
let usageStats = null;
let usageBusy = false;
/** @type {Record<string, any>} providerId → package status */
let packageStatusById = {};

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
  revealLabel: "",
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
  el.textContent = tb(msg);
  host.appendChild(el);
  setTimeout(() => el.remove(), 3000);
}

function errMsg(e) {
  return tb(e?.message || String(e));
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

function normalizeTokenPackage(tp) {
  return {
    id: tp.id || tp.ID || uid(),
    name: tp.name || tp.Name || t("pkg.unnamed"),
    totalTokens: Number(tp.totalTokens ?? tp.TotalTokens ?? 0) || 0,
    usedOffset: Number(tp.usedOffset ?? tp.UsedOffset ?? 0) || 0,
    price: Number(tp.price ?? tp.Price ?? 0) || 0,
    currency: tp.currency || tp.Currency || "CNY",
    startAt: tp.startAt || tp.StartAt || "",
    expireAt: tp.expireAt || tp.ExpireAt || "",
    note: tp.note || tp.Note || "",
    active: !!(tp.active ?? tp.Active),
  };
}

function normalizeProvider(p) {
  const name = p.name || "";
  const baseUrl = (p.baseUrl || p.BaseURL || "").replace(/\/$/, "");
  // useProxy: explicit bool, or auto (local → false, cloud → true)
  let useProxy;
  if (typeof p.useProxy === "boolean") useProxy = p.useProxy;
  else if (typeof p.UseProxy === "boolean") useProxy = p.UseProxy;
  else useProxy = !isLocalProviderHint(name, baseUrl);
  const pkgs = (p.tokenPackages || p.TokenPackages || []).map(normalizeTokenPackage);
  // at most one active
  let sawActive = false;
  for (const pkg of pkgs) {
    if (pkg.active) {
      if (sawActive) pkg.active = false;
      else sawActive = true;
    }
  }
  let formatStandard = p.formatStandard || p.FormatStandard || "openai";
  if (formatStandard !== "passthrough") formatStandard = "openai";
  return {
    id: p.id || uid(),
    name,
    baseUrl,
    apiKey: p.apiKey || p.APIKey || "",
    color: p.color || COLORS[0],
    useProxy,
    formatStandard,
    tokenPackages: pkgs,
    models: (p.models || []).map((m) => ({
      id: m.id,
      name: m.name || m.id,
      enabled: m.enabled !== false,
      isDefault: !!m.isDefault,
      ownedBy: m.ownedBy || m.owned_by || "",
    })),
  };
}

function formatTokens(n) {
  n = Number(n) || 0;
  if (usesChineseUnits()) {
    if (n >= 1e8) return (n / 1e8).toFixed(2) + " " + t("unit.yi");
    if (n >= 1e4) return (n / 1e4).toFixed(2) + " " + t("unit.wan");
    return n.toLocaleString(localeBcp47());
  }
  if (n >= 1e9) return (n / 1e9).toFixed(2) + "B";
  if (n >= 1e6) return (n / 1e6).toFixed(2) + "M";
  if (n >= 1e3) return (n / 1e3).toFixed(2) + "K";
  return n.toLocaleString(localeBcp47());
}

function providerPackageStatus(p) {
  const st = packageStatusById[p.id];
  if (st) {
    return {
      hasPackage: !!(st.hasPackage ?? st.HasPackage),
      total: st.totalTokens ?? st.TotalTokens ?? 0,
      used: st.usedTokens ?? st.UsedTokens ?? 0,
      remaining: st.remaining ?? st.Remaining ?? 0,
      percent: st.percentUsed ?? st.PercentUsed ?? 0,
      name: st.packageName ?? st.PackageName ?? "",
      expired: !!(st.expired ?? st.Expired),
      expireAt: st.expireAt ?? st.ExpireAt ?? "",
    };
  }
  // client-side fallback from packages + usageStats
  const pkgs = p.tokenPackages || [];
  const active = pkgs.find((x) => x.active) || pkgs[0];
  if (!active) return { hasPackage: false, total: 0, used: 0, remaining: 0, percent: 0, name: "", expired: false, expireAt: "" };
  const byProv = usageStats?.byProvider || usageStats?.ByProvider || [];
  let proxyUsed = 0;
  for (const b of byProv) {
    const key = (b.key || b.Key || "").toLowerCase();
    if (key === (p.name || "").toLowerCase() || key === (p.id || "").toLowerCase()) {
      proxyUsed = b.totalTokens ?? b.TotalTokens ?? 0;
      break;
    }
  }
  const used = (active.usedOffset || 0) + proxyUsed;
  const total = active.totalTokens || 0;
  const remaining = Math.max(0, total - used);
  const percent = total > 0 ? Math.min(100, (used / total) * 100) : 0;
  return {
    hasPackage: true,
    total,
    used,
    remaining,
    percent,
    name: active.name,
    expired: false,
    expireAt: active.expireAt || "",
  };
}

async function loadPackageStatuses() {
  if (!hasBackend() || typeof GetProviderPackageStatuses !== "function") return;
  try {
    const list = await GetProviderPackageStatuses();
    packageStatusById = {};
    for (const s of list || []) {
      const id = s.providerId || s.ProviderID;
      if (id) packageStatusById[id] = s;
    }
  } catch (_) {}
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
          toast(t("toast.migrated", { n: legacy.length }));
        }
      }
    } catch (e) {
      console.error(e);
      providers = loadLocalLegacy();
      toast(t("toast.loadProvidersFail"), "err");
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
      systemInfo = { os: "windows", platformName: "Windows", revealLabel: "" };
    } else if (/Mac/i.test(ua)) {
      systemInfo = { os: "darwin", platformName: "macOS", revealLabel: "" };
    } else {
      systemInfo = { os: "linux", platformName: "Linux", revealLabel: "" };
    }
    return;
  }
  try {
    systemInfo = (await GetSystemInfo()) || systemInfo;
  } catch (_) {}
  // reveal label is localized in UI via revealLabelForOs(os)
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
    list.push({ id: c.id, name: c.name || c.id, provider: c.provider || "", group: t("configs.groupInFile") });
  }
  for (const p of providers) {
    for (const m of p.models || []) {
      if (!m.enabled || !m.id || seen.has(m.id)) continue;
      seen.add(m.id);
      list.push({ id: m.id, name: m.name || m.id, provider: "", group: p.name });
    }
  }
  if (st?.model && !seen.has(st.model)) {
    list.unshift({ id: st.model, name: st.model, provider: st.modelProvider || "", group: t("configs.groupCurrent") });
  }
  return list;
}

async function fetchModelsFromApi(provider) {
  if (hasBackend()) {
    return FetchProviderModels(provider.baseUrl, provider.apiKey);
  }
  // browser preview fallback
  await new Promise((r) => setTimeout(r, 400));
  throw new Error(t("toast.fetchInWails"));
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
    toast(errMsg(e), "err");
  } finally {
    configsLoading = false;
    if (force) toast(t("toast.rescanned"));
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
    message: t("configs.browserPreview"),
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
      <div class="main-empty">
        <div>
          <div class="empty-icon"><span class="spinner" style="width:22px;height:22px;border-width:3px"></span></div>
          <h3>${t("boot.loading")}</h3>
          <p>${t("boot.loadingDesc")}</p>
        </div>
      </div>`;
    return;
  }

  app.innerHTML = `
    <header class="topbar">
      <div class="topbar-left">
        <div class="brand">
          <div class="brand-mark">AG</div>
          <div>
            <div class="brand-title">AIGateway <span class="brand-sub">${t("brand.sub")}</span></div>
          </div>
        </div>
        <nav class="nav-tabs">
          <button class="nav-tab ${page === "providers" ? "active" : ""}" data-page="providers">${t("nav.providers")}</button>
          <button class="nav-tab ${page === "models" ? "active" : ""}" data-page="models">${t("nav.models")}</button>
          <button class="nav-tab ${page === "apps" || page === "configs" ? "active" : ""}" data-page="apps">${t("nav.apps")}</button>
          <button class="nav-tab ${page === "proxy" ? "active" : ""}" data-page="proxy">${t("nav.proxy")}</button>
          <button class="nav-tab ${page === "usage" ? "active" : ""}" data-page="usage">${t("nav.usage")}</button>
        </nav>
      </div>
      <div class="topbar-meta">
        <div class="lang-picker" id="lang-picker">
          <button type="button" class="lang-picker-btn" id="lang-picker-btn" title="${escapeAttr(t("lang.switch"))}" aria-haspopup="listbox" aria-expanded="false">
            <span class="lang-picker-icon">🌐</span>
            <span class="lang-picker-label">${escapeHtml(getLocaleMeta()?.native || "EN")}</span>
            <span class="lang-picker-caret">▾</span>
          </button>
          <div class="lang-popup" id="lang-popup" role="listbox" hidden>
            ${LOCALES.map(
              (l) => `
              <button type="button" class="lang-option ${getLocale() === l.id ? "active" : ""}" role="option" data-lang="${escapeAttr(l.id)}" aria-selected="${getLocale() === l.id}">
                <span class="lang-option-native">${escapeHtml(l.native)}</span>
              </button>`
            ).join("")}
          </div>
        </div>
        <span class="stat-pill">${escapeHtml(systemInfo.platformName || "")}</span>
        <span class="stat-pill">${t("stat.providers")} <strong>${providers.length}</strong></span>
        <span class="stat-pill">${t("stat.models")} <strong>${totalModels()}</strong></span>
        <span class="stat-pill">${t("stat.enabled")} <strong>${enabledModels()}</strong></span>
        ${
          proxyStatus?.running
            ? `<span class="stat-pill"><span class="status-dot ok"></span> ${t("stat.proxy")}</span>`
            : ""
        }
      </div>
    </header>
    ${
      page === "providers"
        ? renderProvidersPage()
        : page === "models"
          ? renderModelsPage()
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
  else if (page === "models") bindModelsEvents();
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
            <h2>${t("proxy.title")}</h2>
            <p>${t("proxy.desc")}</p>
          </div>
          <div class="actions">
            ${
              running
                ? `<button class="btn btn-danger" id="btn-proxy-stop" ${proxyBusy ? "disabled" : ""}>${t("proxy.stop")}</button>`
                : `<button class="btn btn-primary" id="btn-proxy-start" ${proxyBusy ? "disabled" : ""}>${t("proxy.start")}</button>`
            }
            <button class="btn" id="btn-proxy-refresh" ${proxyBusy ? "disabled" : ""}>${t("proxy.refresh")}</button>
          </div>
        </div>

        <div class="config-grid">
          <section class="config-card">
            <div class="config-card-head">
              <h3>
                <span class="status-dot ${running ? "ok" : "fail"}"></span>
                ${t("proxy.status")}
              </h3>
              <span class="tag ${running ? "ok" : "off"}">${running ? t("proxy.running") : t("proxy.stopped")}</span>
            </div>
            <div class="config-card-body">
              <div class="kv">
                <label>${t("proxy.baseUrlLabel")}</label>
                <div class="input-with-action">
                  <input class="input mono" id="proxy-base-url" readonly value="${escapeAttr(base)}" />
                  <button class="btn btn-sm" id="btn-copy-base">${t("common.copy")}</button>
                </div>
              </div>
              <div class="form-grid" style="margin-top:12px">
                <div class="field">
                  <label>${t("proxy.host")}</label>
                  <input class="input mono" id="proxy-host" value="${escapeAttr(proxyForm.host)}" ${running ? "disabled" : ""} />
                </div>
                <div class="field">
                  <label>${t("proxy.port")}</label>
                  <input class="input mono" id="proxy-port" type="number" min="1" max="65535" value="${escapeAttr(String(proxyForm.port))}" ${running ? "disabled" : ""} />
                </div>
                <div class="field full">
                  <label>${t("proxy.listenKey")}</label>
                  <input class="input mono" id="proxy-listen-key" value="${escapeAttr(proxyForm.listenKey || "")}" placeholder="${escapeAttr(t("proxy.listenKeyPh"))}" />
                </div>
                <div class="field full">
                  <label style="display:flex;align-items:center;gap:8px;cursor:pointer">
                    <input type="checkbox" id="proxy-autostart" ${proxyForm.autoStart ? "checked" : ""} />
                    ${t("proxy.autoStart")}
                  </label>
                </div>
              </div>
              <div class="actions" style="margin-top:14px">
                <button class="btn btn-primary" id="btn-proxy-save" ${proxyBusy ? "disabled" : ""}>${t("proxy.save")}</button>
              </div>
              <div class="hint" style="margin-top:8px">
                ${t("proxy.hintAuto")}
              </div>
              ${
                st.lastError
                  ? `<div class="test-result err" style="margin-top:12px"><div class="test-result-title">${t("common.error")}</div><pre class="preview-box">${escapeHtml(tb(st.lastError))}</pre></div>`
                  : ""
              }
              <div class="hint" style="margin-top:10px">
                ${t("proxy.hintFlow")}
              </div>
            </div>
          </section>

          <section class="config-card">
            <div class="config-card-head">
              <h3>${t("proxy.routing")}</h3>
            </div>
            <div class="config-card-body">
              <div class="hint">${t("proxy.endpoints")}</div>
              <ul class="search-paths">
                <li>GET  {base}/models</li>
                <li>POST {base}/chat/completions (stream)</li>
                <li>POST {base}/completions</li>
                <li>POST {base}/embeddings</li>
              </ul>
              <div class="hint" style="margin-top:12px">${t("proxy.matchModel")}</div>
              <div class="model-table-wrap" style="margin-top:8px;max-height:220px;overflow:auto">
                <table class="model-table">
                  <thead><tr><th>${t("proxy.colProvider")}</th><th>${t("proxy.colModels")}</th></tr></thead>
                  <tbody>
                    ${
                      providers.length
                        ? providers
                            .map((p) => {
                              const ms = (p.models || []).filter((m) => m.enabled).map((m) => m.id);
                              return `<tr>
                                <td>${escapeHtml(p.name)}</td>
                                <td class="model-id">${ms.length ? escapeHtml(ms.join(", ")) : t("common.none")}</td>
                              </tr>`;
                            })
                            .join("")
                        : `<tr><td colspan="2">${t("proxy.noProviders")}</td></tr>`
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
              <h3>${t("proxy.logs")}</h3>
              <p class="desc">${t("proxy.logsDesc")}</p>
            </div>
          </div>
          <div class="panel-body">
            <pre class="preview-box" style="max-height:220px">${
              logs.length ? escapeHtml(logs.join("\n")) : t("proxy.noLogs")
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
          <h2>${t("providers.title")}</h2>
          <button class="btn btn-primary btn-sm" id="btn-add-provider">${t("providers.add")}</button>
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
                <div class="provider-name">${escapeHtml(item.name || t("common.unnamed"))}</div>
                <div class="provider-hint">${escapeHtml(item.baseUrl || t("providers.noApi"))}</div>
              </div>
              <span class="provider-badge" title="${item.useProxy ? t("providers.viaProxy") : t("providers.direct")}">${item.useProxy ? t("providers.badgeProxy") : t("providers.badgeDirect")} ${item.models?.length || 0}${
                (() => {
                  const s = providerPackageStatus(item);
                  if (!s.hasPackage) return "";
                  return ` · ${Math.max(0, 100 - s.percent).toFixed(0)}%`;
                })()
              }</span>
            </button>`
                  )
                  .join("")
              : `<div class="empty-side">${t("providers.emptySide")}</div>`
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
  const cards = ["codex", "claude", "openclaw", "harness"].map((k) => {
    const names = { codex: "ChatGPT", claude: "Claude Code", openclaw: "OpenClaw", harness: "Harness" };
    return toolConfigs[k] || placeholder(k, names[k] || k);
  });
  return `
    <div class="full-page">
      <div class="config-page">
        <div class="config-hero">
          <div>
            <h2>${t("configs.title")}</h2>
            <p>${t("configs.desc", { platform: escapeHtml(systemInfo.platformName || "macOS / Windows") })}</p>
          </div>
          <div class="actions">
            <button class="btn btn-primary" id="btn-rescan" ${configsLoading ? "disabled" : ""}>
              ${configsLoading ? `<span class="spinner"></span> ${t("configs.searching")}` : t("configs.rescan")}
            </button>
          </div>
        </div>
        <div class="apps-grid">
          ${cards.map((st) => renderAppCard(st)).join("")}
        </div>
      </div>
    </div>
  `;
}

function statusTagForTool(st) {
  if (st.found && st.exists) {
    const mp = (st.modelProvider || "").toLowerCase();
    const pathOk = !!(st.path);
    if (mp.includes("aigateway") || mp.includes("codex_proxy")) {
      return `<span class="tag ok">${t("apps.takenOver")}</span>`;
    }
    return `<span class="tag ok">${t("configs.located")}</span>`;
  }
  return `<span class="tag off">${t("configs.notFound")}</span>`;
}

function renderAppCard(st) {
  const kind = st.kind;
  const busy = configsBusy === kind;
  const ok = !!st.found && !!st.exists;
  return `
    <section class="config-card app-card" data-kind="${escapeAttr(kind)}">
      <div class="config-card-head">
        <h3>
          <span class="status-dot ${ok ? "ok" : "fail"}"></span>
          ${escapeHtml(st.name || kind)}
        </h3>
        <div class="meta-row">${statusTagForTool(st)}</div>
      </div>
      <div class="config-card-body">
        <div class="path-box">
          <label style="font-size:12px;color:var(--text-secondary);font-weight:500">${t("configs.pathLabel")}</label>
          <div class="path-value ${ok ? "" : "missing"}">${escapeHtml(st.path || t("common.dash"))}</div>
        </div>
        <div class="hint">${t("apps.takeoverHint")}</div>
        <div class="actions" style="margin-top:10px;flex-wrap:wrap">
          <button class="btn btn-primary" data-act="takeover" data-kind="${kind}" ${busy ? "disabled" : ""}>
            ${busy ? `<span class="spinner"></span>` : t("apps.takeover")}
          </button>
          <button class="btn" data-act="rollback-gw" data-kind="${kind}" ${busy || !st.hasDefaultBackup ? "disabled" : ""}>
            ${t("apps.rollback")}
          </button>
          <button class="btn btn-sm" data-act="scan" data-kind="${kind}" ${busy ? "disabled" : ""}>${t("configs.autoScan")}</button>
          <button class="btn btn-sm" data-act="pick" data-kind="${kind}" ${busy ? "disabled" : ""}>${t("configs.pick")}</button>
          <button class="btn btn-sm" data-act="reveal" data-kind="${kind}" ${!st.path ? "disabled" : ""}>${escapeHtml(revealLabelForOs(systemInfo.os))}</button>
        </div>
        <div class="config-msg ${ok ? "ok" : st.message ? "warn" : ""}">${escapeHtml(tb(st.message || ""))}</div>
        ${
          configPreview[kind]
            ? `<pre class="preview-box" style="max-height:120px">${escapeHtml((configPreview[kind] || "").slice(0, 600))}</pre>`
            : ""
        }
      </div>
    </section>
  `;
}

function routeStatusLabel(status) {
  const s = (status || "ok").toLowerCase();
  if (s === "standby") return t("models.statusStandby");
  if (s === "disabled") return t("models.statusDisabled");
  if (s === "exhausted") return t("models.statusExhausted");
  if (s === "circuit_open") return t("models.statusCircuit");
  return t("models.statusOk");
}

function routeStatusClass(status) {
  const s = (status || "ok").toLowerCase();
  if (s === "standby") return "warn";
  if (s === "disabled") return "off";
  if (s === "exhausted" || s === "circuit_open") return "err";
  return "ok";
}

function renderModelsPage() {
  return `
    <div class="full-page">
      <div class="config-page">
        <div class="config-hero">
          <div>
            <h2>${t("models.title")}</h2>
            <p>${t("models.desc")} ${t("models.dragHint")}</p>
          </div>
          <div class="actions">
            <button class="btn" id="btn-models-refresh" ${modelsLoading ? "disabled" : ""}>
              ${modelsLoading ? `<span class="spinner"></span>` : t("models.refresh")}
            </button>
          </div>
        </div>
        ${
          modelGroups.length
            ? modelGroups
                .map((g) => {
                  const routes = (g.routes || g.Routes || [])
                    .slice()
                    .sort((a, b) => (a.priority ?? a.Priority ?? 0) - (b.priority ?? b.Priority ?? 0));
                  return `
            <section class="panel model-group-panel" data-group="${escapeAttr(g.id)}">
              <div class="panel-head">
                <div>
                  <h3 class="model-id">${escapeHtml(g.name || g.id)}</h3>
                  <p class="desc">${routes.length} ${t("models.channels")}</p>
                </div>
              </div>
              <div class="panel-body">
                <div class="route-list" data-group="${escapeAttr(g.id)}">
                  ${
                    routes.length
                      ? routes
                          .map((r, i) => {
                            const rid = r.id || r.ID;
                            const prio = r.priority ?? r.Priority ?? 0;
                            const used = r.usedTokens ?? r.UsedTokens ?? 0;
                            const status = r.status || r.Status || "ok";
                            const en = !!(r.enabled ?? r.Enabled);
                            return `
                    <div class="route-row ${i === 0 ? "primary" : ""}" draggable="true" data-route="${escapeAttr(rid)}" data-group="${escapeAttr(g.id)}">
                      <span class="drag-handle" title="${escapeAttr(t("models.drag"))}">⠿</span>
                      <div class="route-meta">
                        <div class="provider-name">${escapeHtml(r.providerName || r.ProviderName || r.providerId || "")}</div>
                        <div class="hint mono">${escapeHtml(r.providerModelId || r.ProviderModelID || "")}</div>
                      </div>
                      <span class="tag ${routeStatusClass(status)}">${routeStatusLabel(status)}</span>
                      <span class="model-id prio-badge">#${prio}</span>
                      <span class="model-id used-badge">${Number(used).toLocaleString()}</span>
                      <button class="btn btn-sm" data-act="route-toggle" ${modelsBusy ? "disabled" : ""}>${en ? t("models.disable") : t("models.enable")}</button>
                    </div>`;
                          })
                          .join("")
                      : `<div class="hint">${t("common.dash")}</div>`
                  }
                </div>
              </div>
            </section>`;
                })
                .join("")
            : `<section class="panel"><div class="panel-body"><div class="empty-models"><strong>${t("models.empty")}</strong></div></div></section>`
        }
      </div>
    </div>
  `;
}

async function loadModelGroups() {
  if (!hasBackend() || typeof ListModelGroups !== "function") {
    modelGroups = [];
    return;
  }
  modelsLoading = true;
  try {
    modelGroups = (await ListModelGroups()) || [];
  } catch (e) {
    toast(errMsg(e), "err");
    modelGroups = [];
  } finally {
    modelsLoading = false;
  }
}

function bindModelsEvents() {
  document.getElementById("btn-models-refresh")?.addEventListener("click", async () => {
    await loadModelGroups();
    render();
  });

  // enable / disable
  document.querySelectorAll(".route-row [data-act='route-toggle']").forEach((btn) => {
    btn.addEventListener("click", async (e) => {
      e.stopPropagation();
      const row = btn.closest(".route-row");
      const rid = row?.dataset.route;
      if (!rid) return;
      const gid = row.dataset.group;
      const g = modelGroups.find((x) => x.id === gid);
      const list = g?.routes || g?.Routes || [];
      const cur = list.find((x) => (x.id || x.ID) === rid);
      const en = !!(cur?.enabled ?? cur?.Enabled);
      modelsBusy = rid;
      try {
        await SetModelGroupRouteEnabled(rid, !en);
        await loadModelGroups();
      } catch (err) {
        toast(errMsg(err), "err");
      } finally {
        modelsBusy = "";
        render();
      }
    });
  });

  // drag-and-drop reorder within a group
  document.querySelectorAll(".route-list").forEach((listEl) => {
    let dragEl = null;
    listEl.querySelectorAll(".route-row").forEach((row) => {
      row.addEventListener("dragstart", (e) => {
        dragEl = row;
        row.classList.add("dragging");
        e.dataTransfer.effectAllowed = "move";
        e.dataTransfer.setData("text/plain", row.dataset.route || "");
      });
      row.addEventListener("dragend", () => {
        row.classList.remove("dragging");
        listEl.querySelectorAll(".route-row").forEach((r) => r.classList.remove("drag-over"));
        dragEl = null;
      });
      row.addEventListener("dragover", (e) => {
        e.preventDefault();
        e.dataTransfer.dropEffect = "move";
        if (!dragEl || dragEl === row) return;
        const rect = row.getBoundingClientRect();
        const before = e.clientY < rect.top + rect.height / 2;
        listEl.querySelectorAll(".route-row").forEach((r) => r.classList.remove("drag-over"));
        row.classList.add("drag-over");
        if (before) listEl.insertBefore(dragEl, row);
        else listEl.insertBefore(dragEl, row.nextSibling);
      });
      row.addEventListener("drop", async (e) => {
        e.preventDefault();
        const gid = listEl.dataset.group;
        const ids = [...listEl.querySelectorAll(".route-row")].map((r) => r.dataset.route).filter(Boolean);
        if (!gid || !ids.length || typeof ReorderModelGroupRoutes !== "function") return;
        modelsBusy = "reorder";
        try {
          await ReorderModelGroupRoutes(gid, ids);
          await loadModelGroups();
          toast(t("models.reordered"));
        } catch (err) {
          toast(errMsg(err), "err");
        } finally {
          modelsBusy = "";
          render();
        }
      });
    });
  });
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
    message: configsLoading ? t("configs.searchingMsg") : t("configs.notLoaded"),
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
    const g = c.group || t("common.models");
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
          <span class="tag ${ok ? "ok" : "off"}">${ok ? t("configs.located") : t("configs.notFound")}</span>
          <span class="tag">${escapeHtml(st.source || "auto")}</span>
        </div>
      </div>
      <div class="config-card-body">
        <div class="path-box">
          <label style="font-size:12px;color:var(--text-secondary);font-weight:500">${t("configs.pathLabel")}</label>
          <div class="path-value ${ok ? "" : "missing"}">${escapeHtml(st.path || t("common.dash"))}</div>
        </div>

        <div class="actions">
          <button class="btn btn-sm" data-act="scan" data-kind="${kind}" ${busy ? "disabled" : ""}>${t("configs.autoScan")}</button>
          <button class="btn btn-sm btn-primary" data-act="pick" data-kind="${kind}" ${busy ? "disabled" : ""}>${t("configs.pick")}</button>
          <button class="btn btn-sm" data-act="reveal" data-kind="${kind}" ${!st.path ? "disabled" : ""}>${escapeHtml(
            revealLabelForOs(systemInfo.os)
          )}</button>
          <button class="btn btn-sm btn-ghost" data-act="clear" data-kind="${kind}" ${st.source !== "override" ? "disabled" : ""}>${t("configs.clearPath")}</button>
        </div>

        <div class="actions" style="margin-top:2px">
          <button class="btn btn-sm" data-act="backup-default" data-kind="${kind}" ${busy || !ok ? "disabled" : ""}>
            ${st.hasDefaultBackup ? t("configs.updateBackup") : t("configs.backupDefault")}
          </button>
          <button class="btn btn-sm" data-act="restore-default" data-kind="${kind}" ${
            busy || !st.hasDefaultBackup ? "disabled" : ""
          }>${t("configs.restoreDefault")}</button>
          <button class="btn btn-sm btn-ghost" data-act="clear-backup" data-kind="${kind}" ${
            busy || !st.hasDefaultBackup ? "disabled" : ""
          }>${t("configs.clearBackup")}</button>
          ${st.hasDefaultBackup ? `<span class="tag ok">${t("configs.hasBackup")}</span>` : `<span class="tag off">${t("configs.noBackup")}</span>`}
        </div>
        ${
          st.hasDefaultBackup
            ? `<div class="hint">${t("configs.backupHint", { at: escapeHtml(formatBackupAt(st.defaultBackupAt)) })}</div>`
            : `<div class="hint">${t("configs.autoBackupHint")}</div>`
        }

        <div class="config-msg ${msgClass}">${escapeHtml(tb(st.message || (ok ? t("common.ready") : t("configs.waitSearch"))))}</div>

        <div class="form-grid" style="grid-template-columns:1fr 1fr">
          <div class="kv">
            <label>${t("configs.currentModel")}</label>
            <div class="value">${escapeHtml(st.model || t("configs.unset"))}</div>
          </div>
          <div class="kv">
            <label>${kind === "codex" ? "model_provider" : t("common.note")}</label>
            <div class="value">${escapeHtml(
              kind === "codex"
                ? st.modelProvider || t("common.dash")
                : st.modelProvider
                  ? `BASE ${st.modelProvider}`
                  : t("configs.claudeCompat")
            )}</div>
          </div>
        </div>

        <div class="field full">
          <label>${t("configs.switchModel")}</label>
          <div class="input-with-action">
            <select class="select" data-act="model-select" data-kind="${kind}">
              <option value="">${t("configs.selectModel")}</option>
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
              ${busy ? `<span class="spinner"></span>` : t("common.apply")}
            </button>
          </div>
          <span class="hint">${t("configs.customModelHint")}</span>
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
                  <button class="btn btn-sm btn-primary" data-act="deepseek-claude" ${busy ? "disabled" : ""} title="${escapeAttr(t("configs.deepseekOneClick"))}">
                    ${t("configs.deepseekOneClick")}
                  </button>
                </div>
                <div class="hint">${t("configs.deepseekHint")}</div>`
              : ""
          }
        </div>

        ${
          st.searchPaths?.length
            ? `<div>
                <label style="font-size:12px;color:var(--text-secondary)">${t("configs.searchPaths")}</label>
                <ul class="search-paths">
                  ${(st.searchPaths || []).map((p) => `<li>${escapeHtml(p)}</li>`).join("")}
                </ul>
              </div>`
            : ""
        }

        ${
          configPreview[kind]
            ? `<div>
                <label style="font-size:12px;color:var(--text-secondary)">${t("configs.preview")}</label>
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
        <h3>${t("empty.title")}</h3>
        <p>${t("empty.desc")}</p>
        <button class="btn btn-primary" id="btn-add-provider-empty">${t("empty.add")}</button>
      </div>
    </div>
  `;
}

function renderTokenPackagePanel(p) {
  const pkgs = p.tokenPackages || [];
  const st = providerPackageStatus(p);
  const pct = Math.min(100, Math.max(0, st.percent || 0));
  const barColor = st.expired || pct >= 90 ? "var(--danger)" : pct >= 70 ? "var(--warning)" : "var(--success)";

  return `
    <section class="panel">
      <div class="panel-head">
        <div>
          <h3>${t("pkg.title")}</h3>
          <p class="desc">${t("pkg.desc")}</p>
        </div>
        <div class="actions">
          <button class="btn btn-sm btn-primary" id="btn-pkg-add">${t("pkg.add")}</button>
        </div>
      </div>
      <div class="panel-body">
        ${
          st.hasPackage
            ? `<div class="test-result ${st.expired || pct >= 90 ? "err" : "ok"}" style="margin-bottom:14px">
                <div class="test-result-title">${escapeHtml(st.name || t("pkg.current"))}${st.expired ? " · " + t("pkg.expired") : ""}</div>
                <div class="test-result-grid">
                  <div class="kv"><label>${t("pkg.total")}</label><div class="value">${formatTokens(st.total)}</div></div>
                  <div class="kv"><label>${t("pkg.used")}</label><div class="value">${formatTokens(st.used)}</div></div>
                  <div class="kv"><label>${t("pkg.remaining")}</label><div class="value">${formatTokens(st.remaining)}</div></div>
                  <div class="kv"><label>${t("pkg.usage")}</label><div class="value">${pct.toFixed(1)}%</div></div>
                </div>
                <div style="margin-top:10px;height:8px;background:var(--bg-base);border-radius:999px;overflow:hidden;border:1px solid var(--border)">
                  <div style="height:100%;width:${pct}%;background:${barColor};transition:width .2s"></div>
                </div>
                ${st.expireAt ? `<div class="hint" style="margin-top:8px">${t("pkg.expireAt", { at: escapeHtml(st.expireAt) })}</div>` : ""}
              </div>`
            : `<div class="hint" style="margin-bottom:12px">${t("pkg.noPkgHint")}</div>`
        }
        <div class="model-table-wrap table-scroll">
          ${
            pkgs.length
              ? `<table class="model-table">
            <thead>
              <tr>
                <th>${t("pkg.colName")}</th>
                <th>${t("pkg.colTotal")}</th>
                <th>${t("pkg.colOffset")}</th>
                <th>${t("pkg.colPrice")}</th>
                <th>${t("pkg.colPeriod")}</th>
                <th>${t("pkg.colStatus")}</th>
                <th style="text-align:right">${t("pkg.colActions")}</th>
              </tr>
            </thead>
            <tbody>
              ${pkgs
                .map(
                  (pkg) => `<tr data-pkg="${escapeAttr(pkg.id)}">
                <td>
                  <div class="model-name">${escapeHtml(pkg.name)}</div>
                  ${pkg.note ? `<div class="hint">${escapeHtml(pkg.note)}</div>` : ""}
                </td>
                <td class="model-id">${formatTokens(pkg.totalTokens)}</td>
                <td class="model-id">${formatTokens(pkg.usedOffset)}</td>
                <td>${pkg.price ? `${pkg.price} ${escapeHtml(pkg.currency || "CNY")}` : t("common.dash")}</td>
                <td class="model-id">${escapeHtml([pkg.startAt, pkg.expireAt].filter(Boolean).join(" ~ ") || t("common.dash"))}</td>
                <td>${pkg.active ? `<span class="tag ok">${t("common.current")}</span>` : `<span class="tag off">${t("common.standby")}</span>`}</td>
                <td>
                  <div class="row-actions">
                    <button class="btn btn-sm" data-act="pkg-active" ${pkg.active ? "disabled" : ""}>${t("pkg.setCurrent")}</button>
                    <button class="btn btn-sm" data-act="pkg-edit">${t("common.edit")}</button>
                    <button class="btn btn-sm btn-ghost" data-act="pkg-del">${t("common.delete")}</button>
                  </div>
                </td>
              </tr>`
                )
                .join("")}
            </tbody>
          </table>`
              : `<div class="empty-models"><strong>${t("pkg.empty")}</strong>${t("pkg.emptyHint")}</div>`
          }
        </div>
      </div>
    </section>
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
            <h3>${t("detail.title")}</h3>
            <p class="desc">${t("detail.desc")}</p>
          </div>
          <div class="actions">
            <button class="btn btn-danger btn-sm" id="btn-delete-provider">${t("detail.delete")}</button>
          </div>
        </div>
        <div class="panel-body">
          <div class="form-grid">
            <div class="field">
              <label>${t("detail.name")} <span class="req">*</span></label>
              <input class="input" id="f-name" value="${escapeAttr(p.name)}" placeholder="${escapeAttr(t("detail.namePh"))}" />
            </div>
            <div class="field">
              <label>${t("detail.color")}</label>
              <input class="input" id="f-color" type="color" value="${escapeAttr(p.color || "#3d8bfd")}" style="padding:4px;height:36px" />
            </div>
            <div class="field full">
              <label>${t("detail.base")} <span class="req">*</span></label>
              <input class="input mono" id="f-base" value="${escapeAttr(p.baseUrl)}" placeholder="https://api.openai.com/v1" />
              <span class="hint">${t("detail.baseHint")}</span>
            </div>
            <div class="field full">
              <label>${t("detail.key")} ${isLocalProviderHint(p.name, p.baseUrl) ? "" : `<span class="req">*</span>`}</label>
              <div class="input-with-action">
                <input class="input mono" id="f-key" type="${showKey ? "text" : "password"}"
                  value="${escapeAttr(p.apiKey)}" placeholder="${escapeAttr(isLocalProviderHint(p.name, p.baseUrl) ? t("detail.keyPhLocal") : t("detail.keyPh"))}" autocomplete="off" />
                <button class="btn btn-sm" id="btn-toggle-key" type="button">${showKey ? t("detail.hide") : t("detail.show")}</button>
              </div>
              <span class="hint">${t("detail.keyHint")}</span>
            </div>
            <div class="field full">
              <label>${t("detail.access")}</label>
              <div class="actions" style="margin-top:4px">
                <label class="stat-pill" style="cursor:pointer;gap:8px">
                  <input type="radio" name="f-proxy-mode" id="f-proxy-on" value="1" ${p.useProxy ? "checked" : ""} />
                  ${t("detail.viaLocalProxy")}
                </label>
                <label class="stat-pill" style="cursor:pointer;gap:8px">
                  <input type="radio" name="f-proxy-mode" id="f-proxy-off" value="0" ${!p.useProxy ? "checked" : ""} />
                  ${t("detail.directNoProxy")}
                </label>
              </div>
              <span class="hint">
                ${
                  p.useProxy
                    ? t("detail.proxyHintOn")
                    : t("detail.proxyHintOff")
                }
              </span>
            </div>
            <div class="field full">
              <label>${t("detail.formatStandard")}</label>
              <div class="actions" style="margin-top:4px">
                <label class="stat-pill" style="cursor:pointer;gap:8px">
                  <input type="radio" name="f-format" id="f-format-openai" value="openai" ${(p.formatStandard || "openai") !== "passthrough" ? "checked" : ""} />
                  ${t("detail.formatOpenAI")}
                </label>
                <label class="stat-pill" style="cursor:pointer;gap:8px">
                  <input type="radio" name="f-format" id="f-format-pass" value="passthrough" ${p.formatStandard === "passthrough" ? "checked" : ""} />
                  ${t("detail.formatPassthrough")}
                </label>
              </div>
              <span class="hint">${t("detail.formatHint")}</span>
            </div>
          </div>
          <div class="actions" style="margin-top:16px">
            <button class="btn btn-primary" id="btn-save">${t("detail.save")}</button>
            <button class="btn" id="btn-test" ${testing || fetching ? "disabled" : ""}>
              ${testing ? `<span class="spinner"></span> ${t("detail.testing")}` : t("detail.test")}
            </button>
            <button class="btn" id="btn-fetch" ${fetching || testing ? "disabled" : ""}>
              ${fetching ? `<span class="spinner"></span> ${t("detail.fetching")}` : t("detail.fetch")}
            </button>
          </div>
          ${renderTestResultPanel()}
        </div>
      </section>

      ${renderTokenPackagePanel(p)}

      <section class="panel">
        <div class="panel-head">
          <div>
            <h3>${t("detail.modelsTitle")}</h3>
            <p class="desc">${t("detail.modelsDesc")}</p>
          </div>
          <div class="actions">
            <button class="btn btn-sm" id="btn-refresh-models" ${fetching ? "disabled" : ""}>${t("detail.refreshModels")}</button>
          </div>
        </div>
        <div class="panel-body">
          <div class="models-toolbar">
            <input class="input search" id="f-search" placeholder="${escapeAttr(t("detail.searchPh"))}" value="${escapeAttr(modelQuery)}" />
            <span class="loading-inline">${t("detail.modelCount", { shown: models.length, total: (p.models || []).length })}</span>
          </div>
          <div class="model-table-wrap table-scroll-lg">
            ${
              models.length
                ? `<table class="model-table">
              <thead>
                <tr>
                  <th style="width:44px">${t("detail.colEnable")}</th>
                  <th>${t("detail.colId")}</th>
                  <th>${t("detail.colName")}</th>
                  <th>${t("detail.colStatus")}</th>
                  <th style="width:120px;text-align:right">${t("detail.colActions")}</th>
                </tr>
              </thead>
              <tbody>
                ${models
                  .map(
                    (m) => `
                  <tr data-model="${escapeAttr(m.id)}">
                    <td><button class="toggle ${m.enabled ? "on" : ""}" data-act="toggle" title="${escapeAttr(t("detail.toggleTitle"))}"></button></td>
                    <td><span class="model-id">${escapeHtml(m.id)}</span></td>
                    <td><span class="model-name">${escapeHtml(m.name || m.id)}</span></td>
                    <td>
                      ${m.isDefault ? `<span class="tag default">${t("detail.tagDefault")}</span>` : ""}
                      ${m.enabled ? `<span class="tag ok">${t("detail.tagOn")}</span>` : `<span class="tag off">${t("detail.tagOff")}</span>`}
                    </td>
                    <td>
                      <div class="row-actions">
                        <button class="btn btn-sm btn-ghost" data-act="remove">${t("detail.remove")}</button>
                      </div>
                    </td>
                  </tr>`
                  )
                  .join("")}
              </tbody>
            </table>`
                : `<div class="empty-models">
                  <strong>${(p.models || []).length ? t("detail.noMatch") : t("detail.noModels")}</strong>
                  ${(p.models || []).length ? t("detail.trySearch") : t("detail.fetchHint")}
                </div>`
            }
          </div>
        </div>
      </section>
    </div>
  `;
}

function formatBackupAt(iso) {
  if (!iso) return t("common.dash");
  try {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleString(localeBcp47());
  } catch {
    return iso;
  }
}

function renderTestResultPanel() {
  if (testing) {
    return `
      <div class="test-result loading" style="margin-top:14px">
        <div class="test-result-title"><span class="spinner"></span> ${t("test.running")}</div>
        <div class="hint">${t("test.hint")}</div>
      </div>`;
  }
  if (!testResult) return "";

  const ok = !!testResult.ok;
  const samples = (testResult.sample || []).filter(Boolean);
  return `
    <div class="test-result ${ok ? "ok" : "err"}" style="margin-top:14px">
      <div class="test-result-title">
        <span class="status-dot ${ok ? "ok" : "fail"}"></span>
        ${escapeHtml(tb(testResult.message || (ok ? t("toast.connOk") : t("toast.connFail"))))}
      </div>
      <div class="test-result-grid">
        <div class="kv">
          <label>${t("test.endpoint")}</label>
          <div class="value mono-sm">${escapeHtml(testResult.endpoint || t("common.dash"))}</div>
        </div>
        <div class="kv">
          <label>${t("test.http")}</label>
          <div class="value">${testResult.statusCode ? escapeHtml(String(testResult.statusCode)) : t("common.dash")}</div>
        </div>
        <div class="kv">
          <label>${t("test.latency")}</label>
          <div class="value">${testResult.latencyMs != null ? `${testResult.latencyMs} ms` : t("common.dash")}</div>
        </div>
        <div class="kv">
          <label>${t("test.modelCount")}</label>
          <div class="value">${ok ? escapeHtml(String(testResult.modelCount ?? 0)) : t("common.dash")}</div>
        </div>
      </div>
      ${
        samples.length
          ? `<div class="test-sample">
              <label>${t("test.sample")}</label>
              <div class="sample-tags">
                ${samples.map((s) => `<span class="tag">${escapeHtml(s)}</span>`).join("")}
                ${(testResult.modelCount || 0) > samples.length ? `<span class="tag off">+${(testResult.modelCount || 0) - samples.length} ${t("common.more")}</span>` : ""}
              </div>
            </div>`
          : ""
      }
      ${
        testResult.error
          ? `<div class="test-error"><label>${t("test.errorDetail")}</label><pre class="preview-box">${escapeHtml(tb(testResult.error))}</pre></div>`
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
      if (p === "configs") page = "apps";
      else if (["models", "apps", "proxy", "usage"].includes(p)) page = p;
      else page = "providers";
      localStorage.setItem(PAGE_KEY, page);
      render();
      if ((page === "apps" || page === "configs") && !Object.keys(toolConfigs).length) {
        loadToolConfigs(false);
      }
      if (page === "models") {
        loadModelGroups().then(() => render());
      }
      if (page === "proxy") {
        refreshProxyStatus();
      }
      if (page === "usage") {
        loadUsageStats().then(() => render());
      }
    });
  });
  const picker = document.getElementById("lang-picker");
  const btn = document.getElementById("lang-picker-btn");
  const popup = document.getElementById("lang-popup");
  let outsideBound = false;
  const onDoc = (e) => {
    if (!picker?.contains(e.target)) closePopup();
  };
  const onKey = (e) => {
    if (e.key === "Escape") closePopup();
  };
  const closePopup = () => {
    if (!popup || !btn) return;
    popup.hidden = true;
    btn.setAttribute("aria-expanded", "false");
    if (outsideBound) {
      document.removeEventListener("click", onDoc);
      document.removeEventListener("keydown", onKey);
      outsideBound = false;
    }
  };
  const openPopup = () => {
    if (!popup || !btn) return;
    popup.hidden = false;
    btn.setAttribute("aria-expanded", "true");
    if (!outsideBound) {
      // defer so the opening click doesn't immediately close
      setTimeout(() => {
        document.addEventListener("click", onDoc);
        document.addEventListener("keydown", onKey);
        outsideBound = true;
      }, 0);
    }
  };
  btn?.addEventListener("click", (e) => {
    e.stopPropagation();
    if (popup?.hidden) openPopup();
    else closePopup();
  });
  popup?.querySelectorAll(".lang-option").forEach((el) => {
    el.addEventListener("click", (e) => {
      e.stopPropagation();
      const next = el.dataset.lang;
      closePopup();
      if (!next || next === getLocale()) return;
      setLocale(next);
      render();
    });
  });
}

async function refreshProxyStatus() {
  if (!hasBackend() || typeof window.go?.main?.App?.GetProxyStatus !== "function") {
    proxyStatus = { running: false, lastError: t("toast.runInWailsShort"), logs: [] };
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
      if (!hasBackend()) throw new Error(t("toast.runInWailsShort"));
      const cfg = readProxyForm();
      await SaveProxyConfig(cfg);
      const st = await StartProxy();
      proxyStatus = normalizeProxyStatus(st);
      toast(proxyStatus.running ? t("toast.proxyStarted") : t("toast.proxyStartFail"), proxyStatus.running ? "ok" : "err");
    } catch (e) {
      toast(errMsg(e), "err");
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
      toast(t("toast.proxyStopped"));
    } catch (e) {
      toast(errMsg(e), "err");
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
      toast(t("toast.proxySaved"));
    } catch (e) {
      toast(errMsg(e), "err");
    } finally {
      proxyBusy = false;
      render();
    }
  });

  document.getElementById("btn-copy-base")?.addEventListener("click", async () => {
    const v = document.getElementById("proxy-base-url")?.value || "";
    try {
      await navigator.clipboard.writeText(v);
      toast(t("toast.copiedBase"));
    } catch {
      toast(t("toast.copyFail"), "err");
    }
  });
}

function renderUsagePage() {
  const tot = usageStats?.total || usageStats?.Total || {};
  const calls = tot.calls ?? tot.Calls ?? 0;
  const input = tot.inputTokens ?? tot.InputTokens ?? 0;
  const output = tot.outputTokens ?? tot.OutputTokens ?? 0;
  const totalTok = tot.totalTokens ?? tot.TotalTokens ?? 0;
  const byModel = usageStats?.byModel || usageStats?.ByModel || [];
  const byProvider = usageStats?.byProvider || usageStats?.ByProvider || [];
  const byDay = usageStats?.byDay || usageStats?.ByDay || [];
  const recent = usageStats?.recent || usageStats?.Recent || [];

  const rowBucket = (b) => {
    const key = b.key ?? b.Key ?? "";
    const c = b.calls ?? b.Calls ?? 0;
    const i = b.inputTokens ?? b.InputTokens ?? 0;
    const o = b.outputTokens ?? b.OutputTokens ?? 0;
    const sum = b.totalTokens ?? b.TotalTokens ?? 0;
    return `<tr>
      <td class="model-id">${escapeHtml(key)}</td>
      <td>${c}</td>
      <td>${i.toLocaleString()}</td>
      <td>${o.toLocaleString()}</td>
      <td><strong>${sum.toLocaleString()}</strong></td>
    </tr>`;
  };

  return `
    <div class="full-page">
      <div class="config-page">
        <div class="config-hero">
          <div>
            <h2>${t("usage.title")}</h2>
            <p>${t("usage.desc")}</p>
          </div>
          <div class="actions">
            <button class="btn" id="btn-usage-refresh" ${usageBusy ? "disabled" : ""}>${t("usage.refresh")}</button>
            <button class="btn btn-ghost" id="btn-usage-clear" ${usageBusy ? "disabled" : ""}>${t("usage.clear")}</button>
          </div>
        </div>

        <div class="config-grid" style="grid-template-columns:repeat(4,1fr)">
          <section class="config-card" style="min-height:auto">
            <div class="config-card-body">
              <div class="kv"><label>${t("usage.calls")}</label><div class="value" style="font-size:22px;font-weight:700">${calls}</div></div>
            </div>
          </section>
          <section class="config-card" style="min-height:auto">
            <div class="config-card-body">
              <div class="kv"><label>${t("usage.input")}</label><div class="value" style="font-size:22px;font-weight:700">${Number(input).toLocaleString()}</div></div>
            </div>
          </section>
          <section class="config-card" style="min-height:auto">
            <div class="config-card-body">
              <div class="kv"><label>${t("usage.output")}</label><div class="value" style="font-size:22px;font-weight:700">${Number(output).toLocaleString()}</div></div>
            </div>
          </section>
          <section class="config-card" style="min-height:auto">
            <div class="config-card-body">
              <div class="kv"><label>${t("usage.total")}</label><div class="value" style="font-size:22px;font-weight:700;color:var(--accent)">${Number(totalTok).toLocaleString()}</div></div>
            </div>
          </section>
        </div>

        <div class="config-grid">
          <section class="config-card">
            <div class="config-card-head"><h3>${t("usage.byModel")}</h3></div>
            <div class="config-card-body">
              <div class="model-table-wrap table-scroll">
                <table class="model-table">
                  <thead><tr><th>${t("usage.colModel")}</th><th>${t("usage.colCalls")}</th><th>${t("usage.colInput")}</th><th>${t("usage.colOutput")}</th><th>${t("usage.colTotal")}</th></tr></thead>
                  <tbody>${byModel.length ? byModel.map(rowBucket).join("") : `<tr><td colspan="5">${t("usage.noData")}</td></tr>`}</tbody>
                </table>
              </div>
            </div>
          </section>
          <section class="config-card">
            <div class="config-card-head"><h3>${t("usage.byProvider")}</h3></div>
            <div class="config-card-body">
              <div class="model-table-wrap table-scroll">
                <table class="model-table">
                  <thead><tr><th>${t("usage.colProvider")}</th><th>${t("usage.colCalls")}</th><th>${t("usage.colInput")}</th><th>${t("usage.colOutput")}</th><th>${t("usage.colTotal")}</th></tr></thead>
                  <tbody>${byProvider.length ? byProvider.map(rowBucket).join("") : `<tr><td colspan="5">${t("usage.noData")}</td></tr>`}</tbody>
                </table>
              </div>
            </div>
          </section>
        </div>

        <section class="panel" style="margin-top:4px">
          <div class="panel-head"><div><h3>${t("usage.byDay")}</h3></div></div>
          <div class="panel-body">
            <div class="model-table-wrap table-scroll-lg">
              <table class="model-table">
                <thead><tr><th>${t("usage.colDate")}</th><th>${t("usage.colCalls")}</th><th>${t("usage.colInput")}</th><th>${t("usage.colOutput")}</th><th>${t("usage.colTotal")}</th></tr></thead>
                <tbody>${byDay.length ? byDay.map(rowBucket).join("") : `<tr><td colspan="5">${t("usage.noData")}</td></tr>`}</tbody>
              </table>
            </div>
          </div>
        </section>

        <section class="panel" style="margin-top:4px">
          <div class="panel-head"><div><h3>${t("usage.recent")}</h3><p class="desc">${t("usage.recentDesc")}</p></div></div>
          <div class="panel-body">
            <div class="model-table-wrap table-scroll-lg">
              <table class="model-table">
                <thead><tr><th>${t("usage.colTime")}</th><th>${t("usage.colProvider")}</th><th>${t("usage.colModel")}</th><th>${t("usage.colEndpoint")}</th><th>${t("usage.colInput")}</th><th>${t("usage.colOutput")}</th><th>${t("usage.colTotal")}</th></tr></thead>
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
                      : `<tr><td colspan="7">${t("usage.noRecent")}</td></tr>`
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
    toast(errMsg(e), "err");
  } finally {
    usageBusy = false;
  }
}

function bindUsageEvents() {
  document.getElementById("btn-usage-refresh")?.addEventListener("click", async () => {
    await loadUsageStats();
    render();
    toast(t("toast.usageRefreshed"));
  });
  document.getElementById("btn-usage-clear")?.addEventListener("click", async () => {
    if (!confirm(t("confirm.clearUsage"))) return;
    try {
      if (!hasBackend()) throw new Error(t("toast.runInWailsShort"));
      usageStats = await ClearUsageStats();
      toast(t("toast.usageCleared"));
      render();
    } catch (e) {
      toast(errMsg(e), "err");
    }
  });
}

function bindConfigEvents() {
  document.getElementById("btn-rescan")?.addEventListener("click", () => loadToolConfigs(true));

  document.querySelectorAll("[data-act='takeover']").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const kind = btn.dataset.kind;
      configsBusy = kind;
      render();
      try {
        if (!hasBackend()) throw new Error(t("toast.runInWailsShort"));
        const st = await InjectGateway(kind);
        await applyToolStatus(st);
        toast(st.message || t("apps.takenOver"));
      } catch (e) {
        toast(errMsg(e), "err");
      } finally {
        configsBusy = "";
        render();
      }
    });
  });

  document.querySelectorAll("[data-act='rollback-gw']").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const kind = btn.dataset.kind;
      const name = toolConfigs[kind]?.name || kind;
      if (!confirm(t("confirm.restoreDefault", { name }))) return;
      configsBusy = kind;
      render();
      try {
        if (!hasBackend()) throw new Error(t("toast.runInWailsShort"));
        const st = await RollbackGateway(kind);
        await applyToolStatus(st);
        toast(st.message || t("toast.restored"));
      } catch (e) {
        toast(errMsg(e), "err");
      } finally {
        configsBusy = "";
        render();
      }
    });
  });

  document.querySelectorAll("[data-act='scan']").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const kind = btn.dataset.kind;
      configsBusy = kind;
      render();
      try {
        if (!hasBackend()) throw new Error(t("toast.runInWailsShort"));
        const st = await ClearToolConfigPath(kind);
        await applyToolStatus(st);
        if (!st.found) toast(t("toast.scanFail", { name: st.name }), "err");
        else toast(t("toast.located", { name: st.name }));
      } catch (e) {
        toast(errMsg(e), "err");
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
        if (!hasBackend()) throw new Error(t("toast.runInWailsShort"));
        const st = await PickToolConfig(kind);
        await applyToolStatus(st);
        if (st.found) toast(t("toast.picked", { path: st.path }));
      } catch (e) {
        toast(errMsg(e), "err");
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
        toast(t("toast.clearedPath"));
        render();
      } catch (e) {
        toast(errMsg(e), "err");
      }
    });
  });

  document.querySelectorAll("[data-act='reveal']").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const kind = btn.dataset.kind;
      try {
        await RevealConfigPath(toolConfigs[kind]?.path || "");
      } catch (e) {
        toast(errMsg(e), "err");
      }
    });
  });

  document.querySelectorAll("[data-act='backup-default']").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const kind = btn.dataset.kind;
      configsBusy = kind;
      render();
      try {
        if (!hasBackend()) throw new Error(t("toast.runInWailsShort"));
        const st = await BackupDefaultConfig(kind, toolConfigs[kind]?.path || "");
        await applyToolStatus(st);
        toast(tb(st.message) || t("toast.backedUp"));
      } catch (e) {
        toast(errMsg(e), "err");
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
        if (!hasBackend()) throw new Error(t("toast.runInWailsShort"));
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
        toast(tb(st.message) || t("toast.deepseekMigrated"));
      } catch (e) {
        toast(errMsg(e), "err");
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
      if (!confirm(t("confirm.restoreDefault", { name }))) return;
      configsBusy = kind;
      render();
      try {
        if (!hasBackend()) throw new Error(t("toast.runInWailsShort"));
        const st = await RestoreDefaultConfig(kind);
        await applyToolStatus(st);
        toast(tb(st.message) || t("toast.restored"));
      } catch (e) {
        toast(errMsg(e), "err");
      } finally {
        configsBusy = "";
        render();
      }
    });
  });

  document.querySelectorAll("[data-act='clear-backup']").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const kind = btn.dataset.kind;
      if (!confirm(t("confirm.clearBackup"))) return;
      configsBusy = kind;
      render();
      try {
        if (!hasBackend()) throw new Error(t("toast.runInWailsShort"));
        const st = await ClearDefaultBackup(kind);
        await applyToolStatus(st);
        toast(tb(st.message) || t("toast.backupCleared"));
      } catch (e) {
        toast(errMsg(e), "err");
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
      if (!model) return toast(t("toast.selectModel"), "err");
      let usePath = toolConfigs[kind]?.path || "";
      configsBusy = kind;
      render();
      try {
        if (!hasBackend()) throw new Error(t("toast.runInWailsShort"));
        if (!toolConfigs[kind]?.exists) {
          toast(t("toast.configMissing"), "err");
          const st = await PickToolConfig(kind);
          await applyToolStatus(st);
          if (!st.path) throw new Error(t("toast.noPath"));
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
        toast(tb(st.message) || t("toast.switched", { model }));
      } catch (e) {
        toast(errMsg(e), "err");
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
  const fmtPass = document.getElementById("f-format-pass");
  if (fmtPass) p.formatStandard = fmtPass.checked ? "passthrough" : "openai";
}

async function applyModelToTool(kind, modelId) {
  if (!hasBackend()) throw new Error(t("toast.runInWailsShort"));
  // ensure configs loaded
  if (!toolConfigs[kind]) {
    await loadToolConfigs(false);
  }
  let st = toolConfigs[kind];
  if (!st?.exists) {
    toast(t("toast.toolConfigMissing", { tool: kind === "codex" ? "Codex" : "Claude" }), "err");
    page = "configs";
    localStorage.setItem(PAGE_KEY, page);
    render();
    await loadToolConfigs(false);
    st = await PickToolConfig(kind);
    await applyToolStatus(st);
    if (!st?.path) throw new Error(t("toast.noConfigPath"));
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
    if (!p.name.trim()) return toast(t("toast.needName"), "err");
    if (!p.baseUrl.trim()) return toast(t("toast.needBase"), "err");
    try {
      await persistProviders();
      // SaveProviders backend already EnsureProxyRouting; refresh proxy status for UI
      if (p.useProxy && hasBackend() && typeof EnsureProxyRouting === "function") {
        try {
          const st = await EnsureProxyRouting();
          proxyStatus = normalizeProxyStatus(st);
        } catch (_) {}
      }
      await loadPackageStatuses();
      toast(p.useProxy ? t("toast.savedProxy") : t("toast.savedDirect"));
      render();
    } catch (e) {
      toast(errMsg(e), "err");
    }
  });

  document.getElementById("btn-pkg-add")?.addEventListener("click", () => openPackageModal(p, null));

  document.querySelectorAll("[data-pkg]").forEach((row) => {
    const pkgId = row.dataset.pkg;
    row.querySelector('[data-act="pkg-active"]')?.addEventListener("click", async () => {
      (p.tokenPackages || []).forEach((x) => (x.active = x.id === pkgId));
      try {
        await persistProviders();
        await loadPackageStatuses();
        toast(t("toast.pkgActive"));
        render();
      } catch (e) {
        toast(errMsg(e), "err");
      }
    });
    row.querySelector('[data-act="pkg-edit"]')?.addEventListener("click", () => {
      const pkg = (p.tokenPackages || []).find((x) => x.id === pkgId);
      if (pkg) openPackageModal(p, pkg);
    });
    row.querySelector('[data-act="pkg-del"]')?.addEventListener("click", async () => {
      if (!confirm(t("confirm.deletePkg"))) return;
      p.tokenPackages = (p.tokenPackages || []).filter((x) => x.id !== pkgId);
      try {
        await persistProviders();
        await loadPackageStatuses();
        toast(t("toast.pkgDeleted"));
        render();
      } catch (e) {
        toast(errMsg(e), "err");
      }
    });
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
        message: t("toast.testFail"),
        error: local ? t("toast.needBaseOnly") : t("toast.needBaseAndKey"),
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
          message: t("toast.cannotTest"),
          error: t("toast.runInWails"),
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
      toast(tb(testResult.message) || (testResult.ok ? t("toast.connOk") : t("toast.connFail")), testResult.ok ? "ok" : "err");
    } catch (e) {
      testResult = {
        ok: false,
        message: t("toast.testException"),
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
    if (!p.name.trim() || !p.baseUrl.trim()) return toast(t("toast.needNameAndApi"), "err");
    if (!p.apiKey.trim()) return toast(t("toast.needKey"), "err");
    fetching = true;
    render();
    try {
      const list = await fetchModelsFromApi(p);
      mergeFetchedModels(p, list);
      await persistProviders();
      toast(t("toast.fetchedModels", { n: list.length }));
    } catch (e) {
      toast(errMsg(e), "err");
    } finally {
      fetching = false;
      render();
    }
  };

  document.getElementById("btn-fetch")?.addEventListener("click", doFetch);
  document.getElementById("btn-refresh-models")?.addEventListener("click", doFetch);

  document.getElementById("btn-delete-provider")?.addEventListener("click", async () => {
    if (!confirm(t("confirm.deleteProvider", { name: p.name }))) return;
    providers = providers.filter((x) => x.id !== p.id);
    selectedId = providers[0]?.id ?? null;
    try {
      await persistProviders();
      toast(t("toast.deletedProvider"));
      render();
    } catch (e) {
      toast(errMsg(e), "err");
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
        toast(errMsg(e), "err");
      }
    });
    row.querySelector('[data-act="default"]')?.addEventListener("click", async () => {
      p.models.forEach((x) => (x.isDefault = x.id === mid));
      const m = p.models.find((x) => x.id === mid);
      if (m) m.enabled = true;
      try {
        await persistProviders();
        toast(t("toast.defaultModel", { id: mid }));
        render();
      } catch (e) {
        toast(errMsg(e), "err");
      }
    });
    row.querySelector('[data-act="remove"]')?.addEventListener("click", async () => {
      p.models = p.models.filter((x) => x.id !== mid);
      try {
        await persistProviders();
        render();
      } catch (e) {
        toast(errMsg(e), "err");
      }
    });
    row.querySelector('[data-act="to-codex"]')?.addEventListener("click", async () => {
      try {
        const st = await applyModelToTool("codex", mid);
        toast(tb(st.message) || t("toast.wroteCodex", { id: mid }));
      } catch (e) {
        toast(errMsg(e), "err");
      }
    });
    row.querySelector('[data-act="to-claude"]')?.addEventListener("click", async () => {
      try {
        const st = await applyModelToTool("claude", mid);
        toast(tb(st.message) || t("toast.wroteClaude", { id: mid }));
      } catch (e) {
        toast(errMsg(e), "err");
      }
    });
  });
}

function parseTokenAmount(raw) {
  // support 1000000, 100万, 1亿, 1M, 1B, 1e6
  const s = String(raw || "").trim().replace(/,/g, "");
  if (!s) return 0;
  if (/万$/.test(s)) return Math.round(parseFloat(s) * 10000) || 0;
  if (/亿$/.test(s)) return Math.round(parseFloat(s) * 1e8) || 0;
  if (/[bB]$/.test(s)) return Math.round(parseFloat(s) * 1e9) || 0;
  if (/[mM]$/.test(s)) return Math.round(parseFloat(s) * 1e6) || 0;
  if (/[kK]$/.test(s)) return Math.round(parseFloat(s) * 1e3) || 0;
  const n = Number(s);
  return Number.isFinite(n) ? Math.round(n) : 0;
}

function openPackageModal(provider, existing) {
  const root = document.getElementById("modal-root");
  const isEdit = !!existing;
  const pkg = existing || {
    id: uid(),
    name: "",
    totalTokens: 1000000,
    usedOffset: 0,
    price: 0,
    currency: "CNY",
    startAt: new Date().toISOString().slice(0, 10),
    expireAt: "",
    note: "",
    active: !(provider.tokenPackages || []).some((x) => x.active),
  };
  root.innerHTML = `
    <div class="modal-backdrop" id="modal-backdrop">
      <div class="modal" role="dialog" aria-modal="true">
        <div class="modal-head">
          <h3>${isEdit ? t("modal.pkgEdit") : t("modal.pkgAdd")}</h3>
          <button class="btn btn-sm btn-ghost" id="modal-close">${t("common.close")}</button>
        </div>
        <div class="modal-body">
          <div class="field">
            <label>${t("modal.pkgName")} <span class="req">*</span></label>
            <input class="input" id="pkg-name" value="${escapeAttr(pkg.name)}" placeholder="${escapeAttr(t("modal.pkgNamePh"))}" />
          </div>
          <div class="field">
            <label>${t("modal.pkgTotal")} <span class="req">*</span></label>
            <input class="input mono" id="pkg-total" value="${escapeAttr(String(pkg.totalTokens || ""))}" placeholder="${escapeAttr(t("modal.pkgTotalPh"))}" />
            <span class="hint">${t("modal.pkgTotalHint")}</span>
          </div>
          <div class="field">
            <label>${t("modal.pkgOffset")}</label>
            <input class="input mono" id="pkg-offset" value="${escapeAttr(String(pkg.usedOffset || 0))}" placeholder="0" />
          </div>
          <div class="form-grid">
            <div class="field">
              <label>${t("modal.pkgPrice")}</label>
              <input class="input mono" id="pkg-price" type="number" step="0.01" value="${escapeAttr(String(pkg.price || 0))}" />
            </div>
            <div class="field">
              <label>${t("modal.pkgCurrency")}</label>
              <select class="select" id="pkg-currency">
                ${["CNY", "USD", "USDT"].map((c) => `<option value="${c}" ${pkg.currency === c ? "selected" : ""}>${c}</option>`).join("")}
              </select>
            </div>
            <div class="field">
              <label>${t("modal.pkgStart")}</label>
              <input class="input" id="pkg-start" type="date" value="${escapeAttr(pkg.startAt || "")}" />
            </div>
            <div class="field">
              <label>${t("modal.pkgExpire")}</label>
              <input class="input" id="pkg-expire" type="date" value="${escapeAttr(pkg.expireAt || "")}" />
            </div>
          </div>
          <div class="field">
            <label>${t("modal.pkgNote")}</label>
            <input class="input" id="pkg-note" value="${escapeAttr(pkg.note || "")}" placeholder="${escapeAttr(t("modal.pkgNotePh"))}" />
          </div>
          <label class="stat-pill" style="cursor:pointer;gap:8px;display:inline-flex;margin-top:4px">
            <input type="checkbox" id="pkg-active" ${pkg.active ? "checked" : ""} />
            ${t("modal.pkgSetActive")}
          </label>
        </div>
        <div class="modal-foot">
          <button class="btn" id="modal-cancel">${t("common.cancel")}</button>
          <button class="btn btn-primary" id="modal-ok">${t("common.save")}</button>
        </div>
      </div>
    </div>
  `;
  const close = () => {
    root.innerHTML = "";
  };
  document.getElementById("modal-close")?.addEventListener("click", close);
  document.getElementById("modal-cancel")?.addEventListener("click", close);
  document.getElementById("modal-backdrop")?.addEventListener("click", (e) => {
    if (e.target.id === "modal-backdrop") close();
  });
  document.getElementById("modal-ok")?.addEventListener("click", async () => {
    const name = document.getElementById("pkg-name").value.trim();
    const totalTokens = parseTokenAmount(document.getElementById("pkg-total").value);
    const usedOffset = parseTokenAmount(document.getElementById("pkg-offset").value);
    const price = Number(document.getElementById("pkg-price").value) || 0;
    const currency = document.getElementById("pkg-currency").value || "CNY";
    const startAt = document.getElementById("pkg-start").value || "";
    const expireAt = document.getElementById("pkg-expire").value || "";
    const note = document.getElementById("pkg-note").value.trim();
    const active = !!document.getElementById("pkg-active").checked;
    if (!name) return toast(t("toast.needPkgName"), "err");
    if (totalTokens <= 0) return toast(t("toast.needPkgTotal"), "err");
    if (!provider.tokenPackages) provider.tokenPackages = [];
    const next = {
      id: pkg.id || uid(),
      name,
      totalTokens,
      usedOffset,
      price,
      currency,
      startAt,
      expireAt,
      note,
      active,
    };
    if (active) {
      provider.tokenPackages.forEach((x) => (x.active = false));
    }
    const idx = provider.tokenPackages.findIndex((x) => x.id === next.id);
    if (idx >= 0) provider.tokenPackages[idx] = next;
    else provider.tokenPackages.push(next);
    // if no active at all, make this active
    if (!provider.tokenPackages.some((x) => x.active)) {
      next.active = true;
    }
    try {
      await persistProviders();
      await loadPackageStatuses();
      close();
      toast(isEdit ? t("toast.pkgUpdated") : t("toast.pkgAdded"));
      render();
    } catch (e) {
      toast(errMsg(e), "err");
    }
  });
}

function openAddModal() {
  const root = document.getElementById("modal-root");
  let presetIdx = 1; // default DeepSeek (index 1 after Ollama)
  if (!PRESETS[presetIdx]) presetIdx = 0;

  const renderPresetGrid = () => {
    const regions = [
      { id: "local", label: t("modal.region.local") },
      { id: "cn", label: t("modal.region.cn") },
      { id: "global", label: t("modal.region.global") },
      { id: "custom", label: t("modal.region.custom") },
    ];
    return regions
      .map((reg) => {
        const items = PRESETS.map((p, i) => ({ p, i })).filter(({ p }) => (p.region || "global") === reg.id);
        if (!items.length) return "";
        return `
          <div class="preset-region">
            <div class="preset-region-label">${escapeHtml(reg.label)}</div>
            <div class="preset-grid">
              ${items
                .map(
                  ({ p, i }) => `
                <button type="button" class="preset-chip ${i === presetIdx ? "active" : ""}" data-preset="${i}" style="--chip:${p.color}">
                  <span class="preset-chip-dot" style="background:${p.color}"></span>
                  <span class="preset-chip-name">${escapeHtml(presetDisplayName(p))}</span>
                </button>`
                )
                .join("")}
            </div>
          </div>`;
      })
      .join("");
  };

  const applyPresetToForm = () => {
    const preset = PRESETS[presetIdx] || PRESETS[0];
    const custom = isCustomPreset(preset);
    const nameEl = document.getElementById("m-name");
    const baseEl = document.getElementById("m-base");
    const keyEl = document.getElementById("m-key");
    const proxyEl = document.getElementById("m-proxy");
    const fmtOpen = document.getElementById("m-format-openai");
    const fmtPass = document.getElementById("m-format-pass");
    const blurb = document.getElementById("m-preset-blurb");
    if (nameEl) nameEl.value = custom ? "" : presetDisplayName(preset);
    if (baseEl) {
      baseEl.value = preset.baseUrl || "";
      baseEl.readOnly = !custom;
    }
    if (keyEl) {
      keyEl.value = preset.apiKey || "";
      keyEl.placeholder = preset.local ? t("detail.keyPhLocal") : t("modal.keyOnlyPh");
    }
    if (proxyEl) proxyEl.value = preset.useProxy === false ? "0" : "1";
    if (fmtOpen && fmtPass) {
      const pass = preset.formatStandard === "passthrough";
      fmtOpen.checked = !pass;
      fmtPass.checked = pass;
    }
    if (blurb) {
      blurb.textContent = preset.blurbKey ? t(preset.blurbKey) : t("modal.presetHint");
    }
    document.querySelectorAll(".preset-chip").forEach((btn) => {
      btn.classList.toggle("active", Number(btn.dataset.preset) === presetIdx);
    });
    const adv = document.getElementById("m-advanced");
    if (adv && custom) adv.open = true;
  };

  root.innerHTML = `
    <div class="modal-backdrop" id="modal-backdrop">
      <div class="modal modal-lg" role="dialog" aria-modal="true">
        <div class="modal-head">
          <h3>${t("modal.addProvider")}</h3>
          <button class="btn btn-sm btn-ghost" id="modal-close">${t("common.close")}</button>
        </div>
        <div class="modal-body">
          <div class="field">
            <label>${t("modal.presetPick")}</label>
            <p class="hint" id="m-preset-blurb">${t("modal.presetHint")}</p>
            <div class="preset-library">${renderPresetGrid()}</div>
          </div>
          <div class="field">
            <label>${t("modal.keyOnly")} <span class="req" id="m-key-req">*</span></label>
            <input class="input mono" id="m-key" type="password" placeholder="${escapeAttr(t("modal.keyOnlyPh"))}" autocomplete="off" />
          </div>
          <details class="advanced-block" id="m-advanced">
            <summary>${t("modal.advanced")}</summary>
            <div class="field" style="margin-top:10px">
              <label>${t("detail.name")} <span class="req">*</span></label>
              <input class="input" id="m-name" />
            </div>
            <div class="field">
              <label>${t("detail.base")} <span class="req">*</span></label>
              <input class="input mono" id="m-base" />
            </div>
            <div class="field">
              <label>${t("modal.access")}</label>
              <select class="select" id="m-proxy">
                <option value="0">${t("detail.directNoProxy")}</option>
                <option value="1" selected>${t("detail.viaLocalProxy")}</option>
              </select>
            </div>
            <div class="field">
              <label>${t("detail.formatStandard")}</label>
              <div class="actions" style="margin-top:4px">
                <label class="stat-pill" style="cursor:pointer;gap:8px">
                  <input type="radio" name="m-format" id="m-format-openai" value="openai" checked />
                  ${t("detail.formatOpenAI")}
                </label>
                <label class="stat-pill" style="cursor:pointer;gap:8px">
                  <input type="radio" name="m-format" id="m-format-pass" value="passthrough" />
                  ${t("detail.formatPassthrough")}
                </label>
              </div>
            </div>
          </details>
        </div>
        <div class="modal-foot">
          <button class="btn" id="modal-cancel">${t("common.cancel")}</button>
          <button class="btn btn-primary" id="modal-ok">${t("common.add")}</button>
        </div>
      </div>
    </div>
  `;

  const close = () => {
    root.innerHTML = "";
  };

  applyPresetToForm();

  document.querySelectorAll(".preset-chip").forEach((btn) => {
    btn.addEventListener("click", () => {
      presetIdx = Number(btn.dataset.preset);
      applyPresetToForm();
    });
  });

  document.getElementById("modal-close")?.addEventListener("click", close);
  document.getElementById("modal-cancel")?.addEventListener("click", close);
  document.getElementById("modal-backdrop")?.addEventListener("click", (e) => {
    if (e.target.id === "modal-backdrop") close();
  });

  document.getElementById("modal-ok")?.addEventListener("click", async () => {
    const preset = PRESETS[presetIdx] || PRESETS[0];
    const name = document.getElementById("m-name").value.trim() || presetDisplayName(preset);
    const baseUrl = document.getElementById("m-base").value.trim().replace(/\/$/, "");
    let apiKey = document.getElementById("m-key").value.trim();
    const color = preset.color || COLORS[providers.length % COLORS.length];
    const useProxy = document.getElementById("m-proxy")?.value === "1";
    const formatStandard = document.getElementById("m-format-pass")?.checked ? "passthrough" : "openai";
    const stableId = presetStableId(preset);

    if (preset.local && !apiKey) apiKey = preset.apiKey || "ollama";
    if (!preset.local && preset.keyRequired !== false && !apiKey) {
      return toast(t("toast.needKeyCloud"), "err");
    }
    if (!name) return toast(t("toast.needName"), "err");
    if (!baseUrl) return toast(t("toast.needBase"), "err");

    // avoid duplicate stable ids for known presets
    let id = isCustomPreset(preset) ? uid() : stableId;
    if (!isCustomPreset(preset) && providers.some((x) => x.id === id)) {
      return toast(t("toast.presetExists"), "err");
    }
    if (name.toLowerCase() === "ollama") id = "ollama";

    const item = {
      id,
      name,
      baseUrl,
      apiKey,
      color,
      useProxy,
      formatStandard,
      models: [],
      tokenPackages: [],
    };
    providers.push(item);
    selectedId = item.id;
    showKey = false;
    modelQuery = "";
    try {
      await persistProviders();
      close();
      toast(t("toast.addedProvider", { name }));
      render();
    } catch (e) {
      providers = providers.filter((x) => x.id !== item.id);
      toast(errMsg(e), "err");
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
  await loadUsageStats();
  await loadPackageStatuses();
  booting = false;
  render();
  if (page === "apps" || page === "configs") {
    loadToolConfigs(false);
  }
  if (page === "models") {
    loadModelGroups().then(() => render());
  }
  if (page === "proxy") {
    // already refreshed
  }
  if (page === "usage") {
    // already loaded
  }
})();
