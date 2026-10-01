package httpserver

const hostUIHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Share Host Control</title>
    <style>
      body { font-family: system-ui, sans-serif; background: #0f172a; color: #e2e8f0; margin: 0; padding: 24px; }
      h1 { margin-top: 0; }
      .row { display: flex; gap: 12px; margin-bottom: 16px; flex-wrap: wrap; }
      button { background: #2563eb; color: white; border: 0; border-radius: 8px; padding: 10px 14px; cursor: pointer; }
      .secondary { background: #334155; }
      .card { background: #111827; border: 1px solid #334155; border-radius: 12px; padding: 12px 14px; margin-bottom: 10px; }
      .muted { color: #94a3b8; font-size: 0.9rem; }
      #status { margin-bottom: 16px; color: #67e8f9; }
      code { color: #cbd5e1; }
    </style>
  </head>
  <body>
    <h1>Select a target window</h1>
    <div id="status">Loading…</div>
    <div class="row">
      <button id="refresh">Refresh windows</button>
    </div>
    <div id="current" class="card"></div>
    <div id="windows"></div>
    <script>
      async function api(path, options = {}) {
        const res = await fetch(path, options);
        if (!res.ok) throw new Error(await res.text());
        return res;
      }
      const status = document.getElementById("status");
      const current = document.getElementById("current");
      const windowsRoot = document.getElementById("windows");

      async function loadCurrent() {
        const response = await api("/api/target-window");
        const payload = await response.json();
        if (!payload.selected) {
          current.innerHTML = "<strong>Current window:</strong> none selected";
          return;
        }
        current.innerHTML =
          "<strong>Current window:</strong> " + escapeHtml(payload.selected.title) +
          "<div class='muted'>" + escapeHtml(payload.selected.process_name) + " | HWND " + payload.selected.handle + "</div>";
      }

      async function loadWindows() {
        status.textContent = "Refreshing windows…";
        const response = await api("/api/windows");
        const windows = await response.json();
        windowsRoot.innerHTML = "";
        for (const windowItem of windows) {
          const card = document.createElement("div");
          card.className = "card";
          card.innerHTML =
            "<strong>" + escapeHtml(windowItem.title) + "</strong>" +
            "<div class='muted'>" + escapeHtml(windowItem.process_name) + " | HWND " + windowItem.handle + "</div>";
          const button = document.createElement("button");
          button.textContent = "Select window";
          button.addEventListener("click", async () => {
            try {
              status.textContent = "Selecting window…";
              await api("/api/target-window", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ handle: windowItem.handle })
              });
              await loadCurrent();
              status.textContent = "Window selected";
            } catch (error) { status.textContent = error.message; }
          });
          card.appendChild(document.createElement("div")).appendChild(button);
          windowsRoot.appendChild(card);
        }
        status.textContent = "Windows refreshed";
      }

      function escapeHtml(value) {
        return String(value)
          .replaceAll("&", "&amp;")
          .replaceAll("<", "&lt;")
          .replaceAll(">", "&gt;")
          .replaceAll('"', "&quot;");
      }

      async function reload() {
        try { await Promise.all([loadCurrent(), loadWindows()]); }
        catch (error) { status.textContent = error.message; }
      }
      document.getElementById("refresh").addEventListener("click", reload);
      reload();
    </script>
  </body>
</html>
`
