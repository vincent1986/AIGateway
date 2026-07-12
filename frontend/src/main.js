import "./style.css";
import "./app.css";

const go = () => window.go?.main?.App;
const hasBackend = () => typeof go()?.ListProviders === "function";

let PRESETS = [
  { id: "custom", name: "自定义", baseUrl: "https://", color: "#8b9cb3", apiFormat: "auto", models: [] },
];

let providers = [];
let selectedId = null;
let page = localStorage.getItem("codex.ui.page") || "providers";
let showKey = false;
let fetching = false;
let testing = false;
let testResult = null;
let booting = true;
let proxyStatus = null;
let proxyForm = { host: "127.0.0.1", port: 18080, autoStart: false, listenKey: "" };
let proxyBusy = false;

const uid = () => crypto.randomUUID?.() || `id_${Date.now()}`;
const toast = (msg, type = "ok") => {
  const host = document.getElementById("toast-host");
  if (!host) return;
  const el = document.createElement("div");
  el.className = `toast ${type}`;
  el.textContent = msg;
  host.appendChild(el);
  setTimeout(() => el.remove(), 2800);
};
const esc = (s) => String(s ?? "").replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;");
const attr = (s) => esc(s).replaceAll("'", "&#39;");
const selected = () => providers.find((p) => p.id === selectedId) || null;
const initials = (n) => (n || "?").slice(0, 2).toUpperCase();

async function loadProviders() {
  if (hasBackend()) {
    try {
      const list = await go().ListProviders();
      providers = (list || []).map(normalize);
    } catch (e) {
      toast(String(e), "err");
      providers = [];
    }
  } else {
    providers = [];
  }
  if (!providers.some((p) => p.id === selectedId)) selectedId = providers[0]?.id || null;
}

async function loadPresets() {
  if (!hasBackend() || typeof go().ListProviderPresets !== "function") return;
  try {
    const list = await go().ListProviderPresets();
    PRESETS = [...(list || []), { id: "custom", name: "自定义", baseUrl: "https://", color: "#8b9cb3", apiFormat: "auto", models: [] }];
  } catch (e) {
    toast(String(e), "err");
  }
}

function normalize(p) {
  return {
    id: p.id || uid(),
    name: p.name || "",
    baseUrl: (p.baseUrl || "").replace(/\/$/, ""),
    apiKey: p.apiKey || "",
    color: p.color || "#3d8bfd",
    apiFormat: normalizeFormat(p.apiFormat || "auto"),
    models: (p.models || []).map((m) => ({
      id: m.id, name: m.name || m.id, enabled: m.enabled !== false, isDefault: !!m.isDefault, ownedBy: m.ownedBy || "",
    })),
  };
}

function normalizeFormat(f) {
  if (f === "openai") return "openai_chat";
  if (f === "anthropic") return "anthropic_messages";
  return f || "auto";
}

async function persist() {
  if (hasBackend()) await go().SaveProviders(providers);
}

async function refreshProxy() {
  if (!hasBackend() || !go().GetProxyStatus) {
    proxyStatus = { running: false, logs: [] };
    return;
  }
  try {
    const st = await go().GetProxyStatus();
    proxyStatus = {
      running: !!(st.running ?? st.Running),
      baseUrl: st.baseUrl || st.BaseURL || "",
      host: st.host || st.Host || "127.0.0.1",
      port: st.port || st.Port || 18080,
      autoStart: !!(st.autoStart ?? st.AutoStart),
      listenKey: st.listenKey || st.ListenKey || "",
      lastError: st.lastError || st.LastError || "",
      logs: st.logs || st.Logs || [],
    };
    const cfg = await go().GetProxyConfig();
    if (cfg) {
      proxyForm = {
        host: cfg.host || cfg.Host || "127.0.0.1",
        port: cfg.port || cfg.Port || 18080,
        autoStart: !!(cfg.autoStart ?? cfg.AutoStart),
        listenKey: cfg.listenKey || cfg.ListenKey || "",
      };
    }
  } catch (e) {
    proxyStatus = { running: false, lastError: String(e), logs: [] };
  }
}

