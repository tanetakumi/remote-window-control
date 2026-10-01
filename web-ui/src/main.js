import "./styles.css";

import { fetchWindows, setTargetWindow } from "./core/api.js";
import { createRemoteConnection } from "./core/webrtc.js";
import { attachGestureControls } from "./input/gestures.js";
import { attachMouseControls } from "./input/mouse.js";
import { attachKeyboardBridge } from "./input/keyboard.js";
import { attachViewportSync, getViewportPayload } from "./input/viewport.js";
import { createListenerTracker } from "./lib/events.js";

const selectScreen = document.querySelector("#select-screen");
const remoteScreen = document.querySelector("#remote-screen");
const selectStatus = document.querySelector("#select-status");
const windowList = document.querySelector("#window-list");
const videoElement = document.querySelector("#remote-video");
const waitingElement = document.querySelector("#video-waiting");
const videoStageElement = document.querySelector("#video-stage");
const statusElement = document.querySelector("#status-pill");
const bitrateElement = document.querySelector("#bitrate-pill");
const fullscreenButton = document.querySelector("#fullscreen-button");
const keyboardButton = document.querySelector("#keyboard-button");
const hiddenInput = document.querySelector("#hidden-text-input");
const backspaceButton = document.querySelector("#backspace-button");
const enterButton = document.querySelector("#enter-button");
const refreshButton = document.querySelector("#refresh-button");
const searchInput = document.querySelector("#window-search");
const searchClearBtn = document.querySelector("#search-clear");
const countBadge = document.querySelector("#window-count-badge");

let connecting = false;
let cleanupRemote = () => {};
let connectionAbort;
let allWindows = [];
let searchQuery = "";

function getAppIconSvg(processName = "") {
  const name = processName.toLowerCase();
  if (
    name.includes("chrome") ||
    name.includes("edge") ||
    name.includes("firefox") ||
    name.includes("brave") ||
    name.includes("browser") ||
    name.includes("safari")
  ) {
    return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="10"></circle><line x1="2" y1="12" x2="22" y2="12"></line><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"></path></svg>`;
  }
  if (
    name.includes("code") ||
    name.includes("devenv") ||
    name.includes("idea") ||
    name.includes("sublime") ||
    name.includes("notepad") ||
    name.includes("vim")
  ) {
    return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="16 18 22 12 16 6"></polyline><polyline points="8 6 2 12 8 18"></polyline></svg>`;
  }
  if (
    name.includes("cmd") ||
    name.includes("powershell") ||
    name.includes("wt") ||
    name.includes("terminal") ||
    name.includes("bash") ||
    name.includes("alacritty") ||
    name.includes("kitty")
  ) {
    return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="4 17 10 11 4 5"></polyline><line x1="12" y1="19" x2="20" y2="19"></line></svg>`;
  }
  if (name.includes("explorer") || name.includes("finder") || name.includes("files")) {
    return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path></svg>`;
  }
  if (
    name.includes("discord") ||
    name.includes("slack") ||
    name.includes("teams") ||
    name.includes("zoom") ||
    name.includes("chat")
  ) {
    return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"></path></svg>`;
  }
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect width="20" height="14" x="2" y="3" rx="2"></rect><line x1="8" x2="16" y1="21" y2="21"></line><line x1="12" x2="12" y1="17" y2="21"></line></svg>`;
}

function renderSkeletons(count = 4) {
  windowList.replaceChildren();
  for (let i = 0; i < count; i++) {
    const skeleton = document.createElement("div");
    skeleton.className = "skeleton-card";
    skeleton.setAttribute("aria-hidden", "true");
    windowList.appendChild(skeleton);
  }
}

function renderWindowList(windows, onSelect) {
  windowList.replaceChildren();

  if (windows.length === 0) {
    const emptyState = document.createElement("div");
    emptyState.className = "empty-state";
    emptyState.innerHTML = `
      <svg class="empty-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <circle cx="11" cy="11" r="8"></circle>
        <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
        <line x1="8" y1="11" x2="14" y2="11"></line>
      </svg>
      <p class="empty-title">No matching windows</p>
      <p class="empty-desc">Check your search filter or ensure the application is active on the host machine.</p>
    `;
    windowList.appendChild(emptyState);
    return;
  }

  for (const target of windows) {
    const card = document.createElement("button");
    card.type = "button";
    card.className = "window-card";
    card.setAttribute(
      "aria-label",
      `Connect to ${target.title || "untitled"} (${target.process_name || "unknown process"})`
    );

    const iconWrapper = document.createElement("div");
    iconWrapper.className = "window-icon-wrapper";
    iconWrapper.innerHTML = getAppIconSvg(target.process_name);

    const content = document.createElement("div");
    content.className = "window-card-content";

    const title = document.createElement("span");
    title.className = "window-card-title";
    title.textContent = target.title || "(untitled)";

    const meta = document.createElement("div");
    meta.className = "window-card-meta";

    if (target.process_name) {
      const tag = document.createElement("span");
      tag.className = "process-tag";
      tag.textContent = target.process_name;
      meta.appendChild(tag);
    }

    const sub = document.createElement("span");
    sub.className = "window-card-sub";
    sub.textContent = target.handle ? `ID: ${target.handle}` : "";
    meta.appendChild(sub);

    content.append(title, meta);

    const arrow = document.createElement("div");
    arrow.className = "window-card-arrow";
    arrow.innerHTML = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="9 18 15 12 9 6"></polyline></svg>`;

    card.append(iconWrapper, content, arrow);
    card.addEventListener("click", () => onSelect(target));
    windowList.appendChild(card);
  }
}

