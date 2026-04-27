(function initDespatchI18n(root, factory) {
  const api = factory(root);
  if (typeof module !== "undefined" && module.exports) {
    module.exports = api;
  }
  root.DespatchI18n = api;
})(typeof globalThis !== "undefined" ? globalThis : window, function buildDespatchI18n(root) {
  const ATTRIBUTE_NAMES = ["aria-label", "title", "placeholder", "data-placeholder"];
  // Browser clients load locale bundles from the Go API, which parses .lpr
  // sources through the upstream Launchpad Rosetta native runtime.
  const COMPILED_FORMAT = "launchpad-rosetta-web-bundle-1";
  const observerState = {
    active: false,
    instance: null,
  };
  const textNodeState = new WeakMap();
  const attributeState = new WeakMap();

  let activeLocale = "en";
  let activeBundle = null;
  let mutationLock = false;

  function normalizeLocaleTag(value) {
    const raw = String(value || "").trim().replace(/_/g, "-");
    if (!raw) return "en";
    const parts = raw.split("-").filter(Boolean);
    if (!parts.length) return "en";
    return parts.map((part, index) => {
      if (index === 0) return part.toLowerCase();
      if (part.length === 2) return part.toUpperCase();
      return part.toLowerCase();
    }).join("-");
  }

  function localeCandidates(value) {
    const normalized = normalizeLocaleTag(value);
    const candidates = [];
    if (normalized) candidates.push(normalized);
    if (normalized.includes("-")) {
      candidates.push(normalized.split("-")[0]);
    }
    if (!candidates.includes("en")) {
      candidates.push("en");
    }
    return Array.from(new Set(candidates));
  }

  function currentLocale() {
    return activeBundle?.locale || activeLocale || "en";
  }

  function slugify(value) {
    const source = String(value || "")
      .normalize("NFKD")
      .replace(/[\u0300-\u036f]/g, "")
      .toLowerCase();
    const slug = source
      .replace(/[^a-z0-9]+/g, "_")
      .replace(/^_+|_+$/g, "")
      .replace(/_+/g, "_");
    return slug || "message";
  }

  function keyForSource(value) {
    const source = String(value || "").trim();
    if (!source) return "";
    return slugify(source);
  }

  function containsCyrillic(value) {
    return /[\u0400-\u04FF]/.test(String(value || ""));
  }

  function getArg(args, path) {
    if (!args || typeof args !== "object") return undefined;
    const parts = String(path || "").split(".");
    let current = args;
    for (const part of parts) {
      if (!part) return undefined;
      if (!Object.prototype.hasOwnProperty.call(current, part)) {
        return undefined;
      }
      current = current[part];
      if (current == null && part !== parts[parts.length - 1]) {
        return undefined;
      }
    }
    return current;
  }

  function formatTemplate(value, args) {
    return String(value || "").replace(/\{([A-Za-z0-9_.-]+)\}/g, (match, name) => {
      const resolved = getArg(args, name);
      if (resolved === undefined || resolved === null) {
        return match;
      }
      return String(resolved);
    });
  }

  function formatPluralEntry(entry, args) {
    const argName = String(entry?.arg || "count");
    const count = Number(getArg(args, argName));
    if (!Number.isFinite(count)) {
      return entry?.forms?.other || "";
    }
    const category = new Intl.PluralRules(currentLocale(), {
      type: entry?.pluralType === "ordinal" ? "ordinal" : "cardinal",
    }).select(count);
    const template = entry?.forms?.[category]
      || entry?.forms?.other
      || "";
    return formatTemplate(template, args);
  }

  function translate(source, args) {
    const text = String(source || "");
    if (!text.trim()) return text;
    const entry = activeBundle?.entries?.[keyForSource(text.trim())] || null;
    if (!entry) {
      return formatTemplate(text, args);
    }
    if (entry.kind === "plural") {
      return formatPluralEntry(entry, args);
    }
    return formatTemplate(entry.value || "", args);
  }

  function shouldSkipElement(element) {
    return !!element?.closest?.("[data-i18n-skip]");
  }

  function readAttributeTranslationState(element, name) {
    const state = attributeState.get(element);
    return state?.[name] || null;
  }

  function writeAttributeTranslationState(element, name, source, rendered) {
    const current = attributeState.get(element) || {};
    current[name] = {
      source: String(source || ""),
      rendered: String(rendered || ""),
      locale: currentLocale(),
    };
    attributeState.set(element, current);
  }

  function translateTextNode(node) {
    if (!node || node.nodeType !== 3) return;
    const parent = node.parentElement;
    if (!parent || shouldSkipElement(parent)) return;
    const original = String(node.nodeValue || "");
    const visibleText = original.trim();
    if (!visibleText) return;
    const cached = textNodeState.get(node) || null;
    let source = visibleText;
    if (cached && visibleText === cached.rendered) {
      source = cached.source;
    } else if (cached && visibleText !== cached.rendered) {
      textNodeState.delete(node);
    }
    if (!cached && containsCyrillic(source)) return;
    if (!/[A-Za-z]/.test(source)) return;
    const translated = translate(source);
    if (!translated) return;
    if (translated !== visibleText) {
      const start = original.indexOf(visibleText);
      const end = start + visibleText.length;
      node.nodeValue = `${original.slice(0, start)}${translated}${original.slice(end)}`;
    }
    textNodeState.set(node, {
      source,
      rendered: translated,
      locale: currentLocale(),
    });
  }

  function translateAttributes(element) {
    if (!element || element.nodeType !== 1 || shouldSkipElement(element)) return;
    for (const name of ATTRIBUTE_NAMES) {
      const value = element.getAttribute(name);
      if (!value) continue;
      const cached = readAttributeTranslationState(element, name);
      let source = value;
      if (cached && value === cached.rendered) {
        source = cached.source;
      }
      if (!cached && containsCyrillic(source)) continue;
      if (!/[A-Za-z]/.test(source)) continue;
      const translated = translate(source);
      if (translated && translated !== value) {
        element.setAttribute(name, translated);
      }
      if (translated) {
        writeAttributeTranslationState(element, name, source, translated);
      }
    }
  }

  function localizeTree(rootNode) {
    if (!activeBundle || !rootNode) return;
    const rootElement = rootNode.nodeType === 1 ? rootNode : rootNode.parentElement;
    if (rootElement && shouldSkipElement(rootElement)) return;
    if (rootNode.nodeType === 3) {
      translateTextNode(rootNode);
      return;
    }
    if (rootNode.nodeType !== 1 && rootNode.nodeType !== 9) {
      return;
    }
    if (rootNode.nodeType === 1) {
      translateAttributes(rootNode);
    }
    const walker = document.createTreeWalker(rootNode, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT, null);
    let current = walker.currentNode;
    while (current) {
      if (current.nodeType === 1) {
        translateAttributes(current);
      } else if (current.nodeType === 3) {
        translateTextNode(current);
      }
      current = walker.nextNode();
    }
  }

  function localizeDocument() {
    if (typeof document === "undefined") return;
    if (activeBundle?.locale) {
      document.documentElement.lang = activeBundle.locale;
    }
    if (document.title) {
      document.title = translate(document.title);
    }
    const description = document.querySelector("meta[name='description']");
    if (description) {
      const content = description.getAttribute("content");
      if (content) {
        description.setAttribute("content", translate(content));
      }
    }
    if (document.body) {
      localizeTree(document.body);
    }
  }

  function observeDocument() {
    if (observerState.instance) {
      observerState.instance.disconnect();
      observerState.instance = null;
      observerState.active = false;
    }
    if (!activeBundle || typeof MutationObserver !== "function" || !document?.body) {
      return;
    }
    observerState.instance = new MutationObserver((mutations) => {
      if (mutationLock) return;
      mutationLock = true;
      try {
        for (const mutation of mutations) {
          if (mutation.type === "characterData") {
            translateTextNode(mutation.target);
            continue;
          }
          if (mutation.type === "attributes") {
            translateAttributes(mutation.target);
            continue;
          }
          for (const node of mutation.addedNodes) {
            localizeTree(node);
          }
        }
      } finally {
        mutationLock = false;
      }
    });
    observerState.instance.observe(document.body, {
      subtree: true,
      childList: true,
      characterData: true,
      attributes: true,
      attributeFilter: ATTRIBUTE_NAMES,
    });
    observerState.active = true;
  }

  async function fetchBundle(locale) {
    if (locale === "en" || typeof fetch !== "function") return null;
    const response = await fetch(`/api/v1/public/i18n/${encodeURIComponent(locale)}`, {
      cache: "no-cache",
    });
    if (!response.ok) {
      if (response.status === 404) return null;
      throw new Error(`Failed to load locale bundle ${locale} (HTTP ${response.status}).`);
    }
    const data = await response.json();
    if (!data || data.format !== COMPILED_FORMAT || typeof data.entries !== "object") {
      throw new Error(`Invalid locale bundle for ${locale}.`);
    }
    return data;
  }

  function resolvePreferredLocale() {
    if (typeof window !== "undefined" && window.location) {
      const params = new URLSearchParams(window.location.search);
      const forced = params.get("lang");
      if (forced) {
        return normalizeLocaleTag(forced);
      }
    }
    if (typeof navigator !== "undefined" && Array.isArray(navigator.languages)) {
      const preferred = navigator.languages.find(Boolean);
      if (preferred) return normalizeLocaleTag(preferred);
    }
    if (typeof navigator !== "undefined" && navigator.language) {
      return normalizeLocaleTag(navigator.language);
    }
    if (typeof document !== "undefined" && document.documentElement?.lang) {
      return normalizeLocaleTag(document.documentElement.lang);
    }
    return "en";
  }

  async function init(preferredLocale) {
    const preferred = preferredLocale ? normalizeLocaleTag(preferredLocale) : resolvePreferredLocale();
    activeLocale = preferred;
    activeBundle = null;
    for (const candidate of localeCandidates(preferred)) {
      if (candidate === "en") break;
      const bundle = await fetchBundle(candidate);
      if (bundle) {
        activeBundle = bundle;
        activeLocale = candidate;
        break;
      }
    }
    localizeDocument();
    observeDocument();
    return {
      locale: currentLocale(),
      hasBundle: !!activeBundle,
    };
  }

  return {
    currentLocale,
    init,
    keyForSource,
    localizeDocument,
    localizeTree,
    translate,
  };
});
