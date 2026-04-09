const patternList = document.getElementById("patternList");
const emptyMsg = document.getElementById("emptyMsg");
const patternType = document.getElementById("patternType");
const patternValue = document.getElementById("patternValue");
const addBtn = document.getElementById("addBtn");
const regexWarning = document.getElementById("regexWarning");

patternType.addEventListener("change", () => {
  regexWarning.style.display =
    patternType.value === "regex" ? "block" : "none";
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
