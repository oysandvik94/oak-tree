import { createQuestionPanel } from "./questions.js";

const $ = (id) => document.getElementById(id);
const canvas = $("office");
const context = canvas.getContext("2d");
const stage = $("stage");
const actors = new Map();
const images = new Map();
const rosterRows = new Map();
const speechNodes = new Map();
const camera = { x: 0, y: 0, scale: 1, fitted: true };
const viewport = { width: 1, height: 1, ratio: 1 };
const reducedMotion = matchMedia("(prefers-reduced-motion: reduce)");
const sourceNames = { task: "CURRENT TASK", session: "SESSION UPDATE", status: "SESSION STATUS" };
const stateColors = { working: "#e5be66", question: "#83d2da", ready: "#9bcc9c", review: "#c5a4d8", blocked: "#ed9992" };
let activeRoom = null;
let scene;
let navigation;
let selectedID = null;
let pointer = null;
let lastActorClick = null;
let doubleClickedActor = null;
let focusPending = false;
let hitAreas = [];
let paused = reducedMotion.matches;
let connected = false;
let polling = false;
let pollTimer;
let lastRosterSignature = "";
let lastFrame = performance.now();
let orderedAgents = [];
let sessionSuffixes = new Map();

const fragmentToken = new URLSearchParams(location.hash.slice(1)).get("token");
let token = fragmentToken;
try {
  if (fragmentToken) sessionStorage.setItem("oak-office-token", fragmentToken);
  else token = sessionStorage.getItem("oak-office-token");
} catch { /* The fragment still works when browser storage is disabled. */ }
if (fragmentToken) history.replaceState(null, "", location.pathname + location.search);
const questions = createQuestionPanel({ getToken: () => token, refresh: () => poll() });

function roomFor(data) {
  return { working: "work", question: "meeting", ready: "meeting", review: "break", blocked: "break" }[data.sessionState];
}
function insideRoom(point, room, margin = 0) {
  const [u0, v0, u1, v1] = room.bounds;
  return point.u >= u0 + margin && point.u <= u1 - margin && point.v >= v0 + margin && point.v <= v1 - margin;
}

function hash(value) {
  let result = 2166136261;
  for (const ch of value) result = Math.imul(result ^ ch.charCodeAt(0), 16777619);
  return result >>> 0;
}
const groupColor = (group) => `hsl(${hash(group.id) % 360} 70% 68%)`;
const clamp = (value, min, max) => Math.max(min, Math.min(max, value));
const distance = (a, b) => Math.hypot(a.u - b.u, a.v - b.v);

function project(u, v) {
  return { x: scene.origin[0] + (u - v) * scene.tile[0], y: scene.origin[1] + (u + v) * scene.tile[1] };
}
function unproject(x, y) {
  const u = (x - scene.origin[0]) / scene.tile[0];
  const v = (y - scene.origin[1]) / scene.tile[1];
  return { u: (u + v) / 2, v: (v - u) / 2 };
}
function localPoint(event) {
  const bounds = canvas.getBoundingClientRect();
  return { x: (event.clientX - bounds.left - camera.x) / camera.scale, y: (event.clientY - bounds.top - camera.y) / camera.scale };
}

function makeNavigation() {
  const [minU, minV, maxU, maxV] = scene.bounds;
  const step = 0.25;
  const columns = Math.floor((maxU - minU) / step) + 1;
  const rows = Math.floor((maxV - minV) / step) + 1;
  const points = [];
  const free = new Uint8Array(columns * rows);
  for (let y = 0; y < rows; y++) {
    for (let x = 0; x < columns; x++) {
      const index = y * columns + x;
      const p = { u: minU + x * step, v: minV + y * step };
      points.push(p);
      free[index] = !scene.rooms.some((room) => insideRoom(p, room)) || scene.obstacles.some((o) => p.u > o.u - 0.12 && p.u < o.u + o.w + 0.12 && p.v > o.v - 0.12 && p.v < o.v + o.d + 0.12) ? 0 : 1;
    }
  }
  return { step, columns, rows, points, free };
}
function nearestNode(point, reserved = [], room = null) {
  let best = -1;
  let bestDistance = Infinity;
  for (let index = 0; index < navigation.points.length; index++) {
    if (!navigation.free[index]) continue;
    const candidate = navigation.points[index];
    if (room && !insideRoom(candidate, room, 0.65)) continue;
    if (reserved.some((p) => distance(p, candidate) < 0.42)) continue;
    const d = (point.u - candidate.u) ** 2 + (point.v - candidate.v) ** 2;
    if (d < bestDistance) { best = index; bestDistance = d; }
  }
  return best;
}
function nearestPoint(point, reserved = [], room = null) {
  const index = nearestNode(point, reserved, room);
  return index < 0 ? null : { ...navigation.points[index] };
}
function route(start, destination) {
  const startIndex = nearestNode(start);
  const endIndex = nearestNode(destination);
  if (startIndex < 0 || endIndex < 0 || startIndex === endIndex) return [];
  const previous = new Int32Array(navigation.free.length).fill(-1);
  const queue = new Int32Array(previous.length);
  previous[startIndex] = startIndex;
  queue[0] = startIndex;
  let head = 0;
  let tail = 1;
  while (head < tail && previous[endIndex] < 0) {
    const index = queue[head++];
    const x = index % navigation.columns;
    const y = Math.floor(index / navigation.columns);
    for (const [dx, dy] of [[1, 0], [0, 1], [-1, 0], [0, -1]]) {
      const nx = x + dx;
      const ny = y + dy;
      if (nx < 0 || ny < 0 || nx >= navigation.columns || ny >= navigation.rows) continue;
      const next = ny * navigation.columns + nx;
      if (!navigation.free[next] || previous[next] >= 0) continue;
      previous[next] = index;
      queue[tail++] = next;
    }
  }
  if (previous[endIndex] < 0) return [];
  const path = [];
  for (let index = endIndex; index !== startIndex; index = previous[index]) path.push({ ...navigation.points[index] });
  return path.reverse();
}

