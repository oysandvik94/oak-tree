import net from "node:net";
import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import { randomBytes } from "node:crypto";

function createOfficeQuestionBridge(pi, getIdentity) {
  const flows = new Map();
  const pending = new Map();
  let server;
  let socketPath;
  let starting;
  let generation;
  let failureCount = 0;
  let latestAnswer = "";
  const answerText = (message) => {
    if (message?.role !== "assistant" || message.stopReason !== "stop" || !Array.isArray(message.content)) return "";
    const text = message.content.filter((part) => part?.type === "text" && typeof part.text === "string").map((part) => part.text).join("\n").trim();
    return text.length > 8000 ? text.slice(0, 8000) + "\n\n[Answer truncated]" : text;
  };
  pi.on?.("session_start", (_event, ctx) => {
    latestAnswer = "";
    for (const entry of ctx.sessionManager.getBranch()) {
      const text = entry?.type === "message" ? answerText(entry.message) : "";
      if (text) latestAnswer = text;
    }
  });
  pi.on?.("message_end", (event) => {
    const text = answerText(event.message);
    if (text) latestAnswer = text;
  });
  const nonce = () => randomBytes(24).toString("hex");
  const resultChannel = "@eko24ive/pi-ask:submit-result";

  pi.events.on("@eko24ive/pi-ask:started", (event) => {
    const identity = getIdentity();
    const bounded = (value, max) => typeof value === "string" && value.length > 0 && value.length <= max;
    if (event?.version !== 1 || !["tool", "answer", "answer:again", "ask:replay", "ask:resume"].includes(event.source) || !bounded(event.flowId, 1024) || !bounded(identity?.session_id, 1024) || !bounded(identity?.tmux_pane, 128) || (identity.session_file && !bounded(identity.session_file, 4096)) || (event.toolCallId !== undefined && !bounded(event.toolCallId, 1024)) || (event.title !== undefined && (typeof event.title !== "string" || event.title.length > 4096)) || !Number.isFinite(event.createdAt) || !Array.isArray(event.questions)) return;
    for (const [id, flow] of flows) if (flow.flowId === event.flowId) flows.delete(id);
    for (const [requestId, entry] of pending) {
      if (entry.flowId !== event.flowId) continue;
      pending.delete(requestId); clearTimeout(entry.timer); entry.resolve({ ok: false, error: "uncertain" });
    }
    if (flows.size >= 16) return;
    const id = nonce();
    const flow = { id, flowId: event.flowId, toolCallId: event.toolCallId, title: event.title || "", source: event.source, createdAt: event.createdAt, questions: event.questions, busy: false };
    try { if (Buffer.byteLength(JSON.stringify([...flows.values(), flow])) > 196608) return; } catch { return; }
    flows.set(id, flow);
  });
  pi.events.on("@eko24ive/pi-ask:completed", (event) => {
    if (event?.version !== 1) return;
    for (const [id, flow] of flows) if (flow.flowId === event.flowId) flows.delete(id);
  });
  pi.events.on(resultChannel, (event) => {
    const waiting = pending.get(event?.requestId);
    if (event?.version !== 1 || !waiting || event.flowId !== waiting.flowId) return;
    pending.delete(event.requestId);
    clearTimeout(waiting.timer);
    const flow = flows.get(waiting.id);
    if (event.ok) flows.delete(waiting.id);
    else if (flow) flow.busy = false;
    waiting.resolve(event.ok ? { ok: true } : { ok: false, error: event.error === "invalid_answer" || event.error === "invalid_request" ? "invalid" : "closed" });
  });

  async function respond(request) {
    const identity = getIdentity();
    if (!identity?.session_id || !identity?.tmux_pane) return { ok: false, error: "unavailable" };
    if (request?.op === "list") {
      return { ok: true, identity: { managedSessionId: process.env.OAK_TREE_SESSION_ID || "", role: process.env.OAK_TREE_AGENT_ROLE || "", piSessionId: identity.session_id, paneId: identity.tmux_pane, sessionFile: identity.session_file || "" }, toolFailures: generation ? { generation, count: failureCount } : undefined, latestAnswer, flows: [...flows.values()].map(({ flowId, toolCallId, busy, ...view }) => view) };
    }
    if (request?.op === "message") {
      if (request.identity?.piSessionId !== identity.session_id || request.identity?.paneId !== identity.tmux_pane || request.identity?.sessionFile !== (identity.session_file || "") || request.identity?.managedSessionId !== (process.env.OAK_TREE_SESSION_ID || "") || request.identity?.role !== (process.env.OAK_TREE_AGENT_ROLE || "")) return { ok: false, error: "closed" };
      if (typeof request.text !== "string" || !request.text.trim() || request.text.length > 16000) return { ok: false, error: "invalid" };
      try { pi.sendUserMessage(request.text, { deliverAs: "followUp" }); return { ok: true }; } catch { return { ok: false, error: "unavailable" }; }
    }
    if (request?.op !== "submit" || typeof request.id !== "string" || typeof request.requestId !== "string" || !request.requestId || !request.response || typeof request.response !== "object") return { ok: false, error: "invalid" };
    if (request.identity?.piSessionId !== identity.session_id || request.identity?.paneId !== identity.tmux_pane || request.identity?.sessionFile !== (identity.session_file || "") || request.identity?.managedSessionId !== (process.env.OAK_TREE_SESSION_ID || "") || request.identity?.role !== (process.env.OAK_TREE_AGENT_ROLE || "")) return { ok: false, error: "closed" };
    const flow = flows.get(request.id);
    if (!flow) return { ok: false, error: "closed" };
    if (flow.busy || pending.has(request.requestId)) return { ok: false, error: "busy" };
    flow.busy = true;
    return await new Promise((resolve) => {
      const timer = setTimeout(() => {
        pending.delete(request.requestId);
        flows.delete(request.id);
        resolve({ ok: false, error: "uncertain" });
      }, 1500);
      pending.set(request.requestId, { id: request.id, flowId: flow.flowId, timer, resolve });
      pi.events.emit("@eko24ive/pi-ask:submit", { version: 1, requestId: request.requestId, flowId: flow.flowId, response: request.response });
    });
  }

  return {
    async start() {
      if (server || starting) return starting;
      starting = (async () => {
        const root = process.env.OAK_TREE_STATE_DIR || path.join(process.env.XDG_STATE_HOME || path.join(os.homedir(), ".local", "state"), "oak-tree");
        const dir = path.join(root, "office-bridges");
        await fs.promises.mkdir(dir, { recursive: true, mode: 0o700 });
        const dirInfo = await fs.promises.lstat(dir);
        if (!dirInfo.isDirectory() || dirInfo.isSymbolicLink()) throw new Error("invalid office bridge directory");
        await fs.promises.chmod(dir, 0o700);
        const candidate = path.join(dir, nonce().slice(0, 24) + ".sock");
        const instance = net.createServer((socket) => {
          socket.setTimeout(3000, () => socket.destroy());
          let input = Buffer.alloc(0);
          socket.on("error", () => {});
          socket.on("data", (data) => {
            input = Buffer.concat([input, data]);
            if (input.length > 262144) { socket.destroy(); return; }
            const end = input.indexOf(10);
            if (end < 0) return;
            socket.pause();
            let request;
            try { request = JSON.parse(input.subarray(0, end).toString("utf8")); } catch { socket.end('{"ok":false,"error":"invalid"}\n'); return; }
            respond(request).then((result) => socket.end(JSON.stringify(result) + "\n"), () => socket.end('{"ok":false,"error":"unavailable"}\n'));
          });
        });
        await new Promise((resolve, reject) => { instance.once("error", reject); instance.listen(candidate, resolve); });
        try { await fs.promises.chmod(candidate, 0o600); } catch (error) { instance.close(); try { await fs.promises.unlink(candidate); } catch {} throw error; }
        instance.on("error", () => {});
        generation = nonce();
        failureCount = 0;
        server = instance;
        socketPath = candidate;
      })();
      try { await starting; } finally { starting = undefined; }
    },
    recordToolFailure() {
      if (generation && failureCount < Number.MAX_SAFE_INTEGER) failureCount++;
    },
    endTool(toolCallId) {
      for (const [id, flow] of flows) if (flow.toolCallId && flow.toolCallId === toolCallId) flows.delete(id);
    },
    async stop() {
      if (starting) { try { await starting; } catch {} }
      generation = undefined;
      failureCount = 0;
      flows.clear();
      for (const entry of pending.values()) { clearTimeout(entry.timer); entry.resolve({ ok: false, error: "uncertain" }); }
      pending.clear();
      if (server) { server.close(); server = undefined; }
      if (socketPath) { try { await fs.promises.unlink(socketPath); } catch {} socketPath = undefined; }
    },
  };
}