function applyFilter() {
  const query = searchQuery.trim().toLowerCase();
  const filtered = query
    ? allWindows.filter(
        (w) =>
          (w.title && w.title.toLowerCase().includes(query)) ||
          (w.process_name && w.process_name.toLowerCase().includes(query))
      )
    : allWindows;

  if (countBadge) {
    if (query) {
      countBadge.textContent = `${filtered.length} of ${allWindows.length} windows`;
    } else {
      countBadge.textContent = `${allWindows.length} ${allWindows.length === 1 ? "window" : "windows"}`;
    }
  }

  renderWindowList(filtered, connectToWindow);
}

function setWindowListEnabled(enabled) {
  for (const button of windowList.querySelectorAll("button")) {
    button.disabled = !enabled;
  }
}

function setStatus(message) {
  statusElement.textContent = message;
  const lower = message.toLowerCase();
  if (lower.includes("ready") || lower.includes("connected") || lower.includes("active")) {
    statusElement.dataset.state = "connected";
  } else if (
    lower.includes("fail") ||
    lower.includes("error") ||
    lower.includes("close") ||
    lower.includes("disconnected")
  ) {
    statusElement.dataset.state = "error";
  } else {
    statusElement.dataset.state = "connecting";
  }
}

function setSelectStatus(message, isError = false) {
  selectStatus.textContent = message;
  if (isError) {
    selectStatus.classList.add("has-error");
  } else {
    selectStatus.classList.remove("has-error");
  }
}

function attachTopBarControls(onFullscreenChange) {
  const { listen, cleanup } = createListenerTracker();
  listen(fullscreenButton, "click", async () => {
    try {
      if (document.fullscreenElement) {
        await document.exitFullscreen();
      } else {
        if (document.documentElement.requestFullscreen) {
          await document.documentElement.requestFullscreen();
        } else {
          throw new Error("Fullscreen unavailable");
        }
      }
      onFullscreenChange?.();
    } catch {
      setStatus("Fullscreen is unavailable in this browser.");
    }
  });
  return cleanup;
}