function assignHomes(now) {
  const zones = new Map();
  const reserved = [];
  for (const actor of [...actors.values()].sort((a, b) => a.data.id.localeCompare(b.data.id))) {
    const room = scene.rooms.find((candidate) => candidate.id === roomFor(actor.data));
    const index = zones.get(room.id) || 0;
    zones.set(room.id, index + 1);
    const target = room.spots[index] || { u: room.bounds[0] + 1.5 + index % 5 * 0.65, v: room.bounds[3] - 1.2 - Math.floor(index / 5) * 0.5 };
    const seat = Boolean(target.seated);
    const home = nearestPoint(target, reserved, room) || nearestPoint(target, [], room);
    if (!home) continue;
    reserved.push(home);
    const changed = !actor.home || distance(actor.home, home) > 0.1 || actor.seat !== seat || actor.homeRoom !== room.id;
    actor.home = home;
    actor.homeRoom = room.id;
    actor.seat = seat;
    actor.seatFacing = target.facing || "back";
    if (actor.u === null) {
      actor.u = home.u;
      actor.v = home.v;
      actor.seated = seat;
    } else if (changed && !actor.held && !actor.crash) {
      actor.path = route(actor, home);
      actor.seated = false;
      actor.pauseUntil = 0;
      actor.nextMoveAt = now;
    }
  }
}

function applyState(payload) {
  if (!payload || !Array.isArray(payload.agents)) throw new Error("Invalid office response");
  const now = performance.now();
  const incoming = new Map(payload.agents.filter((a) => typeof a.id === "string" && typeof a.project === "string").map((a) => [a.id, a]));
  for (const id of actors.keys()) {
    if (!incoming.has(id)) {
      if (pointer?.actorID === id) releasePointer();
      if (id === selectedID) $("interaction-status").textContent = "That agent left the office.";
      actors.delete(id);
    }
  }
  for (const [id, data] of incoming) {
    let actor = actors.get(id);
    if (!actor) {
      actor = { data, pendingCrashes: 0, failureBaseline: null, crash: null, u: null, v: null, palette: hash(id) % scene.sprites.length, path: [], facing: "front", flip: false, held: false, seated: false, walking: false, pauseUntil: 0, nextMoveAt: now + 3000 + hash(id) % 7000, bubbleUntil: now + 6500 };
      actors.set(id, actor);
    } else {
      if (actor.data.activity !== data.activity || actor.data.sessionState !== data.sessionState) actor.bubbleUntil = now + 7000;
      if (roomFor(actor.data) !== roomFor(data) && !actor.crash) {
        actor.path = [];
        actor.seated = false;
        actor.pauseUntil = 0;
        actor.nextMoveAt = now;
      }
      actor.data = data;
    }
    const failures = data.toolFailures;
    if (failures && typeof failures.generation === "string" && failures.generation && Number.isSafeInteger(failures.count) && failures.count >= 0) {
      const previous = actor.failureBaseline;
      if (!previous || previous.generation !== failures.generation || failures.count < previous.count) {
        actor.pendingCrashes = 0;
        if (actor.crash) {
          actor.crash = null;
          actor.path = actor.home ? route(actor, actor.home) : [];
        }
      } else {
        actor.pendingCrashes += failures.count - previous.count;
      }
      actor.failureBaseline = { ...failures };
    }
  }
  const priority = { question: 0, working: 1, ready: 2, review: 3, blocked: 4 };
  orderedAgents = [...actors.values()].sort((a, b) => (priority[a.data.sessionState] ?? 5) - (priority[b.data.sessionState] ?? 5) || a.data.project.localeCompare(b.data.project) || (a.data.role || "").localeCompare(b.data.role || "") || a.data.id.localeCompare(b.data.id));
  if (!actors.has(selectedID)) selectedID = orderedAgents[0]?.data.id || null;
  assignHomes(now);
  updateRoster();
  updateInspector();
  $("stage-message").hidden = actors.size > 0;
  $("stage-message").textContent = "The office is quiet. Start an oak-tree session to welcome an agent.";
  for (const room of scene.rooms) {
    const count = orderedAgents.filter((actor) => roomFor(actor.data) === room.id).length;
    $(`room-${room.id}`).textContent = `${room.name} · ${count}`;
  }
}

