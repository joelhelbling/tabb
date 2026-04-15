// tabb background service worker
// Connects to the native messaging host and proxies Chrome API calls.

const NATIVE_HOST = "com.tabb";
let port = null;

function connect() {
  port = chrome.runtime.connectNative(NATIVE_HOST);

  port.onMessage.addListener((msg) => {
    dispatch(msg)
      .then((response) => port.postMessage(response))
      .catch((err) =>
        port.postMessage({ id: msg.id, error: err.message || String(err) })
      );
  });

  port.onDisconnect.addListener(() => {
    const err = chrome.runtime.lastError?.message || "unknown";
    console.log("tabb: native host disconnected:", err);
    port = null;
    setTimeout(connect, 5000);
  });

  console.log("tabb: connected to native host");

  // Send handshake with browser info
  const brands = navigator.userAgentData?.brands || [];
  const browser = brands.find(b =>
    ["Google Chrome", "Brave", "Microsoft Edge", "Opera", "Vivaldi"].includes(b.brand)
  );
  port.postMessage({
    action: "handshake",
    params: {
      browser: browser?.brand || "Chrome",
      extensionId: chrome.runtime.id
    }
  });
}

connect();

// --- Dispatch ---

async function dispatch(msg) {
  switch (msg.action) {
    case "list_tabs":
      return await listTabs(msg);
    case "show_tab":
      return await showTab(msg);
    case "close_tab":
      return await closeTab(msg);
    case "handshake":
      return { id: msg.id, data: { ok: true } };
    default:
      return { id: msg.id, error: `unknown action: ${msg.action}` };
  }
}

// --- list_tabs ---

async function listTabs(msg) {
  const tabs = await chrome.tabs.query({});
  const patterns = await getTabignorePatterns();

  let result = tabs
    .filter((tab) => !isIgnored(tab.url, patterns))
    .map((tab) => tabMeta(tab));

  if (msg.params?.filter) {
    const f = msg.params.filter.toLowerCase();
    result = result.filter(
      (t) => t.title.toLowerCase().includes(f) || t.url.toLowerCase().includes(f)
    );
  }

  return { id: msg.id, data: result };
}

// --- show_tab ---

async function showTab(msg) {
  const tabId = msg.params?.tabId;
  if (!tabId) return { id: msg.id, error: "tabId is required" };

  const tab = await chrome.tabs.get(tabId);
  const patterns = await getTabignorePatterns();
  if (isIgnored(tab.url, patterns)) {
    return { id: msg.id, error: "tab is in tabignore list" };
  }

  const raw = msg.params?.raw || false;

  try {
    // Inject libraries, then extract content
    await chrome.scripting.executeScript({
      target: { tabId },
      files: ["lib/Readability.js", "lib/turndown.js"],
    });

    const results = await chrome.scripting.executeScript({
      target: { tabId },
      func: extractContent,
      args: [raw],
    });

    const content = results[0]?.result;
    if (!content) {
      return { id: msg.id, error: "failed to extract content from tab" };
    }

    return { id: msg.id, data: { ...tabMeta(tab), content } };
  } catch (err) {
    return { id: msg.id, error: `script execution failed: ${err.message}` };
  }
}

// Runs in the tab's page context after Readability and Turndown are injected.
function extractContent(raw) {
  const turndown =
    typeof TurndownService !== "undefined"
      ? new TurndownService({ headingStyle: "atx" })
      : null;

  if (raw) {
    return turndown
      ? turndown.turndown(document.body.innerHTML)
      : document.body.innerText;
  }

  // Readability mode
  if (typeof Readability !== "undefined") {
    const docClone = document.cloneNode(true);
    const article = new Readability(docClone).parse();
    if (article) {
      return turndown
        ? turndown.turndown(article.content)
        : article.textContent;
    }
  }

  // Fallback
  return turndown
    ? turndown.turndown(document.body.innerHTML)
    : document.body.innerText;
}

// --- close_tab ---

async function closeTab(msg) {
  const tabId = msg.params?.tabId;
  if (!tabId) return { id: msg.id, error: "tabId is required" };

  await chrome.tabs.remove(tabId);
  return { id: msg.id, data: { closed: true } };
}

// --- Helpers ---

function tabMeta(tab) {
  return {
    id: tab.id,
    windowId: tab.windowId,
    title: tab.title || "",
    url: tab.url || "",
    status: tab.status || "",
    active: tab.active || false,
    pinned: tab.pinned || false,
    audible: tab.audible || false,
    discarded: tab.discarded || false,
    favIconUrl: tab.favIconUrl || "",
    index: tab.index,
  };
}

// --- Tabignore ---