async function showWindowSelect() {
  setSelectStatus("");
  refreshButton?.classList.add("is-loading");
  renderSkeletons(4);
  try {
    const windows = await fetchWindows();
    allWindows = Array.isArray(windows) ? windows : [];
    applyFilter();
    if (allWindows.length === 0) {
      setSelectStatus("No windows currently available.");
    }
  } catch (err) {
    windowList.replaceChildren();
    setSelectStatus(err instanceof Error ? err.message : "Loading failed", true);
  } finally {
    refreshButton?.classList.remove("is-loading");
  }
}

async function connectToWindow(target) {
  if (connecting) return;
  connecting = true;
  setWindowListEnabled(false);
  setSelectStatus(`Connecting to ${target.title || "window"}…`);
  try {
    await setTargetWindow(target.handle);
    selectScreen.hidden = true;
    remoteScreen.hidden = false;
    await startRemoteControl();
  } catch (err) {
    console.error("Connection failed:", err);
    returnToWindows(err instanceof Error ? err.message : "Error", true);
  } finally {
    connecting = false;
  }
}

async function startRemoteControl() {
  cleanupRemote();
  connectionAbort?.abort();
  connectionAbort = new AbortController();
  waitingElement.hidden = false;
  setStatus("Connecting…");
  const remote = await createRemoteConnection({
    getInitialViewport: () => getViewportPayload(videoStageElement),
    signal: connectionAbort.signal,
    videoElement,
    onStatus: setStatus,
    onDisconnect: (error) => returnToWindows(error.message, true),
    onBitrate: (kbps) => {
      bitrateElement.textContent = `${kbps} kbps`;
      bitrateElement.hidden = false;
    },
  });
  waitingElement.hidden = true;
  const releaseGestures = attachGestureControls(videoElement, remote.sendControl, setStatus);
  const releaseMouse = attachMouseControls(videoElement, remote.sendControl);
  const keyboard = attachKeyboardBridge(
    {
      buttonElement: keyboardButton,
      inputElement: hiddenInput,
      backspaceButton,
      enterButton,
    },
    remote.sendControl,
    setStatus
  );
  const viewport = attachViewportSync(remote.sendControl, {
    isSuspended: () => keyboard.isActive(),
    targetElement: videoStageElement,
  });
  const releaseTopBar = attachTopBarControls(viewport.triggerFullscreenSync);
  cleanupRemote = () => {
    releaseGestures();
    releaseMouse();
    keyboard.cleanup();
    viewport.cleanup();
    releaseTopBar();
    remote.close();
    bitrateElement.hidden = true;
    cleanupRemote = () => {};
  };
  setStatus("Control ready");
}

function returnToWindows(message = "Select a window to connect.", isError = false) {
  cleanupRemote();
  connectionAbort?.abort();
  selectScreen.hidden = false;
  remoteScreen.hidden = true;
  setSelectStatus(message, isError);
  setWindowListEnabled(true);
}

searchInput?.addEventListener("input", (e) => {
  searchQuery = e.target.value;
  if (searchClearBtn) {
    searchClearBtn.hidden = !searchQuery;
  }
  applyFilter();
});

searchClearBtn?.addEventListener("click", () => {
  if (searchInput) {
    searchInput.value = "";
    searchQuery = "";
    searchClearBtn.hidden = true;
    searchInput.focus();
  }
  applyFilter();
});

videoElement.addEventListener("click", () =>
  videoElement.play()?.catch(() => setStatus("Playback could not start."))
);

document.querySelector("#windows-button").addEventListener("click", () => {
  returnToWindows();
  showWindowSelect();
});

refreshButton?.addEventListener("click", () => {
  if (!connecting) showWindowSelect();
});

window.addEventListener("pagehide", () => returnToWindows());
document.addEventListener("visibilitychange", () => {
  if (document.hidden) returnToWindows("Connection paused. Select a window to reconnect.");
});

async function main() {
  await showWindowSelect();
}

main();
