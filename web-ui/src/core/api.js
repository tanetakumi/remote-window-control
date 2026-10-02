export async function fetchWindows() {
  const res = await fetch("/api/windows");
  if (!res.ok) throw new Error((await res.text()).trim() || "Could not load windows");
  return res.json();
}

export async function setTargetWindow(handle) {
  const res = await fetch("/api/target-window", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ handle }),
  });
  if (!res.ok) throw new Error((await res.text()).trim() || "Could not select the window");
  return res.json();
}

export async function fetchKeepalive() {
  const res = await fetch("/api/session-keepalive");
  if (!res.ok) throw new Error((await res.text()).trim() || "Could not load the keep-alive status");
  return res.json();
}

export async function setKeepalive(enabled) {
  const res = await fetch("/api/session-keepalive", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
  });
  if (!res.ok) throw new Error((await res.text()).trim() || "Could not toggle the session keep-alive");
  return res.json();
}

export async function fetchSettings() {
  const res = await fetch("/api/settings");
  if (!res.ok) throw new Error((await res.text()).trim() || "Could not load the settings");
  return res.json();
}

export async function saveSettings(settings) {
  const res = await fetch("/api/settings", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(settings),
  });
  if (!res.ok) throw new Error((await res.text()).trim() || "Could not save the settings");
  return res.json();
}
