import test from "node:test";
import assert from "node:assert/strict";
import { createInputSender } from "../src/core/input-sender.js";

class Channel extends EventTarget {
  bufferedAmount = 0;
  sent = [];
  send(data) { this.sent.push(JSON.parse(data)); }
}

// A sender with a manual clock: advance() runs timers that come due.
function setup() {
  const channel = new Channel();
  let time = 0;
  let timers = [];
  const congestion = [], failures = [];
  const sender = createInputSender(channel, {
    now: () => time,
    setTimer: (fn, ms) => { const timer = { fn, at: time + ms }; timers.push(timer); return timer; },
    clearTimer: (timer) => { timers = timers.filter((t) => t !== timer); },
    onCongestion: (value) => congestion.push(value),
    onFail: (error) => failures.push(error.message),
  });
  const advance = (ms) => {
    const end = time + ms;
    for (;;) {
      const due = timers.filter((t) => t.at <= end).sort((a, b) => a.at - b.at)[0];
      if (!due) break;
      timers = timers.filter((t) => t !== due);
      time = due.at;
      due.fn();
    }
    time = end;
  };
  return { channel, sender, advance, elapse: (ms) => { time += ms; }, congestion, failures };
}

const move = (x) => ({ type: "input.mouseMove", x, y: 0.5 });

test("pointer moves are merged into the newest position at a limited rate", () => {
  const { channel, sender, advance } = setup();
  assert.equal(sender.send(move(0.1)), true);
  for (const x of [0.2, 0.3, 0.4]) sender.send(move(x));
  assert.deepEqual(channel.sent.map((c) => c.x), [0.1]);
  advance(40);
  assert.deepEqual(channel.sent.map((c) => c.x), [0.1, 0.4]);
  assert.equal(sender.stats.inputMerged, 2);
});

test("other commands flush the pending move first and keep their order", () => {
  const { channel, sender } = setup();
  sender.send(move(0.1));
  sender.send(move(0.2));
  sender.send({ type: "input.mouseDown", button: "left", x: 0.2, y: 0.5 });
  sender.send({ type: "input.keyDown", key: "Enter" });
  assert.deepEqual(channel.sent.map((c) => c.type + (c.x ?? "")), [
    "input.mouseMove0.1", "input.mouseMove0.2", "input.mouseDown0.2", "input.keyDown",
  ]);
});

test("scroll steps at one position add up; a new position starts a new scroll", () => {
  const { channel, sender, advance } = setup();
  const scroll = (deltaY, x = 0.5) => ({ type: "input.scroll", deltaY, x, y: 0.5 });
  sender.send(scroll(1));
  sender.send(scroll(0.5));
  sender.send(scroll(0.25));
  sender.send(scroll(1, 0.7));
  advance(100);
  assert.deepEqual(channel.sent, [scroll(1), scroll(0.75), scroll(1, 0.7)]);
});

test("a full send buffer holds commands until it drains, and long waits are reported", () => {
  const { channel, sender, advance, congestion } = setup();
  channel.bufferedAmount = 64 * 1024;
  sender.send({ type: "input.tap", button: "left", x: 0.5, y: 0.5 });
  sender.send(move(0.3));
  assert.equal(channel.sent.length, 0);
  advance(100);
  assert.deepEqual(congestion, [], "a brief hold is not reported");
  advance(300);
  assert.deepEqual(congestion, [true]);
  channel.bufferedAmount = 0;
  channel.dispatchEvent(new Event("bufferedamountlow"));
  assert.deepEqual(channel.sent.map((c) => c.type), ["input.tap", "input.mouseMove"]);
  assert.deepEqual(congestion, [true, false]);
  assert.equal(sender.stats.inputQueuedMax, 1);
});

test("input that cannot be delivered in time fails instead of applying late", () => {
  const { channel, sender, advance, failures } = setup();
  channel.bufferedAmount = 1 << 20;
  sender.send({ type: "input.mouseDown", button: "left", x: 0.5, y: 0.5 });
  advance(4900);
  assert.deepEqual(failures, []);
  advance(400);
  assert.deepEqual(failures, ["Input could not be delivered in time. Reconnect to continue."]);
  assert.equal(sender.send(move(0.1)), false);
  channel.bufferedAmount = 0;
  channel.dispatchEvent(new Event("bufferedamountlow"));
  assert.equal(channel.sent.length, 0, "the stale command is never sent");
});

test("a send error fails the sender", () => {
  const { channel, sender, failures } = setup();
  channel.send = () => { throw new Error("channel closed"); };
  assert.equal(sender.send({ type: "input.tap" }), false);
  assert.deepEqual(failures, ["channel closed"]);
});

test("a delayed low-water event cannot release an expired click", () => {
  const { channel, sender, advance, elapse, failures } = setup();
  channel.bufferedAmount = 64 * 1024;
  sender.send({ type: "input.tap", button: "left", x: 0.5, y: 0.5 });
  advance(4900);
  // A background tab can delay the timer while a channel event arrives first.
  elapse(200);
  channel.bufferedAmount = 0;
  channel.dispatchEvent(new Event("bufferedamountlow"));
  assert.equal(channel.sent.length, 0);
  assert.equal(failures.length, 1);
  assert.equal(sender.send(move(0.1)), false);
});

test("a pointer-only backlog also expires", () => {
  const { channel, sender, advance, failures } = setup();
  channel.bufferedAmount = 64 * 1024;
  sender.send(move(0.1));
  advance(3000);
  sender.send(move(0.2));
  advance(2000);
  assert.equal(failures.length, 1);
  channel.bufferedAmount = 0;
  channel.dispatchEvent(new Event("bufferedamountlow"));
  assert.equal(channel.sent.length, 0);
});

test("a full command queue fails before delivering any queued command", () => {
  const { channel, sender, failures } = setup();
  channel.bufferedAmount = 64 * 1024;
  for (let i = 0; i < 256; i++) assert.equal(sender.send({ type: "input.keyDown", key: "Enter" }), true);
  assert.equal(sender.send({ type: "input.keyUp", key: "Enter" }), false);
  assert.equal(failures.length, 1);
  channel.bufferedAmount = 0;
  channel.dispatchEvent(new Event("bufferedamountlow"));
  assert.equal(channel.sent.length, 0);
});
