export function getVideoContentRect(element) {
  const rect = element.getBoundingClientRect();
  const sourceWidth = element.videoWidth;
  const sourceHeight = element.videoHeight;

  if (!sourceWidth || !sourceHeight || !rect.width || !rect.height) {
    return rect;
  }

  const scale = Math.min(rect.width / sourceWidth, rect.height / sourceHeight);
  const width = sourceWidth * scale;
  const height = sourceHeight * scale;
  return {
    left: rect.left + (rect.width - width) / 2,
    top: rect.top + (rect.height - height) / 2,
    width,
    height,
  };
}

// New touch interactions must start inside the image. An existing drag can
// continue outside it, with coordinates clamped to the image edge.
export function normalizeClientPoint(point, element, clamp = true) {
  const rect = getVideoContentRect(element);
  if (rect.width <= 0 || rect.height <= 0) return null;
  const x = (point.clientX - rect.left) / rect.width;
  const y = (point.clientY - rect.top) / rect.height;
  if (!Number.isFinite(x) || !Number.isFinite(y)) return null;
  if (!clamp && (x < 0 || x > 1 || y < 0 || y > 1)) return null;
  return {
    x: Math.min(1, Math.max(0, x)),
    y: Math.min(1, Math.max(0, y)),
  };
}