function render() {
  const app = document.getElementById("app");
  if (booting) {
    app.innerHTML = `<div class="main-empty"><div><span class="spinner"></span><h3>加载中…</h3></div></div>`;
    return;
  }
  app.innerHTML = `
    <header class="topbar">
      <div class="topbar-left">
        <div class="brand-title">Codex <span class="brand-sub">模型管理</span></div>
        <nav class="nav-tabs">
          <button class="nav-tab ${page === "providers" ? "active" : ""}" data-page="providers">厂家模型</button>
          <button class="nav-tab ${page === "proxy" ? "active" : ""}" data-page="proxy">代理服务</button>
        </nav>
      </div>
      <div>
        <span class="stat-pill">厂家 <strong>${providers.length}</strong></span>
        ${proxyStatus?.running ? `<span class="stat-pill"><span class="status-dot ok"></span> 代理</span>` : ""}
      </div>
    </header>
    ${page === "proxy" ? renderProxy() : renderProviders()}
    <div class="toast-host" id="toast-host"></div>
    <div id="modal-root"></div>
  `;
  document.querySelectorAll(".nav-tab").forEach((el) => {
    el.onclick = () => {
      page = el.dataset.page;
      localStorage.setItem("codex.ui.page", page);
      render();
      if (page === "proxy") refreshProxy().then(render);
    };
  });
  if (page === "proxy") bindProxy();
  else bindProviders();
}

function renderProviders() {
  const p = selected();
  return `
    <div class="layout">
      <aside class="sidebar">
        <div class="sidebar-head"><h2>厂家</h2><button class="btn btn-primary btn-sm" id="btn-add">+ 添加</button></div>
        <div class="provider-list">
          ${providers.length ? providers.map((item) => `
            <button class="provider-item ${item.id === selectedId ? "active" : ""}" data-id="${item.id}">
              <div class="provider-avatar" style="background:${item.color}">${initials(item.name)}</div>
              <div>
                <div class="provider-name">${esc(item.name)}</div>
                <div class="provider-hint">${esc(item.baseUrl || "未配置")}</div>
              </div>
            </button>`).join("") : `<div class="hint" style="padding:16px">还没有厂家</div>`}
        </div>
      </aside>
      <main class="main">
        ${p ? renderDetail(p) : `<div class="main-empty"><div><h3>选择或添加厂家</h3><button class="btn btn-primary" id="btn-add2">添加厂家</button></div></div>`}
      </main>
    </div>`;
}

function formatLabel(f) {
  f = normalizeFormat(f);
  if (f === "anthropic_messages") return "Anthropic Messages（自动转 OpenAI）";
  if (f === "openai_chat") return "OpenAI Chat Completions";
  if (f === "openai_responses") return "OpenAI Responses 原生";
  return "自动检测";
}

