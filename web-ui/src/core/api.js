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
