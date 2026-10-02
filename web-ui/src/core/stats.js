// Receive statistics. Counters a browser does not provide are left out rather
// than reported as zero.
const VIDEO_COUNTERS = [
  "bytesReceived", "packetsLost", "packetsDiscarded", "framesDecoded", "framesDropped",
  "pliCount", "firCount", "nackCount", "jitterBufferDelay", "jitterBufferEmittedCount",
];

// readConnectionStats returns the cumulative video receive counters and the
// current round-trip time in seconds from an RTCStatsReport.
export function readConnectionStats(report) {
  const counters = {};
  let rtt;
  let selectedPairID;
  report.forEach((entry) => {
    if (entry.type === "inbound-rtp" && (entry.kind === "video" || entry.mediaType === "video")) {
      for (const key of VIDEO_COUNTERS) {
        if (typeof entry[key] === "number") counters[key] = (counters[key] ?? 0) + entry[key];
      }
    } else if (entry.type === "transport" && entry.selectedCandidatePairId) {
      selectedPairID = entry.selectedCandidatePairId;
    } else if (entry.type === "candidate-pair" && entry.nominated && entry.state === "succeeded"
      && typeof entry.currentRoundTripTime === "number") {
      rtt = entry.currentRoundTripTime;
    }
  });
  const selectedPair = report.get(selectedPairID);
  if (typeof selectedPair?.currentRoundTripTime === "number") rtt = selectedPair.currentRoundTripTime;
  return { counters, rtt };
}

// intervalReport turns two samples ({ time in ms, counters, rtt }) into the
// changes over the interval between them. Counters named *Max are maxima so
// far and are reported as they are.
export function intervalReport(previous, current) {
  const report = { intervalMs: Math.round(current.time - previous.time) };
  for (const [key, value] of Object.entries(current.counters)) {
    if (key.endsWith("Max")) report[key] = value;
    else if (typeof previous.counters[key] === "number") report[key] = value - previous.counters[key];
  }
  if (report.bytesReceived !== undefined && report.intervalMs > 0) report.kbps = report.bytesReceived * 8 / report.intervalMs;
  // Average time a frame waited in the jitter buffer; undefined without frames.
  if (report.jitterBufferEmittedCount > 0 && typeof report.jitterBufferDelay === "number") {
    report.jitterBufferMs = report.jitterBufferDelay / report.jitterBufferEmittedCount * 1000;
  }
  delete report.jitterBufferDelay;
  if (current.rtt !== undefined) report.rttMs = current.rtt * 1000;
  for (const key of Object.keys(report)) report[key] = Math.round(report[key] * 10) / 10;
  return report;
}