function renderDetail(p) {
  const models = p.models || [];
  return `
    <div class="main-scroll">
      <section class="panel">
        <div class="panel-head">
          <h3>厂家配置</h3>
          <button class="btn btn-danger btn-sm" id="btn-del">删除</button>
        </div>
        <div class="panel-body">
          <div class="form-grid">
            <div class="field"><label>名称</label><input class="input" id="f-name" value="${attr(p.name)}" /></div>
            <div class="field"><label>主题色</label><input class="input" id="f-color" type="color" value="${attr(p.color || "#3d8bfd")}" /></div>
            <div class="field full"><label>API Base URL</label><input class="input mono" id="f-base" value="${attr(p.baseUrl)}" /></div>
            <div class="field full">
              <label>上游 API 格式</label>
              <select class="select" id="f-format">
                <option value="auto" ${p.apiFormat === "auto" ? "selected" : ""}>自动检测</option>
                <option value="openai_chat" ${p.apiFormat === "openai_chat" ? "selected" : ""}>OpenAI Chat Completions</option>
                <option value="openai_responses" ${p.apiFormat === "openai_responses" ? "selected" : ""}>OpenAI Responses 原生</option>
                <option value="anthropic_messages" ${p.apiFormat === "anthropic_messages" ? "selected" : ""}>Anthropic Messages（代理自动转换）</option>
              </select>
              <span class="hint">客户端始终用标准 OpenAI；若上游不是 OpenAI，代理会转换后再访问</span>
            </div>
            <div class="field full">
              <label>API Key</label>
              <div class="input-with-action">
                <input class="input mono" id="f-key" type="${showKey ? "text" : "password"}" value="${attr(p.apiKey)}" />
                <button class="btn btn-sm" id="btn-key">${showKey ? "隐藏" : "显示"}</button>
              </div>
            </div>
          </div>
          <div class="actions">
            <button class="btn btn-primary" id="btn-save">保存</button>
            <button class="btn" id="btn-test" ${testing ? "disabled" : ""}>${testing ? "测试中…" : "测试连接"}</button>
            <button class="btn" id="btn-fetch" ${fetching ? "disabled" : ""}>${fetching ? "获取中…" : "获取模型"}</button>
          </div>
          ${testResult ? `<div class="test-result ${testResult.ok ? "ok" : "err"}">
            <strong>${esc(testResult.message)}</strong>
            ${testResult.apiFormat ? `<div class="hint">格式：${esc(formatLabel(testResult.apiFormat))}</div>` : ""}
            ${testResult.endpoint ? `<div class="hint mono">${esc(testResult.endpoint)}</div>` : ""}
            ${testResult.error ? `<pre class="preview-box">${esc(testResult.error)}</pre>` : ""}
            ${testResult.sample?.length ? `<div class="hint">示例：${esc(testResult.sample.join(", "))}</div>` : ""}
          </div>` : ""}
        </div>
      </section>
      <section class="panel">
        <div class="panel-head"><h3>模型列表 (${models.filter(m=>m.enabled).length}/${models.length})</h3></div>
        <div class="panel-body">
          ${models.length ? `<table class="model-table"><thead><tr><th>启用</th><th>ID</th><th>名称</th></tr></thead><tbody>
            ${models.map((m) => `<tr data-id="${attr(m.id)}">
              <td><input type="checkbox" data-act="en" ${m.enabled ? "checked" : ""}/></td>
              <td class="model-id">${esc(m.id)}</td>
              <td>${esc(m.name)}</td>
            </tr>`).join("")}
          </tbody></table>` : `<div class="hint">暂无模型，点击「获取模型」</div>`}
        </div>
      </section>
    </div>`;
}

