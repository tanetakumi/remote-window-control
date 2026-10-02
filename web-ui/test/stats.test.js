import test from "node:test";
import assert from "node:assert/strict";
import { intervalReport, readConnectionStats } from "../src/core/stats.js";

test("video counters and the selected pair's round-trip time are read; missing ones are left out", () => {
  const report = new Map([
    ["v", { type: "inbound-rtp", kind: "video", bytesReceived: 1000, packetsLost: 2, pliCount: 1, jitterBufferDelay: 0.5, jitterBufferEmittedCount: 10 }],
    ["a", { type: "inbound-rtp", kind: "audio", bytesReceived: 99 }],
    ["p1", { type: "candidate-pair", nominated: true, state: "succeeded", currentRoundTripTime: 0.12 }],
    ["p2", { type: "candidate-pair", nominated: false, state: "failed", currentRoundTripTime: 9 }],
  ]);
  const { counters, rtt } = readConnectionStats(report);
  assert.deepEqual(counters, { bytesReceived: 1000, packetsLost: 2, pliCount: 1, jitterBufferDelay: 0.5, jitterBufferEmittedCount: 10 });
  assert.equal(rtt, 0.12);
});

test("an interval report holds differences, rates and averages", () => {
  const previous = { time: 0, counters: { bytesReceived: 1000, packetsLost: 2, jitterBufferDelay: 0.5, jitterBufferEmittedCount: 10, inputQueuedMax: 1 } };
  const current = { time: 2000, rtt: 0.1, counters: { bytesReceived: 26000, packetsLost: 3, jitterBufferDelay: 1.5, jitterBufferEmittedCount: 30, inputQueuedMax: 4, framesDropped: 1 } };
  assert.deepEqual(intervalReport(previous, current), {
    intervalMs: 2000, bytesReceived: 25000, packetsLost: 1, jitterBufferEmittedCount: 20,
    inputQueuedMax: 4, kbps: 100, jitterBufferMs: 50, rttMs: 100,
  });
});

test("the jitter buffer delay is not computed without emitted frames", () => {
  const sample = { time: 0, counters: { jitterBufferDelay: 1, jitterBufferEmittedCount: 5 } };
  const report = intervalReport(sample, { ...sample, time: 1000 });
  assert.equal(report.jitterBufferMs, undefined);
});

test("a missing jitter delay is left out even when emitted frames are counted", () => {
  const previous = { time: 0, counters: { jitterBufferEmittedCount: 0 } };
  const current = { time: 1000, counters: { jitterBufferEmittedCount: 5 } };
  const report = intervalReport(previous, current);
  assert.equal(report.jitterBufferMs, undefined);
  assert.deepEqual(JSON.parse(JSON.stringify(report)), { intervalMs: 1000, jitterBufferEmittedCount: 5 });
});

test("the transport's selected pair takes precedence over old nominated pairs", () => {
  const report = new Map([
    ["transport", { type: "transport", selectedCandidatePairId: "current" }],
    ["current", { type: "candidate-pair", nominated: true, state: "succeeded", currentRoundTripTime: 0.1 }],
    ["old", { type: "candidate-pair", nominated: true, state: "succeeded", currentRoundTripTime: 0.9 }],
  ]);
  assert.equal(readConnectionStats(report).rtt, 0.1);
});
