// Sends input commands on the control data channel without letting a slow
// connection pile them up. Pointer moves are merged into the newest position
// and normally sent once per MOVE_INTERVAL_MS; scroll steps at one position
// are added together. Every other command keeps its order in a queue that
// waits while the channel's send buffer is full. Input that cannot be
// delivered in time ends the connection, so a stale click or button release
// is never applied late; the host releases held input when it disconnects.
const MOVE_INTERVAL_MS = 40;
const HIGH_WATER_BYTES = 64 * 1024;
const LOW_WATER_BYTES = 16 * 1024;
const MAX_QUEUED = 256;
const MAX_QUEUE_AGE_MS = 5000;
// How long input may be held back before it is reported as delayed.
const CONGESTION_NOTICE_MS = 250;

const mergeable = (type) => type === "input.mouseMove" || type === "input.scroll";

export function createInputSender(channel, {
  onCongestion, onFail,
  now = () => performance.now(),
  setTimer = (fn, ms) => window.setTimeout(fn, ms),
  clearTimer = (id) => window.clearTimeout(id),
} = {}) {
  let pending = null; // a move or scroll not yet sent; later ones merge into it
  const queue = []; // { command, at }
  let lastPointer = -Infinity;
  let blockedSince = null;
  let congested = false;
  let timer = null;
  let closed = false;
  const stats = { inputSent: 0, inputMerged: 0, inputQueuedMax: 0 };

  channel.bufferedAmountLowThreshold = LOW_WATER_BYTES;
  channel.addEventListener("bufferedamountlow", flush);

  const hasRoom = () => channel.bufferedAmount < HIGH_WATER_BYTES;
  const transmit = (command) => { channel.send(JSON.stringify(command)); stats.inputSent++; };
  const enqueue = (command) => {
    queue.push({ command, at: now() });
    stats.inputQueuedMax = Math.max(stats.inputQueuedMax, queue.length);
  };
  const schedule = (ms) => { timer = setTimer(flush, ms); };

  function setCongested(value) {
    if (congested === value) return;
    congested = value;
    onCongestion?.(value);
  }

  function blocked() {
    blockedSince ??= now();
    if (now() - blockedSince >= CONGESTION_NOTICE_MS) setCongested(true);
    // The low-water event normally comes first; this also ages the queue.
    schedule(CONGESTION_NOTICE_MS);
  }

  function flush() {
    if (closed) return;
    if (timer !== null) clearTimer(timer);
    timer = null;
    // Check before draining: even a low-water event must not send stale input.
    const time = now();
    if (queue.length > MAX_QUEUED
      || (queue.length && time - queue[0].at >= MAX_QUEUE_AGE_MS)
      || (blockedSince !== null && time - blockedSince >= MAX_QUEUE_AGE_MS)) {
      fail(new Error("Input could not be delivered in time. Reconnect to continue."));
      return;
    }
    try {
      while (queue.length && hasRoom()) transmit(queue.shift().command);
      if (queue.length || (pending && !hasRoom())) { blocked(); return; }
      blockedSince = null;
      setCongested(false);
      if (!pending) return;
      const wait = lastPointer + MOVE_INTERVAL_MS - now();
      if (wait > 0) { schedule(wait); return; }
      transmit(pending);
      pending = null;
      lastPointer = now();
    } catch (error) { fail(error); }
  }

  function fail(error) {
    if (closed) return;
    close();
    onFail?.(error);
  }

  function close() {
    closed = true;
    pending = null;
    queue.length = 0;
    if (timer !== null) clearTimer(timer);
    timer = null;
    channel.removeEventListener("bufferedamountlow", flush);
  }

  return {
    stats,
    close,
    // send returns false once the sender has failed or been closed.
    send(command) {
      if (closed) return false;
      const samePlace = pending && pending.type === command.type
        && (command.type === "input.mouseMove" || (pending.x === command.x && pending.y === command.y));
      if (mergeable(command.type) && samePlace) {
        pending = command.type === "input.scroll" ? { ...command, deltaY: pending.deltaY + command.deltaY } : command;
        stats.inputMerged++;
      } else {
        // Anything else first sends what is pending, so it applies at the newest position.
        if (pending) enqueue(pending);
        pending = null;
        if (mergeable(command.type)) pending = command;
        else enqueue(command);
      }
      flush();
      return !closed;
    },
  };
}