function renderProxy() {
  const st = proxyStatus || {};
  const running = !!st.running;
  const base = st.baseUrl || `http://${proxyForm.host}:${proxyForm.port}/v1`;
  const logs = (st.logs || []).slice().reverse();
  const usage = st.usage || st.Usage || {};
  const requests = (st.requests || st.Requests || []).slice().reverse().slice(0, 12);
  return `
    <div class="full-page">
      <div class="config-page">
        <div style="display:flex;justify-content:space-between;align-items:flex-start;gap:12px">
          <div>
            <h2 style="margin:0 0 6px">OpenAI 兼容代理</h2>
            <p class="hint" style="margin:0;max-width:560px">客户端始终用标准 OpenAI 协议访问本地代理。若厂家是 Anthropic 等非标准格式，代理会<strong>自动转换为标准 OpenAI 出入</strong>，再按上游格式访问。</p>
          </div>
          <div class="actions">
            ${running
              ? `<button class="btn btn-danger" id="px-stop">停止</button>`
              : `<button class="btn btn-primary" id="px-start">启动</button>`}
            <button class="btn" id="px-refresh">刷新</button>
          </div>
        </div>
        <div class="config-grid">
          <section class="config-card">
            <div class="config-card-head">
              <h3><span class="status-dot ${running ? "ok" : "fail"}"></span> 状态</h3>
              <span class="tag ${running ? "ok" : "off"}">${running ? "运行中" : "已停止"}</span>
            </div>
            <div class="config-card-body">
              <div class="field">
                <label>OpenAI Base URL</label>
                <div class="input-with-action">
                  <input class="input mono" id="px-base" readonly value="${attr(base)}" />
                  <button class="btn btn-sm" id="px-copy">复制</button>
                </div>
              </div>
              <div class="form-grid">
                <div class="field"><label>Host</label><input class="input mono" id="px-host" value="${attr(proxyForm.host)}" ${running ? "disabled" : ""}/></div>
                <div class="field"><label>Port</label><input class="input mono" id="px-port" type="number" value="${attr(String(proxyForm.port))}" ${running ? "disabled" : ""}/></div>
                <div class="field full"><label>接入密钥（可选）</label><input class="input mono" id="px-key" value="${attr(proxyForm.listenKey)}" /></div>
                <div class="field full"><label><input type="checkbox" id="px-auto" ${proxyForm.autoStart ? "checked" : ""}/> 启动应用时自动开代理</label></div>
              </div>
              <div class="actions">
                <button class="btn btn-primary" id="px-save">保存</button>
                <button class="btn" id="px-codex" ${!running ? "disabled" : ""}>Codex 地址→本地代理</button>
                <button class="btn btn-ghost" id="px-restore">恢复原始 base_url</button>
              </div>
              <div class="hint">转换：Anthropic Messages ⇄ OpenAI Chat；Responses ⇄ Chat（非 OpenAI 上游）</div>
              ${st.lastError ? `<pre class="preview-box">${esc(st.lastError)}</pre>` : ""}
            </div>
          </section>
          <section class="config-card">
            <div class="config-card-head"><h3>厂家格式</h3></div>
            <div class="config-card-body">
              <table class="model-table"><thead><tr><th>厂家</th><th>格式</th><th>模型</th></tr></thead><tbody>
                ${providers.map((p) => `<tr>
                  <td>${esc(p.name)}</td>
                  <td>${esc(formatLabel(p.apiFormat || "auto"))}</td>
                  <td class="model-id">${esc((p.models || []).filter((m) => m.enabled).map((m) => m.id).join(", ") || "—")}</td>
                </tr>`).join("") || `<tr><td colspan="3">暂无厂家</td></tr>`}
              </tbody></table>
            </div>
          </section>
        </div>
        <section class="panel">
          <div class="panel-head"><h3>健康与用量</h3></div>
          <div class="panel-body">
            <div class="metric-row">
              <span>请求 <strong>${Number(usage.totalRequests || usage.TotalRequests || 0)}</strong></span>
              <span>错误 <strong>${Number(usage.errorRequests || usage.ErrorRequests || 0)}</strong></span>
              <span>平均延迟 <strong>${Math.round(Number(usage.avgLatencyMs || usage.AvgLatencyMs || 0))} ms</strong></span>
              <span>输入 Token <strong>${Number(usage.promptTokens || usage.PromptTokens || 0)}</strong></span>
              <span>输出 Token <strong>${Number(usage.completionTokens || usage.CompletionTokens || 0)}</strong></span>
              <span>总 Token <strong>${Number(usage.totalTokens || usage.TotalTokens || 0)}</strong></span>
            </div>
            <div class="actions" style="margin-bottom:12px"><button class="btn btn-sm btn-ghost" id="px-clear-usage">清空 token 统计</button></div>
            <table class="model-table"><thead><tr><th>时间</th><th>状态</th><th>模型</th><th>厂家</th><th>路径</th></tr></thead><tbody>
              ${requests.length ? requests.map((x) => `<tr>
                <td class="model-id">${esc((x.at || x.At || "").replace("T", " ").slice(0, 19))}</td>
                <td>${esc(x.status || x.Status || "")}</td>
                <td class="model-id">${esc(x.model || x.Model || "")}</td>
                <td>${esc(x.provider || x.Provider || "")}</td>
                <td class="model-id">${esc(x.path || x.Path || "")}</td>
              </tr>`).join("") : `<tr><td colspan="5">暂无请求</td></tr>`}
            </tbody></table>
          </div>
        </section>
        <section class="panel">
          <div class="panel-head"><h3>日志</h3></div>
          <div class="panel-body"><pre class="preview-box" style="max-height:180px">${logs.length ? esc(logs.join("\n")) : "（暂无）"}</pre></div>
        </section>
      </div>
    </div>`;
}

function readForm(p) {
  p.name = document.getElementById("f-name")?.value || p.name;
  p.baseUrl = (document.getElementById("f-base")?.value || "").replace(/\/$/, "");
  p.apiKey = document.getElementById("f-key")?.value || "";
  p.color = document.getElementById("f-color")?.value || p.color;
  p.apiFormat = document.getElementById("f-format")?.value || "auto";
}