function statusLabel(data) {
  return { working: "Working", question: "Question", ready: "Ready", review: "Review", blocked: "Blocked" }[data.sessionState];
}
function updateSessionSuffixes() {
  const groups = new Map();
  for (const { data } of orderedAgents) {
    const key = JSON.stringify([data.project, data.branch || ""]);
    if (!groups.has(key)) groups.set(key, new Set());
    groups.get(key).add(data.sessionId);
  }
  sessionSuffixes = new Map();
  for (const ids of groups.values()) {
    if (ids.size < 2) continue;
    for (const id of ids) {
      let length = Math.min(8, id.length);
      while (length < id.length && [...ids].some((other) => other !== id && other.startsWith(id.slice(0, length)))) length++;
      sessionSuffixes.set(id, id.slice(0, length));
    }
  }
}
function branchLabel(data) {
  return data.branch || "Root session";
}
function identityLabel(data) {
  const suffix = sessionSuffixes.get(data.sessionId);
  return `${data.project} · ${branchLabel(data)}${suffix ? ` · ${suffix}` : ""} · ${data.role || "Agent"}`;
}
function updateRoster() {
  const signature = JSON.stringify(orderedAgents.map((a) => a.data));
  if (signature === lastRosterSignature) return;
  lastRosterSignature = signature;
  updateSessionSuffixes();
  const counts = { working: 0, question: 0, ready: 0, review: 0, blocked: 0 };
  for (const actor of orderedAgents) counts[actor.data.sessionState]++;
  $("agent-total").textContent = `${actors.size} agent${actors.size === 1 ? "" : "s"} in the studio`;
  $("roster-count").textContent = String(actors.size);
  $("state-counts").replaceChildren();
  for (const [state, label] of [["working", "working"], ["question", "questions"], ["ready", "ready"], ["review", "review"], ["blocked", "blocked"]]) {
    if (!counts[state]) continue;
    const chip = document.createElement("span");
    chip.className = `count-chip ${state}`;
    chip.textContent = `${counts[state]} ${label}`;
    $("state-counts").append(chip);
  }
  for (const [id, row] of rosterRows) {
    if (!actors.has(id)) { row.remove(); rosterRows.delete(id); }
  }
  $("agent-list").querySelector(".empty-roster")?.remove();
  for (const actor of orderedAgents) {
    const data = actor.data;
    let row = rosterRows.get(data.id);
    if (!row) {
      row = document.createElement("button");
      row.className = "agent-row";
      row.type = "button";
      row.dataset.agentId = data.id;
      const avatar = document.createElement("img");
      avatar.className = "agent-avatar";
      avatar.alt = "";
      const text = document.createElement("span");
      text.className = "agent-row-text";
      const name = document.createElement("span");
      name.className = "agent-name";
      const branch = document.createElement("span");
      branch.className = "agent-branch";
      const branchName = document.createElement("span");
      branchName.className = "agent-branch-name";
      const sessionID = document.createElement("span");
      sessionID.className = "agent-session-id";
      branch.append(branchName, sessionID);
      const role = document.createElement("span");
      role.className = "agent-role";
      const group = document.createElement("span");
      group.className = "agent-group";
      const dot = document.createElement("span");
      text.append(name, branch, role, group);
      row.append(avatar, text, dot);
      row.addEventListener("click", () => selectActor(data.id, true));
      rosterRows.set(data.id, row);
    }
    const avatarSource = `/assets/${scene.sprites[actor.palette]["front-idle"].src}`;
    if (row.children[0].getAttribute("src") !== avatarSource) row.children[0].src = avatarSource;
    row.children[1].children[0].textContent = data.project;
    row.children[1].children[1].children[0].textContent = branchLabel(data);
    row.children[1].children[1].children[1].textContent = sessionSuffixes.get(data.sessionId) || "";
    row.children[1].children[2].textContent = `${data.role || "Agent"} · ${statusLabel(data)}`;
    const group = row.children[1].children[3];
    group.hidden = !data.group;
    group.textContent = data.group?.name || "";
    if (data.group) group.style.setProperty("--group-color", groupColor(data.group));
    else group.style.removeProperty("--group-color");
    row.children[2].className = `agent-state-dot ${data.sessionState}`;
    row.title = `${identityLabel(data)} · ${statusLabel(data)}${data.group ? ` · Group: ${data.group.name}` : ""}`;
    row.setAttribute("aria-label", row.title);
    row.setAttribute("aria-pressed", String(data.id === selectedID));
    $("agent-list").append(row);
  }
  if (!actors.size) {
    const empty = document.createElement("p");
    empty.className = "empty-roster";
    empty.textContent = "No agents here yet. Your managed sessions will appear automatically.";
    $("agent-list").append(empty);
  }
}
function updateInspector() {
  const actor = actors.get(selectedID);
  questions.setAgent(actor?.data);
  $("inspector").hidden = !actor;
  for (const [id, row] of rosterRows) row.setAttribute("aria-pressed", String(id === selectedID));
  if (!actor) return;
  const data = actor.data;
  $("selected-project").textContent = data.project;
  $("selected-role").textContent = data.role || "Agent";
  $("selected-status").textContent = statusLabel(data);
  $("selected-status").className = `state-label ${data.sessionState}`;
  $("selected-branch").textContent = `${branchLabel(data)}${sessionSuffixes.has(data.sessionId) ? ` · ${sessionSuffixes.get(data.sessionId)}` : ""}`;
  const group = $("selected-group");
  group.hidden = !data.group;
  group.textContent = data.group ? `Group: ${data.group.name}` : "";
  if (data.group) group.style.setProperty("--group-color", groupColor(data.group));
  else group.style.removeProperty("--group-color");
  $("activity-source").textContent = sourceNames[data.activitySource] || "CURRENT ACTIVITY";
  $("selected-activity").textContent = data.activitySource === "status" ? statusLabel(data) : data.activity || "No activity reported yet.";
}
function selectActor(id, focus = false) {
  if (!actors.has(id)) return;
  const old = actors.get(selectedID);
  if (old?.held && old.data.id !== id) dropActor(old);
  selectedID = id;
  updateInspector();
  if (focus) {
    if (activeRoom && activeRoom.id !== roomFor(actors.get(id).data)) focusRoom(roomFor(actors.get(id).data));
    canvas.focus({ preventScroll: true });
  }
}

