/** AIGateway i18n — multi-locale with popup language picker. */

import zhCN from "./locales/zh-CN.json";
import zhTW from "./locales/zh-TW.json";
import en from "./locales/en.json";
import ja from "./locales/ja.json";
import ko from "./locales/ko.json";
import de from "./locales/de.json";
import vi from "./locales/vi.json";
import th from "./locales/th.json";

export const LOCALE_KEY = "codex.ui.locale";

/** @typedef {{ id: string, label: string, native: string, htmlLang: string, bcp47: string }} LocaleMeta */

/** Supported languages for the language popup (order = display order). */
export const LOCALES = /** @type {LocaleMeta[]} */ ([
  { id: "zh", label: "简体中文", native: "简体中文", htmlLang: "zh-CN", bcp47: "zh-CN" },
  { id: "zh-TW", label: "繁體中文", native: "繁體中文", htmlLang: "zh-TW", bcp47: "zh-TW" },
  { id: "en", label: "English", native: "English", htmlLang: "en", bcp47: "en-US" },
  { id: "ja", label: "日本語", native: "日本語", htmlLang: "ja", bcp47: "ja-JP" },
  { id: "ko", label: "한국어", native: "한국어", htmlLang: "ko", bcp47: "ko-KR" },
  { id: "de", label: "Deutsch", native: "Deutsch", htmlLang: "de", bcp47: "de-DE" },
  { id: "vi", label: "Tiếng Việt", native: "Tiếng Việt", htmlLang: "vi", bcp47: "vi-VN" },
  { id: "th", label: "ไทย", native: "ไทย", htmlLang: "th", bcp47: "th-TH" },
]);

export const SUPPORTED = LOCALES.map((l) => l.id);

const dict = {
  zh: zhCN,
  "zh-TW": zhTW,
  en,
  ja,
  ko,
  de,
  vi,
  th,
};

/** @type {string} */
let locale = detectLocale();

function detectLocale() {
  try {
    const saved = localStorage.getItem(LOCALE_KEY);
    if (saved && SUPPORTED.includes(saved)) return saved;
    // migrate old keys
    if (saved === "zh-CN") return "zh";
  } catch (_) {}
  const nav = (typeof navigator !== "undefined" ? navigator.language || "" : "").toLowerCase();
  if (nav.startsWith("zh-tw") || nav.startsWith("zh-hk") || nav.startsWith("zh-mo") || nav === "zh-hant") return "zh-TW";
  if (nav.startsWith("zh")) return "zh";
  if (nav.startsWith("ja")) return "ja";
  if (nav.startsWith("ko")) return "ko";
  if (nav.startsWith("de")) return "de";
  if (nav.startsWith("vi")) return "vi";
  if (nav.startsWith("th")) return "th";
  if (nav.startsWith("en")) return "en";
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
  let s = table[key] ?? dict.en[key] ?? dict.zh[key] ?? key;
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

/** Whether locale uses 万/亿 style token formatting. */
export function usesChineseUnits() {
  return locale === "zh" || locale === "zh-TW";
}

/**
 * Best-effort translate of backend (Go) messages when UI is not Chinese.
 * @param {string} msg
 */
export function tb(msg) {
  if (!msg) return msg;
  if (locale === "zh" || locale === "zh-TW") {
    if (locale === "zh-TW" && typeof msg === "string") {
      // light traditional for common backend phrases
      return msg
        .replaceAll("厂家", "廠家")
        .replaceAll("配置", "設定")
        .replaceAll("失败", "失敗")
        .replaceAll("成功", "成功")
        .replaceAll("请", "請")
        .replaceAll("连接", "連線")
        .replaceAll("模型", "模型");
    }
    return msg;
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
