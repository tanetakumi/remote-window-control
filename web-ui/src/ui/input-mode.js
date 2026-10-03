export function renderInputMode(button, mode = "window", switching = false, ready = false) {
  const pc = mode === "pc";
  button.setAttribute("aria-pressed", String(pc));
  button.setAttribute("aria-busy", String(switching));
  button.setAttribute("aria-label", `PC control: ${pc ? "on" : "off"}. Switch to ${pc ? "Window" : "PC"} mode`);
  button.title = `Switch to ${pc ? "Window" : "PC"} mode`;
  button.disabled = !ready || switching;
  button.querySelector(".mode-label").textContent = pc ? "PC" : "Window";
  button.querySelector(".input-window-icon").toggleAttribute("hidden", pc);
  button.querySelector(".input-pc-icon").toggleAttribute("hidden", !pc);
}
