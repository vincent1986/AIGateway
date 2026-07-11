/** AIGateway i18n — popup language picker (EN / 繁中 / 日本語). */

import en from "./locales/en.json";
import zhTW from "./locales/zh-TW.json";
import ja from "./locales/ja.json";

export const LOCALE_KEY = "codex.ui.locale";

/** @typedef {{ id: string, label: string, native: string, htmlLang: string, bcp47: string }} LocaleMeta */

/** Supported languages in the popup (order = display order). Default: English. */
export const LOCALES = /** @type {LocaleMeta[]} */ ([
  { id: "en", label: "English", native: "English", htmlLang: "en", bcp47: "en-US" },
  { id: "zh-TW", label: "繁體中文", native: "繁體中文", htmlLang: "zh-TW", bcp47: "zh-TW" },
  { id: "ja", label: "日本語", native: "日本語", htmlLang: "ja", bcp47: "ja-JP" },
]);

export const SUPPORTED = LOCALES.map((l) => l.id);

const dict = {
  en,
  "zh-TW": zhTW,
  ja,
};

/** @type {string} */
let locale = detectLocale();

function detectLocale() {
  try {
    const saved = localStorage.getItem(LOCALE_KEY);
    if (saved && SUPPORTED.includes(saved)) return saved;
    // migrate removed locales to closest supported
    if (saved === "zh" || saved === "zh-CN") return "zh-TW";
    if (saved === "ko" || saved === "de" || saved === "vi" || saved === "th") return "en";
  } catch (_) {}
  // Default: English. Auto-detect only for 繁中 / 日文 system locales.
  const nav = (typeof navigator !== "undefined" ? navigator.language || "" : "").toLowerCase();
  if (nav.startsWith("zh-tw") || nav.startsWith("zh-hk") || nav.startsWith("zh-mo") || nav === "zh-hant") {
    return "zh-TW";
  }
  if (nav.startsWith("ja")) return "ja";
  // zh-CN and everything else → English
  return "en";
}

export function getLocale() {
  return locale;
}

export function getLocaleMeta(id = locale) {
  return LOCALES.find((l) => l.id === id) || LOCALES.find((l) => l.id === "en");
}

/** @param {string} next */
export function setLocale(next) {
  if (!SUPPORTED.includes(next)) return locale;
  locale = next;
  try {
    localStorage.setItem(LOCALE_KEY, locale);
  } catch (_) {}
  applyDocumentLang();
  return locale;
}

export function applyDocumentLang() {
  if (typeof document === "undefined") return;
  const meta = getLocaleMeta();
  document.documentElement.lang = meta?.htmlLang || "en";
}

/**
 * @param {string} key
 * @param {Record<string, string|number>=} vars
 */
export function t(key, vars) {
  const table = dict[locale] || dict.en;
  let s = table[key] ?? dict.en[key] ?? key;
  if (vars) {
    for (const [k, v] of Object.entries(vars)) {
      s = s.replaceAll(`{${k}}`, String(v));
    }
  }
  return s;
}

/** Localized “reveal in file manager” label for OS. */
export function revealLabelForOs(os) {
  if (os === "windows") return t("reveal.windows");
  if (os === "linux") return t("reveal.linux");
  return t("reveal.darwin");
}

/** BCP 47 tag for Number/Date formatting. */
export function localeBcp47() {
  return getLocaleMeta()?.bcp47 || "en-US";
}

/** Whether locale uses 万/億 style token formatting. */
export function usesChineseUnits() {
  return locale === "zh-TW";
}

/**
 * Best-effort translate of backend (Go) Chinese messages when UI is not 繁中.
 * @param {string} msg
 */
export function tb(msg) {
  if (!msg) return msg;
  if (locale === "zh-TW") {
    return String(msg)
      .replaceAll("厂家", "廠家")
      .replaceAll("配置", "設定")
      .replaceAll("失败", "失敗")
      .replaceAll("请", "請")
      .replaceAll("连接", "連線")
      .replaceAll("文件", "檔案")
      .replaceAll("选择", "選擇")
      .replaceAll("备份", "備份")
      .replaceAll("还原", "還原")
      .replaceAll("启动", "啟動")
      .replaceAll("停止", "停止");
  }
  if (locale === "ja") {
    // fall through to English mapping for common Go messages
  }
  const m = String(msg);

  const exact = {
    "厂家名称不能为空": "Provider name is required",
    "API Base URL 不能为空": "API Base URL is required",
    "请先填写 API Base URL": "Enter API Base URL first",
    "请先填写 API Key": "Enter API Key first",
    "接口未返回模型列表": "API did not return a model list",
    "测试失败": "Test failed",
    "连接失败": "Connection failed",
    "连接成功但无模型": "Connected but no models",
    "接口返回错误": "API returned an error",
    "响应无法解析": "Response could not be parsed",
    "路径为空": "Path is empty",
    "已备份为默认配置": "Saved as default backup",
    "请在 Wails 应用中运行": "Run inside the Wails app",
    "请在 Wails 应用中运行以真实获取模型": "Run in the Wails app to fetch models",
  };
  if (exact[m]) return exact[m];

  let out = m;
  out = out.replace(/^模型已切换为\s*(.+)$/, "Switched model to $1");
  out = out.replace(/^连接成功，发现\s*(\d+)\s*个模型$/, "Connected; found $1 model(s)");
  out = out.replace(/^API Base URL 无效:\s*(.+)$/, "Invalid API Base URL: $1");
  out = out.replace(/^请求失败:\s*(.+)$/, "Request failed: $1");
  out = out.replace(/^读取响应失败:\s*(.+)$/, "Failed to read response: $1");
  out = out.replace(/^配置文件不存在:\s*(.+)$/, "Config file not found: $1");
  out = out.replace(/^未知工具类型:\s*(.+)$/, "Unknown tool type: $1");
  out = out.replace(/^读取备份失败:\s*(.+)$/, "Failed to read backup: $1");
  out = out.replace(
    /^没有可还原的默认备份，请先备份或修改一次以自动生成$/,
    "No default backup to restore; back up first or edit once to auto-create one"
  );
  out = out.replace(/^解析 providers\.json 失败:\s*(.+)$/, "Failed to parse providers.json: $1");
  out = out.replace(/^无法解析模型列表响应$/, "Could not parse model list response");
  return out;
}

applyDocumentLang();