async function poll() {
  if (polling || !token || !navigation) return;
  polling = true;
  clearTimeout(pollTimer);
  try {
    const response = await fetch("/api/state", { headers: { Authorization: `Bearer ${token}` }, cache: "no-store", signal: AbortSignal.timeout(4500) });
    if (!response.ok) throw new Error(`Office state returned ${response.status}`);
    const data = await response.json();
    applyState(data);
    connected = true;
    questions.sync(data.agents, true);
    $("connection").className = "connection live";
    $("connection").textContent = "Live";
    $("connection-warning").hidden = true;
  } catch (error) {
    connected = false;
    questions.sync([...actors.values()].map((actor) => actor.data), false);
    $("connection").className = "connection offline";
    $("connection").textContent = "Disconnected";
    $("connection-warning").hidden = false;
    $("connection-warning").textContent = "Live activity is unavailable. Reopen the office from oak-tree if the connection does not return.";
    console.warn("Office activity unavailable:", error.message);
  } finally {
    polling = false;
    pollTimer = setTimeout(poll, document.hidden ? 10000 : 2000);
  }
}

function wander(actor, now) {
  if (actor.homeRoom !== "break") {
    if (distance(actor, actor.home) > 0.12) actor.path = route(actor, actor.home);
    else actor.seated = actor.seat;
    actor.nextMoveAt = now + 5000;
    return;
  }
  const phase = hash(actor.data.id) + Math.floor(now / 7000) * 1.73;
  const room = scene.rooms.find((candidate) => candidate.id === actor.homeRoom);
  const target = nearestPoint({ u: actor.home.u + Math.sin(phase) * 0.65, v: actor.home.v + Math.cos(phase) * 0.65 }, [], room);
  if (target) actor.path = route(actor, target);
  actor.nextMoveAt = now + 5000 + hash(actor.data.id) % 5000;
}
function wallDistance(point, wall) {
  const [u0, v0, u1, v1] = wall;
  const du = u1 - u0, dv = v1 - v0;
  const t = ((point.u - u0) * du + (point.v - v0) * dv) / (du * du + dv * dv);
  if (t <= 0 || t >= 1) return Infinity;
  return Math.hypot(point.u - (u0 + t * du), point.v - (v0 + t * dv));
}
function startCrash(actor) {
  const room = scene.rooms.find((candidate) => insideRoom(actor, candidate));
  if (!room) return false;
  const candidates = navigation.points.filter((point, index) => navigation.free[index] && insideRoom(point, room, 0.3) && scene.walls.some((wall) => wallDistance(point, wall) <= 0.55));
  candidates.sort((a, b) => distance(actor, a) - distance(actor, b));
  for (const target of candidates) {
    const path = route(actor, target);
    if (!path.length && distance(actor, target) > 0.3) continue;
    actor.path = path;
    actor.seated = false;
    actor.crash = { phase: path.length ? "run" : "impact", elapsed: 0 };
    return true;
  }
  return false;
}
function interruptCrash(actor) {
  if (actor.crash) actor.pendingCrashes++;
  actor.crash = null;
  actor.returningFromCrash = false;
}
function advance(seconds, now) {
  for (const actor of actors.values()) {
    actor.walking = false;
    if (paused || !connected || actor.held || now < actor.pauseUntil) continue;
    if (!actor.crash && !actor.returningFromCrash && actor.pendingCrashes > 0) {
      actor.pendingCrashes--;
      if (!startCrash(actor)) actor.nextMoveAt = now;
    }
    if (actor.crash && actor.crash.phase !== "run") {
      actor.crash.elapsed += seconds;
      if (actor.crash.elapsed >= 1.5) {
        actor.crash = null;
        actor.path = route(actor, actor.home);
        actor.returningFromCrash = actor.path.length > 0;
        actor.nextMoveAt = now + 5000;
      }
      continue;
    }
    if (!actor.crash && !actor.path.length && now >= actor.nextMoveAt) wander(actor, now);
    let remaining = seconds * (actor.crash ? 4.5 : 1.45);
    while (actor.path.length && remaining > 0) {
      const target = actor.path[0];
      const d = distance(actor, target);
      const du = target.u - actor.u;
      const dv = target.v - actor.v;
      if (d > 0.002) {
        actor.facing = du + dv < 0 ? "back" : "front";
        actor.flip = du - dv < 0;
        actor.walking = true;
        actor.seated = false;
      }
      if (d <= remaining) {
        actor.u = target.u;
        actor.v = target.v;
        actor.path.shift();
        remaining -= d;
      } else {
        actor.u += du / d * remaining;
        actor.v += dv / d * remaining;
        remaining = 0;
      }
    }
    if (actor.returningFromCrash && !actor.path.length) actor.returningFromCrash = false;
    if (actor.crash && actor.crash.phase === "run" && !actor.path.length) actor.crash = { phase: "impact", elapsed: 0 };
    if (!actor.crash && !actor.path.length && actor.seat && distance(actor, actor.home) < 0.13) actor.seated = true;
  }
}
function spriteFor(actor, now) {
  const pose = actor.seated ? `${actor.seatFacing}-seated` : `${actor.facing}-${actor.walking ? `walk${Math.floor(now / 170) % 2}` : "idle"}`;
  return scene.sprites[actor.palette][pose];
}
function drawActor(actor, now) {
  const p = project(actor.u, actor.v);
  const sprite = spriteFor(actor, now);
  const image = images.get(sprite.src);
  if (actor.data.id === selectedID) {
    context.strokeStyle = actor.held ? "#ffdf99" : "#e5bb73";
    context.lineWidth = 0.8;
    context.beginPath();
    context.ellipse(p.x, p.y + 1, 8.5, 4, 0, 0, Math.PI * 2);
    context.stroke();
  }
  context.save();
  context.translate(p.x, p.y - (actor.held ? 5 : 0));
  if (actor.crash?.phase === "impact") context.rotate((actor.flip ? 1 : -1) * (actor.crash.elapsed < 0.25 ? -0.35 : actor.crash.elapsed < 1.15 ? 1.3 : 0.4));
  if (actor.flip && !actor.seated) context.scale(-1, 1);
  context.drawImage(image, -sprite.anchorX, -sprite.anchorY);
  if (actor.data.group) {
    context.fillStyle = "#342b30";
    context.fillRect(-4, -sprite.anchorY + 7, 9, 4);
    context.fillRect(2, -sprite.anchorY + 9, actor.walking ? 8 : 4, 4);
    context.fillStyle = groupColor(actor.data.group);
    context.fillRect(-3, -sprite.anchorY + 8, 7, 2);
    context.fillRect(3, -sprite.anchorY + 10, actor.walking ? 6 : 2, 2);
  }
  context.restore();
  if (actor.crash?.phase === "impact" && actor.crash.elapsed < 1.15) {
    context.fillStyle = "#ffe57b";
    context.font = "bold 9px monospace";
    context.fillText("✦", p.x + 7, p.y - sprite.anchorY - 3);
  }
  hitAreas.push({ id: actor.data.id, x: p.x - sprite.anchorX - 2, y: p.y - sprite.anchorY - (actor.held ? 5 : 0) - 2, width: image.width + 4, height: image.height + 4 });
  const markerY = p.y - sprite.anchorY - (actor.held ? 5 : 0) - 5;
  context.beginPath();
  context.arc(p.x, markerY, 3.5, 0, Math.PI * 2);
  context.fillStyle = stateColors[actor.data.sessionState];
  context.fill();
  context.strokeStyle = "#343b3d";
  context.lineWidth = 0.8;
  context.stroke();
  if (actor.data.group) {
    context.fillStyle = "#343b3d";
    context.fillRect(p.x + 6, markerY - 6, 1, 11);
    context.fillStyle = groupColor(actor.data.group);
    context.beginPath();
    context.moveTo(p.x + 7, markerY - 6);
    context.lineTo(p.x + 13, markerY - 4);
    context.lineTo(p.x + 7, markerY - 1);
    context.closePath();
    context.fill();
  }
  if (actor.data.sessionState === "question") {
    context.fillStyle = "#263a3b";
    context.font = "bold 6px monospace";
    context.fillText("?", p.x - 1.8, markerY + 2);
  }
}
function draw(now) {
  context.setTransform(viewport.ratio, 0, 0, viewport.ratio, 0, 0);
  context.fillStyle = "#171d27";
  context.fillRect(0, 0, viewport.width, viewport.height);
  context.translate(camera.x, camera.y);
  context.scale(camera.scale, camera.scale);
  context.imageSmoothingEnabled = false;
  context.drawImage(images.get(scene.background), 0, 0);
  hitAreas = [];
  const drawables = scene.furniture.map((item) => ({ depth: item.depth, item }));
  for (const actor of actors.values()) drawables.push({ depth: actor.held ? Infinity : actor.u + actor.v, actor });
  drawables.sort((a, b) => a.depth - b.depth);
  for (const drawable of drawables) {
    if (drawable.actor) drawActor(drawable.actor, now);
    else context.drawImage(images.get(drawable.item.src), drawable.item.x, drawable.item.y);
  }
  updateBubbles(now);
}
function updateBubbles(now) {
  const speakers = [];
  const selected = actors.get(selectedID);
  if (selected) speakers.push(selected);
  const others = orderedAgents.filter((actor) => actor !== selected && (actor.data.sessionState === "question" || actor.bubbleUntil > now));
  if (others.length) speakers.push(others[Math.floor(now / 7000) % others.length]);
  const visible = new Set(speakers.map((a) => a.data.id));
  for (const [id, node] of speechNodes) {
    if (!visible.has(id)) { node.remove(); speechNodes.delete(id); }
  }
  const occupied = [];
  for (const actor of speakers) {
    let node = speechNodes.get(actor.data.id);
    if (!node) {
      node = document.createElement("button");
      node.type = "button";
      node.className = "speech-bubble";
      node.addEventListener("click", () => {
        selectActor(actor.data.id);
        if (actor.data.questions?.length) questions.open(actor.data);
      });
      const source = document.createElement("span");
      source.className = "bubble-source";
      const text = document.createElement("span");
      text.className = "bubble-text";
      node.append(source, text);
      $("bubbles").append(node);
      speechNodes.set(actor.data.id, node);
    }
    node.classList.toggle("selected", actor.data.id === selectedID);
    const pending = actor.data.questions?.[0];
    const data = actor.data;
    const label = `${identityLabel(data)}${!pending && data.activitySource === "session" ? " · Session update" : ""}`;
    if (node.children[0].textContent !== label) node.children[0].textContent = label;
    node.classList.toggle("has-question", Boolean(pending));
    node.title = identityLabel(data);
    node.setAttribute("aria-label", `${pending ? "Answer question from" : "Select"} ${identityLabel(data)}`);
    const activity = pending ? pending.questions?.[0]?.prompt || pending.title || "Your answer is needed" : actor.data.activitySource === "status" ? statusLabel(actor.data) : actor.data.activity || statusLabel(actor.data);
    if (node.children[1].textContent !== activity) node.children[1].textContent = activity;
    const p = project(actor.u, actor.v);
    const width = node.offsetWidth;
    const height = node.offsetHeight;
    const x = clamp(camera.x + p.x * camera.scale, width / 2 + 8, viewport.width - width / 2 - 8);
    let y = camera.y + (p.y - 28 - (actor.held ? 5 : 0)) * camera.scale - 8;
    y = clamp(y, height + 10, viewport.height - 10);
    for (const other of occupied) {
      if (Math.abs(x - other.x) < (width + other.width) / 2 + 8 && y > other.y - other.height - 8 && y - height < other.y + 8) y = other.y - other.height - 12;
    }
    const inView = camera.x + p.x * camera.scale > -10 && camera.x + p.x * camera.scale < viewport.width + 10 && y > height + 3;
    node.style.visibility = inView ? "visible" : "hidden";
    node.style.left = `${x}px`;
    node.style.top = `${y}px`;
    if (inView) occupied.push({ x, y, width, height });
  }
}
function tick(now) {
  const seconds = Math.min(0.05, Math.max(0, (now - lastFrame) / 1000));
  lastFrame = now;
  if (!document.hidden) { advance(seconds, now); draw(now); }
  requestAnimationFrame(tick);
}

