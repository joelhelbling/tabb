// Injected into the page by background.js via chrome.scripting.executeScript.
// Receives config as the first argument: { url, mode, matchValue, matchType, matchIndex }
function showTabignoreDialog(config) {
  // Remove any existing dialog
  const existing = document.getElementById("tabb-tabignore-host");
  if (existing) existing.remove();

  const host = document.createElement("div");
  host.id = "tabb-tabignore-host";
  host.style.cssText = "all:initial; position:fixed; z-index:2147483647; top:0; left:0; width:100%; height:100%;";
  document.body.appendChild(host);

  const shadow = host.attachShadow({ mode: "closed" });

  const style = document.createElement("style");
  style.textContent = `
    * { box-sizing: border-box; margin: 0; padding: 0; }
    .overlay {
      position: fixed; top: 0; left: 0; width: 100%; height: 100%;
      background: rgba(0,0,0,0.35);
      display: flex; align-items: flex-start; justify-content: center;
      padding-top: 80px;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      font-size: 13px; color: #333;
    }
    .dialog {
      background: #fff; border-radius: 8px;
      box-shadow: 0 8px 32px rgba(0,0,0,0.25);
      padding: 16px; width: 380px; max-width: 90vw;
    }
    h3 { font-size: 14px; font-weight: 600; margin-bottom: 12px; }
    .tab-url {
      font-family: monospace; font-size: 11px; color: #666;
      word-break: break-all; margin-bottom: 14px;
      padding: 6px 8px; background: #f5f5f5; border-radius: 4px;
    }
    select, input[type="text"] {
      width: 100%; padding: 6px 8px; border: 1px solid #ccc;
      border-radius: 4px; font-size: 13px;
    }
    select { margin-bottom: 8px; }
    input[type="text"] { margin-bottom: 6px; }
    .warning { font-size: 10px; color: #b45309; margin-bottom: 6px; }
    .buttons { display: flex; justify-content: flex-end; gap: 8px; margin-top: 12px; }
    button {
      padding: 6px 16px; border: 1px solid #ccc; border-radius: 4px;
      cursor: pointer; font-size: 13px; background: #fff;
    }
    button:hover { background: #f5f5f5; }
    button.primary { background: #2563eb; color: white; border-color: #2563eb; }
    button.primary:hover { background: #1d4ed8; }
    .ignored-msg { margin-bottom: 8px; color: #666; }
    .match-pattern {
      font-family: monospace; font-size: 12px; padding: 8px;
      background: #fef3c7; border: 1px solid #f59e0b; border-radius: 4px;
      margin-bottom: 4px; word-break: break-all;
    }
    .match-type { font-size: 10px; color: #888; text-transform: uppercase; margin-bottom: 12px; }
  `;
  shadow.appendChild(style);

  const overlay = document.createElement("div");
  overlay.className = "overlay";
  shadow.appendChild(overlay);

  const dialog = document.createElement("div");
  dialog.className = "dialog";
  overlay.appendChild(dialog);

  // Close on overlay click (outside dialog)
  overlay.addEventListener("click", (e) => {
    if (e.target === overlay) close();
  });

  function close() {
    host.remove();
  }

  // Close on Escape
  function onKeyDown(e) {
    if (e.key === "Escape") {
      close();
      document.removeEventListener("keydown", onKeyDown, true);
    }
  }
  document.addEventListener("keydown", onKeyDown, true);

  const tabUrl = config.url || "";
  let parsed;
  try { parsed = new URL(tabUrl); } catch { parsed = null; }

  const presets = {
    domain: parsed ? parsed.hostname : tabUrl,
    domain_path: parsed ? parsed.hostname + parsed.pathname.replace(/\/$/, "") : tabUrl,
    url: tabUrl,
    regex: "",
  };

  if (config.mode === "ignored") {
    const typeLabels = { domain: "Domain", domain_path: "Domain + path", url: "Full URL", regex: "Regex" };
    dialog.innerHTML = `
      <h3>Tab already ignored</h3>
      <div class="tab-url"></div>
      <div class="ignored-msg">This tab is hidden by the following pattern:</div>
      <div class="match-pattern"></div>
      <div class="match-type"></div>
      <div class="buttons">
        <button class="remove-btn">Remove pattern</button>
        <button class="ok-btn">OK</button>
      </div>
    `;
    dialog.querySelector(".tab-url").textContent = tabUrl;
    dialog.querySelector(".match-pattern").textContent = config.matchValue;
    dialog.querySelector(".match-type").textContent = typeLabels[config.matchType] || config.matchType;

    dialog.querySelector(".ok-btn").addEventListener("click", close);
    dialog.querySelector(".remove-btn").addEventListener("click", () => {
      chrome.runtime.sendMessage({
        type: "tabignore-remove",
        index: config.matchIndex,
      });
      close();
    });
  } else {
    dialog.innerHTML = `
      <h3>Add to tabignore</h3>
      <div class="tab-url"></div>
      <select class="pattern-type">
        <option value="domain">Domain</option>
        <option value="domain_path">Domain + path</option>
        <option value="url">Full URL</option>
        <option value="regex">Regex (caveat emptor)</option>
      </select>
      <input type="text" class="pattern-value">
      <div class="warning regex-warning" style="display:none">
        Regex patterns are powerful but can cause unexpected matches.
      </div>
      <div class="buttons">
        <button class="cancel-btn">Cancel</button>
        <button class="primary add-btn">Ignore</button>
      </div>
    `;
    dialog.querySelector(".tab-url").textContent = tabUrl;

    const typeSelect = dialog.querySelector(".pattern-type");
    const valueInput = dialog.querySelector(".pattern-value");
    const regexWarning = dialog.querySelector(".regex-warning");

    valueInput.value = presets[typeSelect.value];

    typeSelect.addEventListener("change", () => {
      valueInput.value = presets[typeSelect.value];
      regexWarning.style.display = typeSelect.value === "regex" ? "block" : "none";
    });

    dialog.querySelector(".cancel-btn").addEventListener("click", close);
    dialog.querySelector(".add-btn").addEventListener("click", () => {
      const type = typeSelect.value;
      const value = valueInput.value.trim();
      if (!value) return;

      if (type === "regex") {
        try { new RegExp(value); } catch (e) {
          alert("Invalid regex: " + e.message);
          return;
        }
      }

      chrome.runtime.sendMessage({
        type: "tabignore-add",
        pattern: { type, value, label: value },
      });
      close();
    });
  }
}
