const patternList = document.getElementById("patternList");
const emptyMsg = document.getElementById("emptyMsg");
const patternType = document.getElementById("patternType");
const patternValue = document.getElementById("patternValue");
const addBtn = document.getElementById("addBtn");
const regexWarning = document.getElementById("regexWarning");
const ignoreTabBtn = document.getElementById("ignoreTabBtn");
const alreadyIgnored = document.getElementById("alreadyIgnored");
const ignoredPattern = document.getElementById("ignoredPattern");

// Pre-fill from current tab and check if already ignored
let presets = { domain: "", domain_path: "", url: "", regex: "" };
let currentTab = null;

chrome.tabs.query({ active: true, currentWindow: true }).then(async (tabs) => {
  currentTab = tabs[0];
  if (currentTab?.url) {
    try {
      const parsed = new URL(currentTab.url);
      presets.domain = parsed.hostname;
      presets.domain_path =
        parsed.hostname + parsed.pathname.replace(/\/$/, "");
      presets.url = currentTab.url;
      presets.regex = "";
      patternValue.value = presets[patternType.value];
    } catch {
      // not a valid URL
    }

    // Check if current tab is already ignored
    const resp = await chrome.runtime.sendMessage({
      type: "tabignore-check",
      url: currentTab.url,
    });
    if (resp?.ignored) {
      ignoreTabBtn.style.display = "none";
      alreadyIgnored.style.display = "flex";
      ignoredPattern.textContent = `${resp.matchValue} (${resp.matchType})`;
    }
  }
});

ignoreTabBtn.addEventListener("click", async () => {
  if (!currentTab) return;
  // Ask background to open the modal on the active tab
  await chrome.runtime.sendMessage({
    type: "tabignore-open-dialog",
    tabId: currentTab.id,
    url: currentTab.url,
  });
  window.close(); // close the popup
});

patternType.addEventListener("change", () => {
  regexWarning.style.display =
    patternType.value === "regex" ? "block" : "none";
  patternValue.value = presets[patternType.value];
});

addBtn.addEventListener("click", async () => {
  const type = patternType.value;
  const value = patternValue.value.trim();
  if (!value) return;

  // Validate regex
  if (type === "regex") {
    try {
      new RegExp(value);
    } catch (e) {
      alert("Invalid regex: " + e.message);
      return;
    }
  }

  const result = await chrome.storage.local.get("tabignore");
  const patterns = result.tabignore || [];
  patterns.push({ type, value, label: value });
  await chrome.storage.local.set({ tabignore: patterns });

  patternValue.value = "";
  renderPatterns(patterns);
});

async function removePattern(index) {
  const result = await chrome.storage.local.get("tabignore");
  const patterns = result.tabignore || [];
  patterns.splice(index, 1);
  await chrome.storage.local.set({ tabignore: patterns });
  renderPatterns(patterns);
}

function renderPatterns(patterns) {
  // Clear existing patterns (keep empty message element)
  patternList.innerHTML = "";

  if (patterns.length === 0) {
    const empty = document.createElement("div");
    empty.className = "empty";
    empty.textContent = "No tabignore patterns yet.";
    patternList.appendChild(empty);
    return;
  }

  patterns.forEach((p, i) => {
    const div = document.createElement("div");
    div.className = "pattern";
    div.innerHTML = `
      <div class="pattern-info">
        <div class="pattern-value">${escapeHtml(p.value)}</div>
        <div class="pattern-type">${escapeHtml(p.type)}</div>
      </div>
    `;
    const btn = document.createElement("button");
    btn.className = "pattern-remove";
    btn.textContent = "×";
    btn.addEventListener("click", () => removePattern(i));
    div.appendChild(btn);
    patternList.appendChild(div);
  });
}

function escapeHtml(str) {
  const div = document.createElement("div");
  div.textContent = str;
  return div.innerHTML;
}

// Load patterns on popup open
chrome.storage.local.get("tabignore").then((result) => {
  renderPatterns(result.tabignore || []);
});
