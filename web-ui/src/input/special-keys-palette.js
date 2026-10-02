import { createListenerTracker } from "../lib/events.js";

const MARGIN = 8;
const MOVE_STEP = 10;
const clamp = (value, min, max) => Math.min(max, Math.max(min, value));

export function attachSpecialKeysPalette({
  buttonElement, paletteElement, stageElement, moveHandle, keyButtons,
}, sendControl) {
  const view = stageElement.ownerDocument.defaultView;
  const { listen, cleanup: removeListeners } = createListenerTracker();
  let opened = false;
  let textInputActive = false;
  let disposed = false;
  let position = null;
  let drag = null;
  let sending = false;
  const keyPointers = new Map();

  const isVisible = () => !disposed && opened && !textInputActive;
  const getBounds = () => {
    const stage = stageElement.getBoundingClientRect();
    const viewport = view.visualViewport;
    const left = viewport?.offsetLeft ?? 0;
    const top = viewport?.offsetTop ?? 0;
    const right = left + (viewport?.width ?? view.innerWidth);
    const bottom = top + (viewport?.height ?? view.innerHeight);
    const style = view.getComputedStyle(paletteElement);
    const safe = (side) => parseFloat(style.getPropertyValue(`--palette-safe-${side}`)) || 0;
    const minX = Math.max(0, left - stage.left) + MARGIN + safe("left");
    const minY = Math.max(0, top - stage.top) + MARGIN;
    return {
      minX,
      minY,
      maxX: Math.max(minX, Math.min(stage.width, right - stage.left)
        - MARGIN - safe("right") - paletteElement.offsetWidth),
      maxY: Math.max(minY, Math.min(stage.height, bottom - stage.top)
        - MARGIN - safe("bottom") - paletteElement.offsetHeight),
    };
  };
  const constrain = (point, bounds) => ({
    x: clamp(point.x, bounds.minX, bounds.maxX),
    y: clamp(point.y, bounds.minY, bounds.maxY),
  });
  const paintPosition = (point) => {
    paletteElement.style.transform = `translate3d(${point.x}px, ${point.y}px, 0)`;
  };
  const place = () => {
    if (!isVisible()) return;
    const bounds = getBounds();
    position ??= { x: bounds.maxX, y: bounds.minY };
    // Temporary viewport changes must not overwrite the user's saved position.
    const visiblePosition = constrain(position, bounds);
    paintPosition(visiblePosition);
    return visiblePosition;
  };
  const stopDragging = () => {
    if (!drag) return;
    const { pointerId } = drag;
    drag = null;
    paletteElement.classList.remove("dragging");
    if (moveHandle.hasPointerCapture(pointerId)) moveHandle.releasePointerCapture(pointerId);
  };
  const syncUi = () => {
    const visible = isVisible();
    if (!visible) {
      stopDragging();
      keyPointers.clear();
    }
    paletteElement.hidden = !visible;
    buttonElement.disabled = disposed;
    buttonElement.classList.toggle("active", visible);
    buttonElement.setAttribute("aria-expanded", String(visible));
    buttonElement.setAttribute("aria-pressed", String(visible));
    moveHandle.disabled = !visible;
    for (const { element } of keyButtons) element.disabled = !visible || sending;
    place();
  };

  listen(buttonElement, "click", () => {
    if (disposed || textInputActive) return;
    opened = !opened;
    syncUi();
  });
  // Presses the keys in order and releases them in reverse, so Shift wraps Enter.
  const pressKeys = (keys) => {
    for (const key of keys) {
      if (!sendControl({ type: "input.keyDown", key }) || disposed) return;
    }
    for (const key of [...keys].reverse()) {
      if (!sendControl({ type: "input.keyUp", key })) return;
    }
  };
  for (const { element, keys } of keyButtons) {
    listen(element, "pointerdown", (event) => {
      if (isVisible() && event.isPrimary && event.button === 0) {
        keyPointers.set(element, { pointerId: event.pointerId, cancelled: false });
      }
    });
    listen(element, "pointercancel", (event) => {
      const pointer = keyPointers.get(element);
      if (pointer?.pointerId === event.pointerId) pointer.cancelled = true;
    });
    listen(element, "pointerup", (event) => {
      const pointer = keyPointers.get(element);
      if (!pointer || pointer.pointerId !== event.pointerId) return;
      // Touch pointers implicitly capture the button, even when released outside it.
      const rect = element.getBoundingClientRect();
      pointer.cancelled ||= event.clientX < rect.left || event.clientX > rect.left + rect.width
        || event.clientY < rect.top || event.clientY > rect.top + rect.height;
    });
    listen(element, "click", (event) => {
      const cancelled = keyPointers.get(element)?.cancelled && event.detail !== 0;
      keyPointers.delete(element);
      if (cancelled || !isVisible() || sending) return;
      sending = true;
      for (const button of keyButtons) button.element.disabled = true;
      try {
        pressKeys(keys);
      } finally {
        sending = false;
        for (const button of keyButtons) button.element.disabled = !isVisible();
      }
    });
  }
  listen(moveHandle, "pointerdown", (event) => {
    if (!isVisible() || drag || !event.isPrimary || event.button !== 0) return;
    event.preventDefault();
    moveHandle.focus({ preventScroll: true });
    const point = place();
    drag = { pointerId: event.pointerId, lastX: event.clientX, lastY: event.clientY, ...point };
    moveHandle.setPointerCapture(event.pointerId);
    paletteElement.classList.add("dragging");
  });
  listen(moveHandle, "pointermove", (event) => {
    if (!isVisible() || !drag || drag.pointerId !== event.pointerId) return;
    event.preventDefault();
    position = constrain({
      x: drag.x + event.clientX - drag.lastX,
      y: drag.y + event.clientY - drag.lastY,
    }, getBounds());
    Object.assign(drag, position, { lastX: event.clientX, lastY: event.clientY });
    paintPosition(position);
  });
  for (const type of ["pointerup", "pointercancel", "lostpointercapture"]) {
    listen(moveHandle, type, (event) => {
      if (drag?.pointerId === event.pointerId) stopDragging();
    });
  }
  listen(moveHandle, "keydown", (event) => {
    if (!isVisible()) return;
    const delta = {
      ArrowLeft: [-MOVE_STEP, 0], ArrowRight: [MOVE_STEP, 0],
      ArrowUp: [0, -MOVE_STEP], ArrowDown: [0, MOVE_STEP],
    }[event.key];
    if (!delta) return;
    event.preventDefault();
    stopDragging();
    const point = place();
    position = constrain({ x: point.x + delta[0], y: point.y + delta[1] }, getBounds());
    paintPosition(position);
  });
  listen(paletteElement, "contextmenu", (event) => event.preventDefault());
  const refresh = () => { stopDragging(); place(); };
  listen(view, "resize", refresh);
  listen(view, "blur", stopDragging);
  if (view.visualViewport) {
    for (const type of ["resize", "scroll"]) listen(view.visualViewport, type, refresh);
  }
  const observer = view.ResizeObserver ? new view.ResizeObserver(refresh) : null;
  observer?.observe(stageElement);
  syncUi();

  return {
    setTextInputActive(active) {
      if (disposed) return;
      textInputActive = active;
      syncUi();
    },
    cleanup() {
      if (disposed) return;
      disposed = true;
      opened = false;
      stopDragging();
      removeListeners();
      observer?.disconnect();
      syncUi();
    },
  };
}
