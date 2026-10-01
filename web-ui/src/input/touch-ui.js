import { createListenerTracker } from "../lib/events.js";
import { getVideoContentRect } from "./coordinates.js";
import { attachGestureControls, DEFAULT_TOUCH_MODE, TOUCH_MODES } from "./gestures.js";

const STORAGE_KEY = "share-app.touch-mode";
const MODE_LABELS = {
  [TOUCH_MODES.RELATIVE]: "Pointer",
  [TOUCH_MODES.DIRECT]: "Tap",
};

export function attachTouchControlsUI({ videoElement, stageElement, cursorElement, modeElement }, sendControl) {
  const view = videoElement.ownerDocument.defaultView;
  const { listen, cleanup: removeListeners } = createListenerTracker();
  let storage;
  let mode = DEFAULT_TOUCH_MODE;
  let cursor = { x: 0.5, y: 0.5 };
  try {
    storage = view.localStorage;
    const saved = storage?.getItem(STORAGE_KEY);
    if (Object.values(TOUCH_MODES).includes(saved)) mode = saved;
  } catch { /* Private browsing can disable storage; the controls still work. */ }

  const renderCursor = () => {
    const rect = getVideoContentRect(videoElement);
    const stage = stageElement.getBoundingClientRect();
    cursorElement.hidden = mode !== TOUCH_MODES.RELATIVE || rect.width <= 0 || rect.height <= 0;
    cursorElement.style.left = `${rect.left - stage.left + cursor.x * rect.width}px`;
    cursorElement.style.top = `${rect.top - stage.top + cursor.y * rect.height}px`;
  };
  const renderMode = () => {
    const label = MODE_LABELS[mode];
    const nextLabel = MODE_LABELS[mode === TOUCH_MODES.RELATIVE ? TOUCH_MODES.DIRECT : TOUCH_MODES.RELATIVE];
    modeElement.textContent = label;
    modeElement.setAttribute("aria-label", `Input mode: ${label}. Switch to ${nextLabel}.`);
    modeElement.title = `Switch to ${nextLabel} mode`;
    renderCursor();
  };
  const gestures = attachGestureControls(videoElement, sendControl, {
    mode,
    onCursorChange(point) { cursor = point; renderCursor(); },
  });
  listen(modeElement, "click", () => {
    mode = mode === TOUCH_MODES.RELATIVE ? TOUCH_MODES.DIRECT : TOUCH_MODES.RELATIVE;
    gestures.setMode(mode);
    try { storage?.setItem(STORAGE_KEY, mode); } catch { /* Session-only preference. */ }
    renderMode();
  });
  for (const type of ["loadedmetadata", "resize"]) listen(videoElement, type, renderCursor);
  listen(view, "resize", renderCursor);
  const observer = view.ResizeObserver ? new view.ResizeObserver(renderCursor) : null;
  observer?.observe(videoElement);
  renderMode();
  modeElement.disabled = false;

  return () => {
    gestures.cleanup();
    removeListeners();
    observer?.disconnect();
    cursorElement.hidden = true;
    modeElement.disabled = true;
  };
}
