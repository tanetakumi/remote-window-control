import { createListenerTracker } from "../lib/events.js";
import { getVideoContentRect, normalizeClientPoint } from "./coordinates.js";

export const TOUCH_MODES = Object.freeze({ DIRECT: "direct", RELATIVE: "relative" });
export const DEFAULT_TOUCH_MODE = TOUCH_MODES.RELATIVE;

const LONG_PRESS_MS = 450;
const DOUBLE_TAP_MS = 300;
const TAP_SLOP_PX = 8;
const DOUBLE_TAP_SLOP_PX = 24;
const PIXELS_PER_NOTCH = 100;
const WHEEL_UNITS_PER_NOTCH = 120;

const clientPoint = (touch) => ({ x: touch.clientX, y: touch.clientY });
const distance = (a, b) => Math.hypot(a.x - b.x, a.y - b.y);
const center = ([a, b]) => ({ x: (a.clientX + b.clientX) / 2, y: (a.clientY + b.clientY) / 2 });
const separation = ([a, b]) => distance(clientPoint(a), clientPoint(b));
const clamp = (value) => Math.min(1, Math.max(0, value));
// Contacts on the mode selector or keyboard must not keep a video gesture alive.
const contacts = (event) => Array.from(event.targetTouches ?? event.touches);

// Touch gestures share one logical cursor. Both modes use the existing window
// coordinate protocol; there are no physical mouse or wheel event handlers.
export function attachGestureControls(videoElement, sendControl, options = {}) {
  const view = videoElement.ownerDocument.defaultView;
  const doc = videoElement.ownerDocument;
  const { listen, cleanup: removeListeners } = createListenerTracker();
  const onCursorChange = options.onCursorChange ?? (() => {});
  let mode = options.mode ?? DEFAULT_TOUCH_MODE;
  let cursor = { x: 0.5, y: 0.5 };
  let gesture = null;
  let lastTap = null;
  let heldButton = null;
  let longPressTimer = null;
  let scrollRemainder = 0;
  let disposed = false;

  const updateCursor = (point, sendMove = false) => {
    cursor = { ...point };
    onCursorChange({ ...cursor });
    if (sendMove) sendControl({ type: "input.mouseMove", ...cursor });
  };

  const cancelLongPress = () => {
    if (longPressTimer !== null) view.clearTimeout(longPressTimer);
    longPressTimer = null;
  };

  const releaseButton = () => {
    if (!heldButton) return;
    const button = heldButton;
    heldButton = null;
    sendControl({ type: "input.mouseUp", button, ...cursor });
  };

  const stopGesture = (blockContacts) => {
    cancelLongPress();
    // Ignore remaining contacts until all fingers lift after a mode change,
    // blur, cancellation, or an unsupported gesture.
    gesture = blockContacts ? { kind: "blocked" } : null;
    lastTap = null;
    scrollRemainder = 0;
    releaseButton();
  };

  const cancel = () => stopGesture(gesture !== null);

  const moveCursor = (touch, active) => {
    const point = clientPoint(touch);
    if (mode === TOUCH_MODES.DIRECT) {
      const normalized = normalizeClientPoint(touch, videoElement);
      if (normalized) updateCursor(normalized, true);
    } else {
      const rect = getVideoContentRect(videoElement);
      if (rect.width > 0 && rect.height > 0) {
        updateCursor({
          x: clamp(cursor.x + (point.x - active.last.x) / rect.width),
          y: clamp(cursor.y + (point.y - active.last.y) / rect.height),
        }, true);
      }
    }
    active.last = point;
  };

  const scroll = (pixels) => {
    scrollRemainder += pixels * WHEEL_UNITS_PER_NOTCH / PIXELS_PER_NOTCH;
    const units = Math.trunc(scrollRemainder);
    if (!units) return;
    scrollRemainder -= units;
    sendControl({ type: "input.scroll", deltaY: units / WHEEL_UNITS_PER_NOTCH, ...cursor });
  };

  const updateTwoFingerTravel = (touches, active) => {
    for (const touch of touches) {
      const start = active.starts.get(touch.identifier);
      if (!start || distance(start, clientPoint(touch)) > TAP_SLOP_PX) active.moved = true;
    }
  };

  listen(videoElement, "touchstart", (event) => {
    event.preventDefault();
    if (disposed || gesture?.kind === "blocked") return;
    const touches = contacts(event);
    if (touches.length > 2) { stopGesture(true); return; }

    if (touches.length === 1 && !gesture) {
      const touch = touches[0];
      const point = normalizeClientPoint(touch, videoElement, false);
      if (!point) { stopGesture(true); return; }
      const start = clientPoint(touch);
      const now = Date.now();
      const doubleTap = lastTap && now - lastTap.time <= DOUBLE_TAP_MS
        && distance(start, lastTap.client) <= DOUBLE_TAP_SLOP_PX;
      lastTap = null;
      scrollRemainder = 0;
      if (mode === TOUCH_MODES.DIRECT) updateCursor(point);
      const active = gesture = {
        kind: "single", id: touch.identifier, start, last: start,
        startedAt: now, moved: false, longPressed: false, dragging: Boolean(doubleTap),
      };
      if (doubleTap) {
        heldButton = "left";
        sendControl({ type: "input.mouseDown", button: "left", ...cursor });
      } else {
        longPressTimer = view.setTimeout(() => {
          longPressTimer = null;
          if (disposed || gesture !== active || active.moved) return;
          active.longPressed = true;
          sendControl({ type: "input.tap", button: "right", ...cursor });
        }, LONG_PRESS_MS);
      }
      return;
    }

    if (touches.length === 2 && gesture?.kind !== "two") {
      if (touches.some((touch) => !normalizeClientPoint(touch, videoElement, false))) {
        stopGesture(true);
        return;
      }
      const tapAllowed = !gesture || (!gesture.moved && !gesture.dragging && !gesture.longPressed);
      const startedAt = gesture?.startedAt ?? Date.now();
      cancelLongPress();
      lastTap = null;
      scrollRemainder = 0;
      gesture = {
        kind: "two", starts: new Map(touches.map((t) => [t.identifier, clientPoint(t)])),
        startCenter: center(touches), lastCenter: center(touches),
        startSeparation: separation(touches), startedAt, moved: false,
        scrolling: false, pinching: false, ending: false, tapAllowed,
      };
      releaseButton();
      // A failed button-release send can dispose the connection synchronously.
      if (!disposed && mode === TOUCH_MODES.DIRECT) {
        const point = center(touches);
        updateCursor(normalizeClientPoint({ clientX: point.x, clientY: point.y }, videoElement));
      }
    }
  }, { passive: false });

  listen(videoElement, "touchmove", (event) => {
    event.preventDefault();
    if (disposed || !gesture || gesture.kind === "blocked") return;
    const active = gesture;
    const touches = contacts(event);
    if (touches.length > 2) { stopGesture(true); return; }

    if (active.kind === "single" && touches.length === 1) {
      const touch = touches.find((t) => t.identifier === active.id);
      if (!touch || (active.longPressed && mode === TOUCH_MODES.DIRECT)) return;
      const point = clientPoint(touch);
      if (distance(active.start, point) > TAP_SLOP_PX) {
        active.moved = true;
        cancelLongPress();
      }
      if (active.dragging || mode === TOUCH_MODES.RELATIVE) {
        moveCursor(touch, active);
      } else if (active.moved) {
        scroll(active.last.y - point.y);
        active.last = point;
      }
      return;
    }

    if (active.kind === "two" && touches.length === 2 && !active.ending) {
      updateTwoFingerTravel(touches, active);
      const current = center(touches);
      // A pinch is not a remote wheel gesture. Suppress it for this contact
      // sequence once the finger separation changes beyond tap tolerance.
      if (Math.abs(separation(touches) - active.startSeparation) > TAP_SLOP_PX) {
        active.pinching = true;
        active.moved = true;
      }
      if (!active.pinching && (active.scrolling || distance(current, active.startCenter) > TAP_SLOP_PX)) {
        active.scrolling = true;
        active.moved = true;
        scroll(active.lastCenter.y - current.y);
        active.lastCenter = current;
      }
    }
  }, { passive: false });

  listen(videoElement, "touchend", (event) => {
    event.preventDefault();
    if (disposed || !gesture) return;
    const active = gesture;
    if (active.kind === "two") {
      updateTwoFingerTravel(Array.from(event.changedTouches), active);
      active.ending = true;
    }
    if (contacts(event).length > 0) return;
    cancelLongPress();
    // Clear the gesture before sending input, since a failed send may dispose
    // the connection and run cleanup synchronously.
    gesture = null;
    if (active.kind === "single") {
      const touch = Array.from(event.changedTouches).find((t) => t.identifier === active.id);
      if (touch) {
        if (distance(active.start, clientPoint(touch)) > TAP_SLOP_PX) active.moved = true;
        if (active.dragging || mode === TOUCH_MODES.RELATIVE) {
          if (distance(active.last, clientPoint(touch)) > 0) moveCursor(touch, active);
        } else if (mode === TOUCH_MODES.DIRECT && !active.moved && !active.longPressed) {
          const point = normalizeClientPoint(touch, videoElement);
          if (point) updateCursor(point);
        }
      }
      if (active.dragging) {
        releaseButton();
      } else if (touch && !active.moved && !active.longPressed) {
        lastTap = { time: Date.now(), client: clientPoint(touch) };
        sendControl({ type: "input.tap", button: "left", ...cursor });
      }
    } else if (active.kind === "two" && active.tapAllowed && !active.moved
      && Date.now() - active.startedAt <= DOUBLE_TAP_MS) {
      sendControl({ type: "input.tap", button: "right", ...cursor });
    }
    scrollRemainder = 0;
  }, { passive: false });

  listen(videoElement, "touchcancel", (event) => {
    cancel();
    if (contacts(event).length === 0) gesture = null;
  });
  listen(view, "blur", cancel);
  listen(view, "pagehide", cancel);
  listen(doc, "visibilitychange", () => { if (doc.hidden) cancel(); });
  listen(videoElement, "contextmenu", (event) => event.preventDefault());
  listen(videoElement, "dragstart", (event) => event.preventDefault());
  onCursorChange({ ...cursor });

  return {
    setMode(nextMode) {
      if (disposed || !Object.values(TOUCH_MODES).includes(nextMode) || nextMode === mode) return;
      cancel();
      mode = nextMode;
      onCursorChange({ ...cursor });
    },
    cleanup() {
      if (disposed) return;
      disposed = true;
      cancel();
      gesture = null;
      removeListeners();
    },
  };
}