function bindProviders() {
  document.getElementById("btn-add")?.addEventListener("click", openAdd);
  document.getElementById("btn-add2")?.addEventListener("click", openAdd);
  document.querySelectorAll(".provider-item").forEach((el) => {
    el.onclick = () => { selectedId = el.dataset.id; showKey = false; testResult = null; render(); };
  });
  const p = selected();
  if (!p) return;
  document.getElementById("btn-key")?.addEventListener("click", () => { readForm(p); showKey = !showKey; render(); });
  document.getElementById("btn-save")?.addEventListener("click", async () => {
    readForm(p);
    try { await persist(); toast("已保存"); render(); } catch (e) { toast(String(e), "err"); }
  });
  document.getElementById("btn-del")?.addEventListener("click", async () => {
    if (!confirm(`删除 ${p.name}？`)) return;
    try {
      if (hasBackend() && typeof go().DeleteProvider === "function") {
        providers = (await go().DeleteProvider(p.id) || []).map(normalize);
      } else {
        providers = providers.filter((x) => x.id !== p.id);
        await persist();
      }
      selectedId = providers[0]?.id || null;
      toast("已删除厂家");
      render();
    } catch (e) {
      toast(String(e), "err");
      await loadProviders();
      render();
    }
  });
  document.getElementById("btn-test")?.addEventListener("click", async () => {
    readForm(p);
    testing = true; testResult = null; render();
    try {
      if (!hasBackend()) throw new Error("请用 wails dev 运行");
	      const res = await go().TestProviderConnection(p.baseUrl, p.apiKey, p.apiFormat);
      testResult = {
        ok: !!(res.ok ?? res.OK),
        message: res.message || res.Message || "",
        endpoint: res.endpoint || res.Endpoint || "",
        error: res.error || res.Error || "",
        sample: res.sample || res.Sample || [],
        apiFormat: res.apiFormat || res.APIFormat || p.apiFormat,
      };
      toast(testResult.message, testResult.ok ? "ok" : "err");
    } catch (e) {
      testResult = { ok: false, message: "测试失败", error: String(e) };
      toast(String(e), "err");
    } finally { testing = false; render(); }
  });
  document.getElementById("btn-fetch")?.addEventListener("click", async () => {
    readForm(p);
    fetching = true; render();
    try {
      if (!hasBackend()) throw new Error("请用 wails dev 运行");
      if (p.apiFormat === "anthropic_messages" || (p.apiFormat === "auto" && /anthropic\.com/i.test(p.baseUrl) && !/compatible/i.test(p.baseUrl))) {
        toast("Anthropic 原生无 /models 列表，请手动添加模型 ID", "err");
      } else {
        const list = await go().FetchProviderModels(p.baseUrl, p.apiKey);
        p.models = (list || []).map((m, i) => ({
          id: m.id || m.ID, name: m.name || m.Name || m.id,
          enabled: true, isDefault: i === 0, ownedBy: m.ownedBy || m.OwnedBy || "",
        }));
        await persist();
        toast(`已获取 ${p.models.length} 个模型`);
      }
    } catch (e) { toast(String(e), "err"); }
    finally { fetching = false; render(); }
  });
  document.querySelectorAll("tr[data-id]").forEach((row) => {
    row.querySelector("[data-act=en]")?.addEventListener("change", async (e) => {
      const m = p.models.find((x) => x.id === row.dataset.id);
      if (m) { m.enabled = e.target.checked; await persist(); }
    });
  });
}

function bindProxy() {
  const read = () => ({
    enabled: !!proxyStatus?.running,
    host: document.getElementById("px-host")?.value || "127.0.0.1",
    port: Number(document.getElementById("px-port")?.value || 18080),
    autoStart: !!document.getElementById("px-auto")?.checked,
    listenKey: document.getElementById("px-key")?.value || "",
  });
  document.getElementById("px-refresh")?.addEventListener("click", async () => { await refreshProxy(); render(); });
  document.getElementById("px-clear-usage")?.addEventListener("click", async () => {
    if (!confirm("清空 token 统计和最近请求记录？")) return;
    try {
      proxyStatus = await go().ClearProxyUsageStats();
      await refreshProxy();
      toast("已清空 token 统计");
      render();
    } catch (e) { toast(String(e), "err"); }
  });
  document.getElementById("px-start")?.addEventListener("click", async () => {
    try {
      await go().SaveProxyConfig(read());
      proxyStatus = await go().StartProxy();
      toast(proxyStatus.running || proxyStatus.Running ? "代理已启动" : "启动失败", (proxyStatus.running || proxyStatus.Running) ? "ok" : "err");
      await refreshProxy(); render();
    } catch (e) { toast(String(e), "err"); }
  });
  document.getElementById("px-stop")?.addEventListener("click", async () => {
    try { await go().StopProxy(); await refreshProxy(); toast("已停止"); render(); } catch (e) { toast(String(e), "err"); }
  });
  document.getElementById("px-save")?.addEventListener("click", async () => {
    try { await go().SaveProxyConfig(read()); toast("已保存"); await refreshProxy(); render(); } catch (e) { toast(String(e), "err"); }
  });
  document.getElementById("px-copy")?.addEventListener("click", async () => {
    try { await navigator.clipboard.writeText(document.getElementById("px-base")?.value || ""); toast("已复制"); } catch { toast("复制失败", "err"); }
  });
  document.getElementById("px-codex")?.addEventListener("click", async () => {
    try {
      const st = await go().ApplyProxyToCodex("", "");
      toast(st.message || st.Message || "已改写 Codex base_url");
    } catch (e) { toast(String(e), "err"); }
  });
  document.getElementById("px-restore")?.addEventListener("click", async () => {
    try {
      const st = await go().RestoreCodexOriginalBases();
      toast(st.message || st.Message || "已恢复");
    } catch (e) { toast(String(e), "err"); }
  });
}

