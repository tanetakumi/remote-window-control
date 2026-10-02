import { createListenerTracker } from "../lib/events.js";
import { getVideoContentRect } from "./coordinates.js";
import { attachGestureControls, DEFAULT_TOUCH_MODE, TOUCH_MODES } from "./gestures.js";

const STORAGE_KEY = "share-app.touch-mode";

export function attachTouchControlsUI({ videoElement, stageElement, cursorElement, modeElement }, sendControl) {
  const view = videoElement.ownerDocument.defaultView;
  const { listen, cleanup: removeListeners } = createListenerTracker();
  const modeButtons = [...modeElement.querySelectorAll("button[data-mode]")];
  let storage;
  let mode = DEFAULT_TOUCH_MODE;
  let cursor = { x: 0.5, y: 0.5 };
  let rect;
  let stage;
  try {
    storage = view.localStorage;
    const saved = storage?.getItem(STORAGE_KEY);
    if (Object.values(TOUCH_MODES).includes(saved)) mode = saved;
  } catch { /* Private browsing can disable storage; the controls still work. */ }

  const renderCursor = () => {
    // Movement only changes the compositor transform. Reading layout here
    // after writing left/top on the previous move would force a reflow.
    const x = rect.left - stage.left + cursor.x * rect.width;
    const y = rect.top - stage.top + cursor.y * rect.height;
    cursorElement.style.transform = `translate3d(${x}px, ${y}px, 0)`;
  };
  const refreshGeometry = () => {
    rect = getVideoContentRect(videoElement);
    stage = stageElement.getBoundingClientRect();
    cursorElement.hidden = mode !== TOUCH_MODES.RELATIVE || rect.width <= 0 || rect.height <= 0;
    renderCursor();
  };
  const renderMode = () => {
    for (const button of modeButtons) {
      button.setAttribute("aria-pressed", String(button.dataset.mode === mode));
    }
    refreshGeometry();
  };
  renderMode();
  // Refresh at gesture boundaries as well, in case surrounding UI moved
  // without resizing the video. Register before the gesture's touchstart.
  listen(videoElement, "touchstart", refreshGeometry, { passive: true });
  const gestures = attachGestureControls(videoElement, sendControl, {
    mode,
    onCursorChange(point) { cursor = point; renderCursor(); },
  });
  for (const button of modeButtons) {
    button.disabled = false;
    listen(button, "click", () => {
      const nextMode = button.dataset.mode;
      if (button.disabled || nextMode === mode || !Object.values(TOUCH_MODES).includes(nextMode)) return;
      mode = nextMode;
      gestures.setMode(mode);
      try { storage?.setItem(STORAGE_KEY, mode); } catch { /* Session-only preference. */ }
      renderMode();
    });
  }
  for (const type of ["loadedmetadata", "resize"]) listen(videoElement, type, refreshGeometry);
  listen(view, "resize", refreshGeometry);
  listen(view, "scroll", refreshGeometry, { capture: true, passive: true });
  for (const type of ["fullscreenchange", "webkitfullscreenchange"]) {
    listen(videoElement.ownerDocument, type, refreshGeometry);
  }
  if (view.visualViewport) {
    for (const type of ["resize", "scroll"]) listen(view.visualViewport, type, refreshGeometry);
  }
  const observer = view.ResizeObserver ? new view.ResizeObserver(refreshGeometry) : null;
  observer?.observe(videoElement);
  observer?.observe(stageElement);

  return () => {
    gestures.cleanup();
    removeListeners();
    observer?.disconnect();
    cursorElement.hidden = true;
    for (const button of modeButtons) button.disabled = true;
  };
}
