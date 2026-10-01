import { createListenerTracker } from "../lib/events.js";

function getViewportPayload(targetElement) {
  const stageRect = targetElement?.getBoundingClientRect();
  const viewport = window.visualViewport;

  const width = Math.round(stageRect?.width ?? viewport?.width ?? window.innerWidth);
  const height = Math.round(stageRect?.height ?? viewport?.height ?? window.innerHeight);

  return {
    type: "viewport.resize",
    width,
    height,
    devicePixelRatio: window.devicePixelRatio || 1,
  };
}

export function attachViewportSync(sendControl, options = {}) {
  let timerId = null;
  let disposed = false;
  let animationId;
  let orientationTimerIds = [];
  const isSuspended = options.isSuspended ?? (() => false);
  const targetElement = options.targetElement ?? null;
  let lastPayload = null;
  const { listen, cleanup: removeListeners } = createListenerTracker();

  const sendViewport = () => {
    if (disposed) return;
    const payload = getViewportPayload(targetElement);
    const keyboardLikeResize =
      isSuspended()
      && lastPayload
      && payload.width === lastPayload.width
      && payload.height !== lastPayload.height;

    if (keyboardLikeResize) {
      return;
    }

    if (
      lastPayload
      && payload.width === lastPayload.width
      && payload.height === lastPayload.height
      && payload.devicePixelRatio === lastPayload.devicePixelRatio
    ) {
      return;
    }

    lastPayload = payload;
    sendControl(payload);
  };

  const scheduleViewportSync = () => {
    if (timerId) {
      window.clearTimeout(timerId);
    }

    timerId = window.setTimeout(sendViewport, 120);
  };

  const scheduleOrientationSync = () => {
    lastPayload = null;
    scheduleViewportSync();
    for (const timeoutId of orientationTimerIds) {
      window.clearTimeout(timeoutId);
    }

    orientationTimerIds = [0, 100, 250, 500, 900, 1500].map((delayMs) =>
      window.setTimeout(() => sendViewport(), delayMs)
    );
  };

  const triggerFullscreenSync = () => {
    lastPayload = null;
    animationId = requestAnimationFrame(() => {
      if (disposed) return;
      scheduleViewportSync();
      scheduleOrientationSync();
    });
  };

  const orientationMedia = window.matchMedia?.("(orientation: portrait)");
  const onOrientationChange = () => scheduleOrientationSync();

  sendViewport();
  listen(window, "resize", scheduleViewportSync);
  listen(window, "orientationchange", onOrientationChange);
  if (orientationMedia?.addEventListener) listen(orientationMedia, "change", onOrientationChange);
  if (window.visualViewport) listen(window.visualViewport, "resize", scheduleViewportSync);
  listen(document, "fullscreenchange", triggerFullscreenSync);
  listen(document, "webkitfullscreenchange", triggerFullscreenSync);
  if (screen.orientation?.addEventListener) listen(screen.orientation, "change", onOrientationChange);

  return {
    triggerFullscreenSync,
    cleanup() {
      disposed = true;
      cancelAnimationFrame(animationId);
      removeListeners();
      if (timerId) {
        window.clearTimeout(timerId);
      }
      for (const timeoutId of orientationTimerIds) {
        window.clearTimeout(timeoutId);
      }
    },
  };
}
