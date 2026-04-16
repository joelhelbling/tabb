---
name: tabb
description: Use when the user mentions the browser tabs they currently have open, or asks a question whose answer depends on what they're currently browsing (e.g. "summarize the articles I have open on X", "what tabs do I have open about Y", "close the tabs I'm done with"). Provides access to open Chrome tabs via the tabb MCP server.
---

# tabb

`tabb` exposes the user's open Chrome tabs through an MCP server. Reach for it whenever the user refers to tabs they currently have open — **do not** try to use puppeteer, a headless browser, or web search as a substitute. Those retrieve *public* pages; `tabb` retrieves the *user's* pages in the state they are in (logged in, mid-form, behind paywalls, etc.).

## Tools

- **`list_profiles`** — returns the user's registered browser profiles (name, browser, profileId, active status). Call this when you need to disambiguate between profiles or when a tool returns a "multiple profiles" error.
- **`list_tabs`** — returns metadata for every open tab (id, title, url, window, pinned). Supports a `filter` string that matches titles and URLs case-insensitively. Call this **first** to survey what's there.
- **`show_tab`** — returns one tab's content as markdown. Token-heavy: a single long article can be thousands of tokens. Call this only after you've narrowed the set with `list_tabs`.
- **`focus_tab`** — brings a tab to the foreground (activates it and focuses its window). Pass `reload: true` to also refresh the page. Not destructive — safe to call without confirmation. Useful when the user wants to find a tab or when retrying a `show_tab` that failed due to page restrictions.
- **`close_tab`** — closes one tab by id. Destructive from the user's perspective (they lose their place). Only call after explicit user confirmation.

## Usage pattern

1. **Narrow with `list_tabs` first.** If the user's question is topical, pass a filter that matches the topic. Don't `show_tab` everything — open tabs can number in the hundreds.
2. **Show the matched list to the user** before fetching content, especially if the filter returns more than a handful. Let them confirm or narrow further. This protects both their time and your context budget.
3. **Use `show_tab` sparingly** — usually for the 3–10 tabs that matter. Prefer the Readability-extracted mode (the default); only use raw DOM when Readability fails or the user explicitly asks.
4. **Never close tabs without explicit confirmation.** After using tab content to answer a question, it's often helpful to ask the user if they'd like you to close the tabs you just consumed — but wait for a clear yes before calling `close_tab`. Skip pinned tabs unless the user says otherwise.

## Notes

- The user may have configured a **tabignore** list; those tabs are filtered out by the extension before they reach the MCP server. You never see them, and that's intentional — don't work around it.
- **Profiles**: if the user has registered more than one browser profile, tabb can't auto-pick one. Call `list_profiles` to see what's available (name, browser, active status), then pass `profile: "<name>"` to `list_tabs` / `show_tab` / `close_tab`. If the user refers to a specific profile by name ("my Brave tabs", "my work browser"), pass it directly without the extra round trip. If a tool call returns an error like "multiple active tabb profiles found", that's your cue to call `list_profiles` and retry with an explicit `profile`.
- If `list_tabs` returns an error about no active profile or missing socket, tell the user to make sure Chrome is running with the tabb extension loaded. Don't try to fix their setup from a skill.
