import { createListenerTracker } from "../lib/events.js";
import { getVideoContentRect, normalizeClientPoint } from "./coordinates.js";

const BUTTONS = [["left", 1], ["right", 2]];
const PIXELS_PER_NOTCH = 100;
const LINES_PER_NOTCH = 3;

export function attachMouseControls(videoElement, sendControl) {
  const { listen, cleanup: removeListeners } = createListenerTracker();
  const held = new Set();
  let pointerId = null;
  let lastPoint = { x: 0.5, y: 0.5 };

  const releaseCapture = () => {
    const id = pointerId;
    pointerId = null;
    if (id !== null && videoElement.hasPointerCapture(id)) {
      videoElement.releasePointerCapture(id);
    }
  };

  const release = () => {
    for (const button of held) {
      held.delete(button);
      sendControl({ type: "input.mouseUp", button, ...lastPoint });
    }
    releaseCapture();
  };

  const syncButtons = (buttons) => {
    // Chorded presses/releases arrive as pointermove, rather than additional
    // pointerdown/pointerup events. Track the full buttons bitmask.
    for (const [button, mask] of BUTTONS) {
      const down = Boolean(buttons & mask);
      if (down === held.has(button)) continue;
      if (down) held.add(button);
      else held.delete(button);
      sendControl({ type: down ? "input.mouseDown" : "input.mouseUp", button, ...lastPoint });
    }
  };

  listen(videoElement, "pointerdown", (event) => {
    if (event.pointerType !== "mouse" || !(event.buttons & 3)) return;
    const point = normalizeClientPoint(event, videoElement, false);
    if (!point) return;
    event.preventDefault();
    lastPoint = point;
    pointerId = event.pointerId;
    videoElement.setPointerCapture(pointerId);
    syncButtons(event.buttons);
  });

  listen(videoElement, "pointermove", (event) => {
    if (event.pointerType !== "mouse") return;
    const active = pointerId === event.pointerId;
    // Do not start a drag when a button was pressed elsewhere or after blur.
    if (!active && event.buttons) return;
    const point = normalizeClientPoint(event, videoElement, active);
    if (!point) return;
    lastPoint = point;
    if (active) {
      event.preventDefault();
      syncButtons(event.buttons);
      if (!event.buttons) releaseCapture();
    }
    sendControl({ type: "input.mouseMove", ...point });
  });

  listen(videoElement, "pointerup", (event) => {
    if (event.pointerType !== "mouse" || pointerId !== event.pointerId) return;
    event.preventDefault();
    lastPoint = normalizeClientPoint(event, videoElement) ?? lastPoint;
    syncButtons(event.buttons);
    if (!event.buttons) releaseCapture();
  });

  for (const type of ["pointercancel", "lostpointercapture"]) {
    listen(videoElement, type, (event) => {
      if (pointerId === event.pointerId) release();
    });
  }
  listen(videoElement.ownerDocument.defaultView, "blur", release);
  listen(videoElement, "contextmenu", (event) => event.preventDefault());
  listen(videoElement, "dragstart", (event) => event.preventDefault());
  listen(videoElement, "wheel", (event) => {
    // Ctrl+wheel also represents trackpad pinch zoom; keep browser zoom local.
    if (event.ctrlKey || !event.deltaY) return;
    const point = normalizeClientPoint(event, videoElement, false);
    if (!point) return;
    event.preventDefault();
    const unit = event.deltaMode === 1 ? 1 / LINES_PER_NOTCH
      : event.deltaMode === 2 ? getVideoContentRect(videoElement).height / PIXELS_PER_NOTCH
      : 1 / PIXELS_PER_NOTCH;
    sendControl({ type: "input.scroll", deltaY: event.deltaY * unit, ...point });
  }, { passive: false });

  return () => {
    release();
    removeListeners();
  };
}