async function getTabignorePatterns() {
  const result = await chrome.storage.local.get("tabignore");
  return result.tabignore || [];
}

function isIgnored(url, patterns) {
  if (!url) return false;
  let parsed;
  try {
    parsed = new URL(url);
  } catch {
    return false;
  }

  for (const p of patterns) {
    switch (p.type) {
      case "domain":
        if (
          parsed.hostname === p.value ||
          parsed.hostname.endsWith("." + p.value)
        )
          return true;
        break;
      case "domain_path": {
        const parts = p.value.split("/");
        const domain = parts[0];
        const path = "/" + parts.slice(1).join("/");
        if (
          (parsed.hostname === domain ||
            parsed.hostname.endsWith("." + domain)) &&
          parsed.pathname.startsWith(path)
        )
          return true;
        break;
      }
      case "url":
        if (url === p.value || url.startsWith(p.value)) return true;
        break;
      case "regex":
        try {
          if (new RegExp(p.value).test(url)) return true;
        } catch {
          // invalid regex, skip
        }
        break;
    }
  }
  return false;
}

// --- Context Menu ---

chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.create({
    id: "tabb-tabignore",
    title: "Add to tabignore",
    contexts: ["page"],
  });
});

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  if (info.menuItemId !== "tabb-tabignore" || !tab) return;
  try {
    await openTabignoreDialog(tab.id, tab.url);
  } catch (e) {
    console.error("tabb: failed to open tabignore dialog:", e);
  }
});

// Find the first tabignore pattern that matches a URL
function findMatchingPattern(url, patterns) {
  if (!url) return null;
  let parsed;
  try {
    parsed = new URL(url);
  } catch {
    return null;
  }

  for (let i = 0; i < patterns.length; i++) {
    const p = patterns[i];
    let matches = false;
    switch (p.type) {
      case "domain":
        matches =
          parsed.hostname === p.value ||
          parsed.hostname.endsWith("." + p.value);
        break;
      case "domain_path": {
        const parts = p.value.split("/");
        const domain = parts[0];
        const path = "/" + parts.slice(1).join("/");
        matches =
          (parsed.hostname === domain ||
            parsed.hostname.endsWith("." + domain)) &&
          parsed.pathname.startsWith(path);
        break;
      }
      case "url":
        matches = url === p.value || url.startsWith(p.value);
        break;
      case "regex":
        try {
          matches = new RegExp(p.value).test(url);
        } catch {
          // invalid regex
        }
        break;
    }
    if (matches) return { pattern: p, index: i };
  }
  return null;
}

// Handle messages from tabignore dialog and popup
chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (msg.type === "tabignore-add") {
    getTabignorePatterns().then((patterns) => {
      patterns.push(msg.pattern);
      chrome.storage.local.set({ tabignore: patterns });
      console.log(`tabb: added tabignore pattern: ${msg.pattern.value}`);
      sendResponse({ ok: true });
    });
    return true;
  }
  if (msg.type === "tabignore-remove") {
    getTabignorePatterns().then((patterns) => {
      if (msg.index >= 0 && msg.index < patterns.length) {
        const removed = patterns.splice(msg.index, 1)[0];
        chrome.storage.local.set({ tabignore: patterns });
        console.log(`tabb: removed tabignore pattern: ${removed.value}`);
      }
      sendResponse({ ok: true });
    });
    return true;
  }
  if (msg.type === "tabignore-check") {
    getTabignorePatterns().then((patterns) => {
      const match = findMatchingPattern(msg.url, patterns);
      if (match) {
        sendResponse({
          ignored: true,
          matchValue: match.pattern.value,
          matchType: match.pattern.type,
          matchIndex: match.index,
        });
      } else {
        sendResponse({ ignored: false });
      }
    });
    return true;
  }
  if (msg.type === "tabignore-open-dialog") {
    openTabignoreDialog(msg.tabId, msg.url);
    sendResponse({ ok: true });
  }
});

// Open the injected tabignore dialog on a tab
async function openTabignoreDialog(tabId, url) {
  const patterns = await getTabignorePatterns();
  const match = findMatchingPattern(url, patterns);

  const config = {
    url: url,
    mode: match ? "ignored" : "add",
  };
  if (match) {
    config.matchValue = match.pattern.value;
    config.matchType = match.pattern.type;
    config.matchIndex = match.index;
  }

  await chrome.scripting.executeScript({
    target: { tabId },
    files: ["tabignore-dialog.js"],
  });
  await chrome.scripting.executeScript({
    target: { tabId },
    func: (cfg) => showTabignoreDialog(cfg),
    args: [config],
  });
}
