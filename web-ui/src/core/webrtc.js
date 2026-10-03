import { createInputSender } from "./input-sender.js";
import { intervalReport, readConnectionStats } from "./stats.js";

// How long the connection may stay "disconnected" (as during a brief network
// change) before it is given up; the host waits as long.
const DISCONNECT_GRACE_MS = 5000;

function makeSignalingUrl() {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/ws`;
}

export function createRemoteConnection({ videoElement, onStatus, onNotice, onInputError, onBitrate, onDisconnect, onInputMode, signal, getInitialViewport }) {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) { reject(new Error("Connection cancelled.")); return; }
    const peer = new RTCPeerConnection();
    const signaling = new WebSocket(makeSignalingUrl());
    const inputChannel = peer.createDataChannel("control");
    const pendingICE = [];
    let closed = false;
    let ready = false;
    let hasTrack = false;
    let hasVideoFrame = false;
    let statsTimer;
    let graceTimer;
    let modeTimer;
    let inputMode = "window";
    let switching = false;
    let requestedMode;
    let statsIntervalMs = 0;
    let lastBitrate;
    let lastReport;
    let messageChain = Promise.resolve();
    // Conditions shown in the status once connected.
    const condition = { reconnecting: false, inputDelayed: false };

    const status = (state, message) => onStatus?.(state, message);
    const timeout = window.setTimeout(() => fail(new Error("Connection timed out. Check host and media connectivity.")), 15000);
    const waitingTimer = window.setTimeout(() => { if (!closed && !hasTrack) status("connecting", "Waiting for video…"); }, 3000);
    const input = createInputSender(inputChannel, {
      onCongestion(delayed) { condition.inputDelayed = delayed; renderStatus(); },
      onFail: (error) => fail(error),
    });

    function close() {
      if (closed) return;
      closed = true;
      window.clearTimeout(timeout);
      window.clearTimeout(waitingTimer);
      window.clearTimeout(statsTimer);
      window.clearTimeout(graceTimer);
      window.clearTimeout(modeTimer);
      videoElement.removeEventListener("loadeddata", onVideoFrame);
      signal?.removeEventListener("abort", onAbort);
      peer.ontrack = peer.onicecandidate = peer.onconnectionstatechange = null;
      input.close();
      inputChannel.close();
      peer.close();
      signaling.close();
      if (videoElement.srcObject) {
        videoElement.srcObject.getTracks().forEach((track) => track.stop());
        videoElement.srcObject = null;
      }
      if (!ready) reject(new Error("Connection closed before it was ready."));
    }

    function fail(error) {
      if (closed) return;
      status("error", "Connection lost");
      // Reject with the actual failure before close's fallback rejection.
      if (!ready) reject(error);
      close();
      if (ready) onDisconnect?.(error);
    }

    function renderStatus() {
      if (!ready || closed) return;
      if (condition.reconnecting) status("connecting", "Reconnecting…");
      else if (condition.inputDelayed) status("connecting", "Input delayed…");
      else if (switching) status("connecting", `Switching to ${requestedMode === "pc" ? "PC" : "Window"} mode…`);
      else status("connected", "Connected");
    }

    async function pollStats() {
      try {
        const sample = readConnectionStats(await peer.getStats());
        if (closed) return;
        Object.assign(sample.counters, input.stats);
        sample.time = performance.now();
        const bytes = sample.counters.bytesReceived ?? 0;
        if (lastBitrate) onBitrate?.(Math.max(0, Math.round((bytes - lastBitrate.bytes) * 8 / (sample.time - lastBitrate.time))));
        lastBitrate = { time: sample.time, bytes };
        if (statsIntervalMs > 0 && !lastReport) lastReport = sample;
        else if (statsIntervalMs > 0 && sample.time - lastReport.time >= statsIntervalMs) {
          if (signaling.readyState === WebSocket.OPEN) {
            signaling.send(JSON.stringify({ type: "client.stats", report: intervalReport(lastReport, sample) }));
          }
          lastReport = sample;
        }
      } catch { /* Closing a peer may interrupt getStats. */ }
      if (!closed) statsTimer = window.setTimeout(pollStats, 1000);
    }

    const remote = {
      close,
      get inputMode() { return inputMode; },
      setInputMode(mode) {
        if (closed || !ready || switching || !["window", "pc"].includes(mode)) return false;
        // The sender flushes pending moves/scrolls ahead of this request.
        if (!input.send({ type: "input.mode", mode })) return false;
        switching = true;
        requestedMode = mode;
        modeTimer = window.setTimeout(() => fail(new Error("Input mode switch timed out. Select a window to reconnect.")), 15000);
        onInputMode?.(inputMode, switching);
        renderStatus();
        return true;
      },
      sendControl(payload) {
        if (closed || !ready || switching) return false;
        return input.send(payload);
      },
    };

    function checkReady() {
      // Ready means input can be delivered: video is playing and the control channel is open.
      if (closed || ready || !hasTrack || !hasVideoFrame || peer.connectionState !== "connected" || inputChannel.readyState !== "open") return;
      ready = true;
      window.clearTimeout(timeout);
      window.clearTimeout(waitingTimer);
      renderStatus();
      if (onBitrate || statsIntervalMs > 0) pollStats();
      onInputMode?.(inputMode, switching);
      resolve(remote);
    }

    function onVideoFrame() { hasVideoFrame = true; checkReady(); }
    function onAbort() { fail(new Error("Connection cancelled.")); }
    videoElement.addEventListener("loadeddata", onVideoFrame);
    signal?.addEventListener("abort", onAbort, { once: true });
    inputChannel.addEventListener("open", () => {
      if (closed) return;
      // Resize as soon as input can be delivered, without waiting for video.
      try {
        const viewport = getInitialViewport?.();
        if (viewport?.width > 0 && viewport?.height > 0) {
          inputChannel.send(JSON.stringify({ type: "viewport.resize", width: viewport.width, height: viewport.height, devicePixelRatio: viewport.devicePixelRatio }));
        }
      } catch (error) { fail(error); return; }
      checkReady();
    });
    inputChannel.addEventListener("close", () => fail(new Error("Control channel closed.")));
    peer.addTransceiver("video", { direction: "recvonly" });
    peer.ontrack = (event) => {
      if (closed) return;
      videoElement.srcObject = event.streams[0] ?? new MediaStream([event.track]);
      hasTrack = true;
      videoElement.play()?.catch(() => { if (!closed) onNotice?.("Tap the video to start playback."); });
      checkReady();
    };
    peer.onconnectionstatechange = () => {
      const state = peer.connectionState;
      if (state === "failed" || state === "closed") {
        fail(new Error("Media connection lost. Select a window to reconnect."));
        return;
      }
      // A brief network change passes through "disconnected"; give it time.
      condition.reconnecting = state === "disconnected";
      if (condition.reconnecting) {
        graceTimer ??= window.setTimeout(() => fail(new Error("Media connection lost. Select a window to reconnect.")), DISCONNECT_GRACE_MS);
      } else {
        window.clearTimeout(graceTimer);
        graceTimer = undefined;
      }
      renderStatus();
      checkReady();
    };
    peer.onicecandidate = (event) => {
      if (!closed && event.candidate && signaling.readyState === WebSocket.OPEN) {
        try { signaling.send(JSON.stringify({ type: "webrtc.ice", candidate: event.candidate })); }
        catch (error) { fail(error); }
      }
    };
    signaling.addEventListener("open", () => {
      (async () => {
        status("connecting", "Waiting for video…");
        const offer = await peer.createOffer();
        if (closed) return;
        await peer.setLocalDescription(offer);
        if (!closed) signaling.send(JSON.stringify({ type: "webrtc.offer", sdp: offer.sdp }));
      })().catch(fail);
    });
    signaling.addEventListener("message", (event) => {
      messageChain = messageChain.then(async () => {
        if (closed) return;
        const message = JSON.parse(event.data);
        if (message.type === "error") throw new Error(message.message || "Host connection failed.");
        if (message.type === "input.error") { onInputError?.(message.message || "The host could not apply the input."); return; }
        if (message.type === "input.mode") {
          if (!["window", "pc"].includes(message.mode) || (switching && message.mode !== requestedMode)) {
            throw new Error("Invalid input mode confirmation from host.");
          }
          window.clearTimeout(modeTimer);
          inputMode = message.mode;
          switching = false;
          requestedMode = undefined;
          onInputMode?.(inputMode, switching);
          renderStatus();
        } else if (message.type === "session.config") {
          statsIntervalMs = message.statsIntervalMs > 0 ? message.statsIntervalMs : 0;
        } else if (message.type === "webrtc.answer") {
          await peer.setRemoteDescription({ type: "answer", sdp: message.sdp });
          for (const candidate of pendingICE.splice(0)) await peer.addIceCandidate(candidate);
        } else if (message.type === "webrtc.ice" && message.candidate) {
          if (peer.remoteDescription) await peer.addIceCandidate(message.candidate);
          else {
            if (pendingICE.length >= 128) throw new Error("Too many pending ICE candidates.");
            pendingICE.push(message.candidate);
          }
        }
      }).catch(fail);
    });
    signaling.addEventListener("error", () => fail(new Error("Signaling connection failed. Check the host address.")));
    signaling.addEventListener("close", () => fail(new Error("Connection closed. Select a window to reconnect.")));
  });
}