function fitRoom() {
  if (!scene) return;
  const [x, y, width, height] = activeRoom?.view || [0, 0, scene.width, scene.height];
  camera.scale = Math.min(viewport.width / width, viewport.height / height) * 0.94;
  camera.x = (viewport.width - width * camera.scale) / 2 - x * camera.scale;
  camera.y = (viewport.height - height * camera.scale) / 2 - y * camera.scale;
  camera.fitted = true;
}
function focusRoom(id = null) {
  if (!scene) return;
  activeRoom = scene.rooms.find((room) => room.id === id) || null;
  for (const button of $("room-navigation").querySelectorAll("button")) {
    button.setAttribute("aria-pressed", String(button.dataset.room === (activeRoom?.id || "all")));
  }
  fitRoom();
}
function resize() {
  const bounds = stage.getBoundingClientRect();
  const oldWidth = viewport.width;
  const oldHeight = viewport.height;
  viewport.width = Math.max(1, bounds.width);
  viewport.height = Math.max(1, bounds.height);
  viewport.ratio = Math.min(2, devicePixelRatio || 1);
  canvas.width = Math.round(viewport.width * viewport.ratio);
  canvas.height = Math.round(viewport.height * viewport.ratio);
  if (camera.fitted) fitRoom();
  else { camera.x += (viewport.width - oldWidth) / 2; camera.y += (viewport.height - oldHeight) / 2; }
}
function hitActor(p) {
  return [...hitAreas].reverse().find((area) => p.x >= area.x && p.x <= area.x + area.width && p.y >= area.y && p.y <= area.y + area.height);
}
function dropActor(actor) {
  actor.held = false;
  actor.pauseUntil = performance.now() + 8000;
  actor.nextMoveAt = actor.pauseUntil;
  actor.path = [];
  $("interaction-status").textContent = "Dropped. Back to work in a moment.";
}
function releasePointer() {
  if (pointer?.actorID) {
    const actor = actors.get(pointer.actorID);
    if (actor && pointer.moved) dropActor(actor);
  }
  if (pointer && canvas.hasPointerCapture(pointer.id)) canvas.releasePointerCapture(pointer.id);
  pointer = null;
  canvas.dataset.dragging = "false";
}
canvas.addEventListener("pointerdown", (event) => {
  if (!scene || ![0, 1].includes(event.button)) return;
  const p = localPoint(event);
  const hit = hitActor(p);
  canvas.focus({ preventScroll: true });
  if (hit && !event.shiftKey && event.button === 0) {
    const actor = actors.get(hit.id);
    selectActor(hit.id);
    const foot = project(actor.u, actor.v);
    pointer = { id: event.pointerId, actorID: hit.id, x: event.clientX, y: event.clientY, offsetX: foot.x - p.x, offsetY: foot.y - p.y, moved: false };
  } else if (event.shiftKey || event.button === 1 || event.pointerType === "touch") {
    lastActorClick = doubleClickedActor = null;
    pointer = { id: event.pointerId, pan: true, x: event.clientX, y: event.clientY, cameraX: camera.x, cameraY: camera.y, moved: false };
  } else {
    lastActorClick = doubleClickedActor = null;
    return;
  }
  event.preventDefault();
  canvas.setPointerCapture(event.pointerId);
});
canvas.addEventListener("pointermove", (event) => {
  if (!scene) return;
  const p = localPoint(event);
  if (!pointer) { canvas.dataset.hover = String(Boolean(hitActor(p))); return; }
  if (event.pointerId !== pointer.id) return;
  if (!pointer.moved && Math.hypot(event.clientX - pointer.x, event.clientY - pointer.y) < 4) return;
  pointer.moved = true;
  lastActorClick = doubleClickedActor = null;
  canvas.dataset.dragging = "true";
  if (pointer.pan) {
    camera.fitted = false;
    camera.x = pointer.cameraX + event.clientX - pointer.x;
    camera.y = pointer.cameraY + event.clientY - pointer.y;
    return;
  }
  const actor = actors.get(pointer.actorID);
  if (!actor) { releasePointer(); return; }
  const target = nearestPoint(unproject(p.x + pointer.offsetX, p.y + pointer.offsetY));
  if (!target) return;
  interruptCrash(actor);
  actor.held = true;
  actor.seated = false;
  actor.path = [];
  actor.u = target.u;
  actor.v = target.v;
  $("interaction-status").textContent = `Holding ${actor.data.role || "agent"} · release to drop`;
});
canvas.addEventListener("pointerup", (event) => {
  if (pointer?.id !== event.pointerId) return;
  const hit = hitActor(localPoint(event));
  if (pointer.actorID && !pointer.moved && hit?.id === pointer.actorID) {
    const now = performance.now();
    doubleClickedActor = lastActorClick?.id === hit.id && now - lastActorClick.at < 700 ? hit.id : null;
    lastActorClick = { id: hit.id, at: now };
  } else lastActorClick = doubleClickedActor = null;
  releasePointer();
});
canvas.addEventListener("dblclick", async (event) => {
  const hit = hitActor(localPoint(event));
  const id = doubleClickedActor;
  lastActorClick = doubleClickedActor = null;
  if (event.button !== 0 || event.shiftKey || focusPending || !hit || hit.id !== id || !actors.has(id)) return;
  focusPending = true;
  $("interaction-status").textContent = "Opening session…";
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 7000);
  try {
    const response = await fetch("/api/focus", { method: "POST", headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" }, body: JSON.stringify({ agentId: id }), signal: controller.signal });
    const result = await response.json();
    $("interaction-status").textContent = response.ok ? "Session opened." : result.message || "Could not open the session.";
  } catch {
    $("interaction-status").textContent = "Could not open the session. Check the terminal before trying again.";
  } finally {
    clearTimeout(timeout);
    focusPending = false;
  }
});
canvas.addEventListener("pointercancel", () => { lastActorClick = doubleClickedActor = null; releasePointer(); });
canvas.addEventListener("lostpointercapture", () => { if (pointer) { lastActorClick = doubleClickedActor = null; releasePointer(); } });
canvas.addEventListener("wheel", (event) => {
  if (!scene) return;
  event.preventDefault();
  const p = localPoint(event);
  const fitted = Math.min(viewport.width / scene.width, viewport.height / scene.height) * 0.97;
  const nextScale = clamp(camera.scale * Math.exp(-event.deltaY * 0.001), fitted * 0.6, fitted * 3.5);
  camera.x += p.x * (camera.scale - nextScale);
  camera.y += p.y * (camera.scale - nextScale);
  camera.scale = nextScale;
  camera.fitted = false;
}, { passive: false });
canvas.addEventListener("keydown", (event) => {
  if (!scene || event.ctrlKey || event.metaKey || event.altKey) return;
  if (event.key.toLowerCase() === "f") { focusRoom(); event.preventDefault(); return; }
  if (event.key === "Enter" && actors.get(selectedID)?.data.questions?.length) {
    event.preventDefault();
    questions.open(actors.get(selectedID).data);
    return;
  }
  const actor = actors.get(selectedID);
  if (!actor) return;
  if (event.key === " " || event.key === "Escape") {
    event.preventDefault();
    if (actor.held || event.key === "Escape") { releasePointer(); dropActor(actor); }
    else { interruptCrash(actor); actor.held = true; actor.seated = false; actor.path = []; $("interaction-status").textContent = "Picked up · arrows move · Space drops"; }
    return;
  }
  const directions = { ArrowUp: [-0.25, -0.25], ArrowDown: [0.25, 0.25], ArrowLeft: [-0.25, 0.25], ArrowRight: [0.25, -0.25] };
  const direction = directions[event.key];
  if (!direction) return;
  event.preventDefault();
  const target = { u: actor.u + direction[0], v: actor.v + direction[1] };
  const free = nearestPoint(target);
  if (!free || distance(free, target) > 0.15) return;
  interruptCrash(actor);
  actor.u = free.u;
  actor.v = free.v;
  actor.facing = direction[0] + direction[1] < 0 ? "back" : "front";
  actor.flip = direction[0] - direction[1] < 0;
  actor.seated = false;
  actor.path = [];
  actor.pauseUntil = performance.now() + 8000;
  actor.nextMoveAt = actor.pauseUntil;
  $("interaction-status").textContent = actor.held ? "Picked up · arrows move · Space drops" : "Moved. Real agent work continues.";
});
$("fit-room").addEventListener("click", () => focusRoom());
$("room-navigation").addEventListener("click", (event) => {
  const button = event.target.closest("button[data-room]");
  if (button) focusRoom(button.dataset.room);
});
function updateMotionButton() {
  $("motion-toggle").textContent = paused ? "Resume motion" : "Pause motion";
  $("motion-toggle").setAttribute("aria-pressed", String(paused));
}
$("motion-toggle").addEventListener("click", () => { paused = !paused; updateMotionButton(); });
$("roster-toggle").addEventListener("click", () => {
  const open = $("roster-panel").classList.toggle("open");
  $("roster-toggle").setAttribute("aria-expanded", String(open));
});
document.addEventListener("visibilitychange", () => { if (!document.hidden) { lastFrame = performance.now(); poll(); } });
window.addEventListener("blur", releasePointer);
new ResizeObserver(resize).observe(stage);

async function loadArtwork() {
  const response = await fetch("/assets/scene.json");
  if (!response.ok) throw new Error("Scene manifest unavailable");
  scene = await response.json();
  const names = new Set([scene.background, ...scene.furniture.map((f) => f.src), ...scene.sprites.flatMap((variants) => Object.values(variants).map((v) => v.src))]);
  await Promise.all([...names].map(async (name) => {
    const image = new Image();
    image.src = `/assets/${name}`;
    await image.decode();
    images.set(name, image);
  }));
  navigation = makeNavigation();
  resize();
}
async function start() {
  updateMotionButton();
  try {
    await loadArtwork();
    requestAnimationFrame(tick);
    if (!token) {
      $("stage-message").textContent = "Open this office with oak-tree office to connect your agents.";
      $("connection").textContent = "Not connected";
      return;
    }
    await poll();
  } catch (error) {
    $("stage-message").hidden = false;
    $("stage-message").textContent = "The office could not load. Reopen it from oak-tree.";
    $("connection").className = "connection offline";
    $("connection").textContent = "Unavailable";
    console.error("Office initialization failed:", error);
  }
}
start();