function openAdd() {
  const root = document.getElementById("modal-root");
  root.innerHTML = `
    <div class="modal-backdrop" id="bd">
      <div class="modal">
        <div class="modal-head"><h3>添加厂家</h3><button class="btn btn-sm btn-ghost" id="mc">关闭</button></div>
        <div class="modal-body">
          <div class="field"><label>预设</label>
            <select class="select" id="m-preset">${PRESETS.map((x, i) => `<option value="${i}">${esc(x.name)}</option>`).join("")}</select>
          </div>
          <div class="field"><label>名称</label><input class="input" id="m-name" value="${attr(PRESETS[0].name)}"/></div>
          <div class="field"><label>Base URL</label><input class="input mono" id="m-base" value="${attr(PRESETS[0].baseUrl)}"/></div>
          <div class="field"><label>API 格式</label>
            <select class="select" id="m-fmt">
              <option value="auto">自动检测</option>
              <option value="openai_chat" selected>OpenAI Chat Completions</option>
              <option value="openai_responses">OpenAI Responses 原生</option>
              <option value="anthropic_messages">Anthropic Messages</option>
            </select>
          </div>
          <div class="field"><label>API Key</label><input class="input mono" id="m-key" type="password"/></div>
        </div>
        <div class="modal-foot"><button class="btn" id="mx">取消</button><button class="btn btn-primary" id="mok">添加</button></div>
      </div>
    </div>`;
  const close = () => { root.innerHTML = ""; };
  document.getElementById("m-preset").onchange = (e) => {
    const pr = PRESETS[Number(e.target.value)];
    document.getElementById("m-name").value = pr.name === "自定义" ? "" : pr.name;
    document.getElementById("m-base").value = pr.baseUrl === "https://" ? "" : pr.baseUrl;
    document.getElementById("m-fmt").value = pr.apiFormat || "auto";
  };
  document.getElementById("mc").onclick = close;
  document.getElementById("mx").onclick = close;
  document.getElementById("mok").onclick = async () => {
    const name = document.getElementById("m-name").value.trim();
    const baseUrl = document.getElementById("m-base").value.trim().replace(/\/$/, "");
    const apiKey = document.getElementById("m-key").value.trim();
    const apiFormat = document.getElementById("m-fmt").value;
    const pr = PRESETS[Number(document.getElementById("m-preset").value)];
    if (!name || !baseUrl) return toast("请填写名称与 URL", "err");
    const models = (pr.models || []).map((m, i) => ({
      id: m.id || m.ID,
      name: m.name || m.Name || m.id || m.ID,
      enabled: m.enabled !== false,
      isDefault: !!(m.isDefault ?? m.IsDefault ?? i === 0),
      ownedBy: m.ownedBy || m.OwnedBy || "",
    })).filter((m) => m.id);
    const item = { id: pr.id === "custom" ? uid() : `${pr.id}_${uid().slice(0, 8)}`, name, baseUrl, apiKey, color: pr.color, apiFormat, models };
    providers.push(item);
    selectedId = item.id;
    await persist();
    close();
    toast("已添加");
    render();
  };
}

(async () => {
  booting = true;
  render();
  await loadPresets();
  await loadProviders();
  await refreshProxy();
  booting = false;
  render();
})();
