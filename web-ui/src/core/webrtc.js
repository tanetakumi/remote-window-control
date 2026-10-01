function makeSignalingUrl() {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/ws`;
}

export function createRemoteConnection({ videoElement, onStatus, onBitrate, onDisconnect, signal }) {
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
    let useDataChannel = false;
    let statsTimer;
    let lastBytes;
    let lastTime;
    let messageChain = Promise.resolve();

    const status = (message) => onStatus?.(message);
    const timeout = window.setTimeout(() => fail(new Error("Connection timed out. Check host and media connectivity.")), 15000);
    const waitingTimer = window.setTimeout(() => { if (!closed && !hasTrack) status("Waiting for host video…"); }, 3000);

    function close() {
      if (closed) return;
      closed = true;
      window.clearTimeout(timeout);
      window.clearTimeout(waitingTimer);
      window.clearTimeout(statsTimer);
      videoElement.removeEventListener("loadeddata", onVideoFrame);
      signal?.removeEventListener("abort", onAbort);
      peer.ontrack = peer.onicecandidate = peer.onconnectionstatechange = null;
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
      status(error.message);
      // Reject with the actual failure before close's fallback rejection.
      if (!ready) reject(error);
      close();
      if (ready) onDisconnect?.(error);
    }

    async function pollBitrate() {
      try {
        const stats = await peer.getStats();
        if (closed) return;
        let bytes = 0;
        stats.forEach((report) => {
          if (report.type === "inbound-rtp" && (report.kind === "video" || report.mediaType === "video")) bytes += report.bytesReceived ?? 0;
        });
        const now = performance.now();
        if (lastTime !== undefined) onBitrate?.(Math.max(0, Math.round((bytes - lastBytes) * 8 / (now - lastTime))));
        lastBytes = bytes;
        lastTime = now;
      } catch { /* Closing a peer may interrupt getStats. */ }
      if (!closed) statsTimer = window.setTimeout(pollBitrate, 1000);
    }

    const remote = {
      close,
      sendControl(payload) {
        if (closed || !ready) return false;
        const serialized = JSON.stringify(payload);
        try {
          if (useDataChannel && inputChannel.readyState === "open" && inputChannel.bufferedAmount < 64 * 1024) {
            inputChannel.send(serialized);
            return true;
          }
          if (!useDataChannel && signaling.readyState === WebSocket.OPEN && signaling.bufferedAmount < 64 * 1024) {
            signaling.send(serialized);
            return true;
          }
          fail(new Error("Control connection is congested. Reconnect to continue."));
        } catch (error) { fail(error); }
        return false;
      },
    };

    function checkReady() {
      if (closed || ready || !hasTrack || !hasVideoFrame || peer.connectionState !== "connected") return;
      ready = true;
      // Keep one ordered control transport for the lifetime of the connection.
      useDataChannel = inputChannel.readyState === "open";
      window.clearTimeout(timeout);
      window.clearTimeout(waitingTimer);
      status("Control ready");
      if (onBitrate) pollBitrate();
      resolve(remote);
    }

    function onVideoFrame() { hasVideoFrame = true; checkReady(); }
    function onAbort() { fail(new Error("Connection cancelled.")); }
    videoElement.addEventListener("loadeddata", onVideoFrame);
    signal?.addEventListener("abort", onAbort, { once: true });
    inputChannel.addEventListener("close", () => { if (ready && useDataChannel) fail(new Error("Control channel closed.")); });
    peer.addTransceiver("video", { direction: "recvonly" });
    peer.ontrack = (event) => {
      if (closed) return;
      videoElement.srcObject = event.streams[0] ?? new MediaStream([event.track]);
      hasTrack = true;
      videoElement.play()?.catch(() => status("Tap the video to start playback."));
      checkReady();
    };
    peer.onconnectionstatechange = () => {
      if (["failed", "disconnected", "closed"].includes(peer.connectionState)) fail(new Error("Media connection lost. Select a window to reconnect."));
      else checkReady();
    };
    peer.onicecandidate = (event) => {
      if (!closed && event.candidate && signaling.readyState === WebSocket.OPEN) {
        try { signaling.send(JSON.stringify({ type: "webrtc.ice", candidate: event.candidate })); }
        catch (error) { fail(error); }
      }
    };
    signaling.addEventListener("open", () => {
      (async () => {
        status("Signaling connected; waiting for video…");
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
        if (message.type === "input.error") { status(message.message); return; }
        if (message.type === "webrtc.answer") {
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
