import { createQuestionPanel } from "./questions.js";
import { advanceBubble, createActivityTracker } from "./activity.js";
import { agentState, gymAgents, hash } from "./assets/gym/state.js";
import { placeOverlays, positionOverlay } from "./assets/gym/overlays.js";

const $ = id => document.getElementById(id);
const canvas = $("office"), stage = $("stage");
const actors = new Map(), rosterRows = new Map(), speechNodes = new Map();
const reducedMotion = matchMedia("(prefers-reduced-motion: reduce)");
const sourceNames = {task:"CURRENT TASK", session:"CAMPFIRE UPDATE", status:"AGENT STATUS"};
// Undefined allows automatic selection; null preserves an explicit deselection.
let selectedID, focusPending = false;
let connected = false, polling = false, pollTimer, view = null, renderingError = "";
let lastRosterSignature = "", orderedAgents = [], sessionSuffixes = new Map();
const agentColor = id => `hsl(${hash(id)%360} 65% 76%)`;
const groupColor = group => `hsl(${hash(group.id)%360} 70% 68%)`;
const fragmentToken = new URLSearchParams(location.hash.slice(1)).get("token");
let token = fragmentToken;
try {
  if (fragmentToken) sessionStorage.setItem("oak-office-token", fragmentToken);
  else token = sessionStorage.getItem("oak-office-token");
} catch { /* The fragment still works when browser storage is disabled. */ }
if (fragmentToken) history.replaceState(null, "", location.pathname + location.search);
const questions = createQuestionPanel({ getToken: () => token, refresh: () => poll() });
const activityTracker = createActivityTracker();
let chatEntries = [];
let lastChatSignature = "";

function renderChatLog() {
  const signature = JSON.stringify([chatEntries, [...actors.keys()]]);
  if (signature === lastChatSignature) return;
  lastChatSignature = signature;
  const list = $("chat-log");
  const scroll = list.parentElement;
  const previousHeight = scroll.scrollHeight;
  const previousTop = scroll.scrollTop;
  list.replaceChildren();
  const entries = chatEntries;
  $("chat-empty").hidden = entries.length > 0;
  $("chat-clear").disabled = entries.length === 0;
  for (const entry of [...entries].reverse()) {
    const row = document.createElement("li"); row.className = "chat-entry";
    row.style.setProperty("--agent-color", agentColor(entry.agentId));
    const author = document.createElement("button"); author.type = "button"; author.className = "chat-author quiet-button"; author.textContent = entry.author;
    author.disabled = !actors.has(entry.agentId);
    author.addEventListener("click", () => selectActor(entry.agentId, true));
    const time = document.createElement("time"); time.className = "chat-time"; time.dateTime = entry.time;
    const date = new Date(entry.time); time.textContent = Number.isNaN(date.getTime()) ? "" : date.toLocaleTimeString([], { hour:"2-digit", minute:"2-digit" });
    time.title = Number.isNaN(date.getTime()) ? "" : date.toLocaleString();
    const text = document.createElement("span"); text.className = "chat-message"; text.textContent = entry.text;
    row.append(time, " ", author, ": ", text); list.append(row);
  }
  if (previousTop > 0) scroll.scrollTop = previousTop + scroll.scrollHeight - previousHeight;
}
$("chat-clear").addEventListener("click", async () => {
  $("chat-clear").disabled = true;
  try {
    const response = await fetch("/api/activity/clear", { method:"POST", headers:{ Authorization:`Bearer ${token}`, "Content-Type":"application/json" }, body:"{}", signal:AbortSignal.timeout(5000) });
    if (!response.ok) throw new Error("clear failed");
    chatEntries = [];
    renderChatLog();
    await poll();
  } catch { $("interaction-status").textContent = "Could not clear the activity log. Try again."; }
  finally { $("chat-clear").disabled = chatEntries.length === 0; }
});
renderChatLog();

function statusLabel(data) {
  if(data.spotting)return 'Spotting · Session busy';
  return { working: "Working", question: "Question", ready: "Ready", review: "Review", blocked: "Blocked" }[agentState(data)] || "Unknown";
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
  return `${data.name || data.project} · ${branchLabel(data)}${suffix ? ` · ${suffix}` : ""} · ${data.role || "Agent"}`;
}
function updateRoster() {
  const signature = JSON.stringify(orderedAgents.map((a) => a.data));
  if (signature === lastRosterSignature) return;
  lastRosterSignature = signature;
  updateSessionSuffixes();
  const counts = { working: 0, spotting: 0, question: 0, ready: 0, review: 0, blocked: 0 };
  for(const {data} of orderedAgents){const state=data.spotting?'spotting':agentState(data);counts[state]=(counts[state]||0)+1;}
  $("roster-count").textContent = String(actors.size);
  $("state-counts").replaceChildren();
  for (const [state, label] of [["working", "working"], ["spotting", "spotting"], ["question", "questions"], ["ready", "ready"], ["review", "review"], ["blocked", "blocked"], ["unknown", "unknown"]]) {
    if (!counts[state]) continue;
    const chip = document.createElement("span");
    chip.className = `count-chip ${state==='spotting'?'working':state}`;
    chip.textContent = `${counts[state]} ${label}`;
    $("state-counts").append(chip);
  }
  const busySessions=new Set(orderedAgents.filter(actor=>actor.data.sessionBusy).map(actor=>actor.data.sessionId));
  if(busySessions.size){const chip=document.createElement('span');chip.className='count-chip working';chip.textContent=`${busySessions.size} busy ${busySessions.size===1?'session':'sessions'}`;$('state-counts').append(chip);}
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
      const avatar = document.createElement("span");
      avatar.className = "agent-avatar";
      avatar.setAttribute("aria-hidden", "true");
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
    row.children[0].textContent = (data.role || data.project || "A").slice(0,2).toUpperCase();
    row.children[0].style.color = agentColor(data.id);
    row.children[1].children[0].textContent = data.name || data.project;
    row.children[1].children[0].style.color = agentColor(data.id);
    row.children[1].children[1].children[0].textContent = branchLabel(data);
    row.children[1].children[1].children[1].textContent = sessionSuffixes.get(data.sessionId) || "";
    row.children[1].children[2].textContent = `${data.role || "Agent"} · ${statusLabel(data)}`;
    const group = row.children[1].children[3];
    group.hidden = !data.group;
    group.textContent = data.group?.name || "";
    if (data.group) group.style.setProperty("--group-color", groupColor(data.group));
    else group.style.removeProperty("--group-color");
    row.children[2].className = `agent-state-dot ${agentState(data)}`;
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
  view?.select(selectedID);
  $("inspector").hidden = !actor;
  for (const [id, row] of rosterRows) row.setAttribute("aria-pressed", String(id === selectedID));
  if (!actor) return;
  const data = actor.data;
  renderTasksAndPR(data);
  $("selected-project").textContent = data.name || data.project;
  $("selected-role").textContent = data.role || "Agent";
  $("selected-status").textContent = statusLabel(data);
  $("selected-status").className = `state-label ${agentState(data)}`;
  $("selected-branch").textContent = `${branchLabel(data)}${sessionSuffixes.has(data.sessionId) ? ` · ${sessionSuffixes.get(data.sessionId)}` : ""}`;
  const group = $("selected-group");
  group.hidden = !data.group;
  group.textContent = data.group ? `Group: ${data.group.name}` : "";
  if (data.group) group.style.setProperty("--group-color", groupColor(data.group));
  else group.style.removeProperty("--group-color");
  $("activity-source").textContent = sourceNames[data.activitySource] || "CURRENT ACTIVITY";
  $("selected-activity").textContent = data.activitySource === "status" ? statusLabel(data) : data.activity || "No activity reported yet.";
}
let notesAgentID;
function renderNotes() {
  const list = $("notes-list");
  list.replaceChildren();
  const notes = actors.get(notesAgentID)?.data.notes || [];
  if (!notes.length) { const empty = document.createElement("p"); empty.className = "answer-hint"; empty.textContent = "No notes yet."; list.append(empty); }
  for (const note of notes) {
    const row = document.createElement("div"); row.className = "note-row";
    const text = document.createElement("p"); text.textContent = note.text;
    const remove = document.createElement("button"); remove.type = "button"; remove.className = "quiet-button"; remove.textContent = "Delete";
    remove.addEventListener("click", () => updateNotes("delete", "", note.id));
    row.append(text, remove); list.append(row);
  }
}
async function updateNotes(action, text = "", id = "") {
  const agentId = notesAgentID;
  $("notes-add").disabled = true;
  for (const button of $("notes-list").querySelectorAll("button")) button.disabled = true;
  $("notes-feedback").textContent = "Saving…";
  try {
    const response = await fetch("/api/notes", { method:"POST", headers:{ Authorization:`Bearer ${token}`, "Content-Type":"application/json" }, body:JSON.stringify({ agentId, action, text, id }), signal:AbortSignal.timeout(5000) });
    if (!response.ok) throw new Error("failed");
    const result = await response.json();
    const data = actors.get(agentId)?.data;
    if (data) data.notes = action === "add" ? [...(data.notes || []), result.note] : (data.notes || []).filter((note) => note.id !== id);
    if (action === "add") $("notes-text").value = "";
    $("notes-feedback").textContent = "Saved.";
    renderNotes();
  } catch { $("notes-feedback").textContent = "Could not confirm the update. Reopen notes to check before retrying."; }
  finally { $("notes-add").disabled = false; for (const button of $("notes-list").querySelectorAll("button")) button.disabled = false; }
}
$("notes-open").addEventListener("click", () => {
  const data = actors.get(selectedID)?.data;
  if (!data) return;
  notesAgentID = data.id;
  $("notes-agent").textContent = identityLabel(data);
  $("notes-text").value = ""; $("notes-feedback").textContent = "";
  renderNotes(); $("notes-dialog").showModal();
});
$("notes-close").addEventListener("click", () => $("notes-dialog").close());
$("notes-form").addEventListener("submit", (event) => { event.preventDefault(); const text = $("notes-text").value; if (text.trim()) updateNotes("add", text); });
let conversationAgentID;
$("conversation-open").addEventListener("click", () => {
  const data = actors.get(selectedID)?.data;
  if (!data) return;
  conversationAgentID = data.id;
  $("conversation-agent").textContent = identityLabel(data);
  $("conversation-answer").textContent = data.latestAnswer || "No answer available from this agent yet.";
  $("conversation-reply").value = "";
  $("conversation-feedback").textContent = data.canReply ? "" : "This agent is not connected.";
  $("conversation-send").disabled = !connected || !data.canReply;
  $("conversation-dialog").showModal();
});
$("conversation-close").addEventListener("click", () => $("conversation-dialog").close());
$("conversation-send").addEventListener("click", async () => {
  const text = $("conversation-reply").value;
  if (!text.trim()) return;
  $("conversation-send").disabled = true;
  $("conversation-feedback").textContent = "Sending…";
  try {
    const response = await fetch("/api/message", { method:"POST", headers:{ Authorization:`Bearer ${token}`, "Content-Type":"application/json" }, body:JSON.stringify({ agentId:conversationAgentID, text }), signal:AbortSignal.timeout(5000) });
    if (!response.ok) throw new Error("send failed");
    $("conversation-reply").value = "";
    $("conversation-feedback").textContent = "Sent or queued in Pi.";
  } catch {
    $("conversation-feedback").textContent = "Delivery could not be confirmed. Check Pi before sending again.";
  } finally { $("conversation-send").disabled = !connected || !actors.get(conversationAgentID)?.data.canReply; }
});
async function poll() {
  if (polling || !token) return;
  polling = true;
  clearTimeout(pollTimer);
  try {
    const response = await fetch("/api/state", { headers: { Authorization: `Bearer ${token}` }, cache: "no-store", signal: AbortSignal.timeout(4500) });
    if (!response.ok) throw new Error(`Office state returned ${response.status}`);
    const data = await response.json();
    applyState(data);
    connected = true;
    view?.setConnected(true);
    $("session-open").disabled = focusPending;
    questions.sync(data.agents, true);
    $("connection").className = "connection live";
    $("connection").textContent = "Live";
    $("connection-warning").hidden = !renderingError;
    if (renderingError) $("connection-warning").textContent = renderingError;
  } catch (error) {
    connected = false;
    view?.setConnected(false);
    $("session-open").disabled = true;
    questions.sync([...actors.values()].map((actor) => actor.data), false);
    $("connection").className = "connection offline";
    $("connection").textContent = "Disconnected";
    $("connection-warning").hidden = false;
    $("connection-warning").textContent = "Live activity is unavailable. Reopen the gym with oak-tree office if the connection does not return.";
    console.warn("Office activity unavailable:", error.message);
  } finally {
    polling = false;
    pollTimer = setTimeout(poll, document.hidden ? 10000 : 2000);
  }
}

function applyState(payload) {
  if (!payload || !Array.isArray(payload.agents)) throw new Error('Invalid office response');
  const incoming = new Map(gymAgents(payload.agents).filter(a => typeof a.id === 'string' && typeof a.project === 'string').map(a => [a.id, a]));
  for (const id of actors.keys()) if (!incoming.has(id)) {actors.delete(id); speechNodes.get(id)?.remove(); speechNodes.delete(id);}
  for (const [id, data] of incoming) {
    if (!actors.has(id)) actors.set(id, {data, bubbleQueue: [], bubble: null, bubbleUntil: 0});
    else actors.get(id).data = data;
  }
  const priority = {question:0, blocked:1, working:2, review:3, ready:4, unknown:5};
  orderedAgents = [...actors.values()].sort((a,b) => priority[agentState(a.data)]-priority[agentState(b.data)] || a.data.project.localeCompare(b.data.project) || a.data.id.localeCompare(b.data.id));
  if (selectedID !== null && !actors.has(selectedID)) selectedID = orderedAgents[0]?.data.id;
  view?.sync([...incoming.values()]);
  updateRoster(); updateInspector();
  chatEntries = Array.isArray(payload.history) ? payload.history : [];
  for (const entry of activityTracker.observe(chatEntries)) {
    const actor = actors.get(entry.agentId);
    if (actor && (entry.source === 'CAMPFIRE UPDATE' || entry.questionId)) actor.bubbleQueue.push(entry);
  }
  renderChatLog();
  $('stage-message').hidden = actors.size > 0;
  $('stage-message').textContent = 'The gym is quiet. Start an oak-tree session to welcome an agent.';
}

function renderTasksAndPR(data) {
  const tasks = $('selected-tasks'); tasks.replaceChildren();
  for (const task of data.todo?.tasks || []) {
    if (task.status === 'deleted') continue;
    const row = document.createElement('li'); row.textContent = `${task.status.replaceAll('_',' ')} · ${task.subject}`; row.dataset.status=task.status;tasks.append(row);
  }
  $('tasks-section').hidden = !tasks.children.length;
  const pr = $('selected-pr'); pr.replaceChildren();pr.hidden = !data.pr?.found;
  if (data.pr?.found) {
    const summary=document.createElement('p'); summary.textContent=`PR #${data.pr.number} · ${data.pr.state || 'unknown'}${data.pr.is_draft ? ' · draft' : ''} · CI ${data.pr.checks_state || 'unknown'}`;pr.append(summary);
    const review=document.createElement('p'); review.textContent=`${data.pr.review_decision || 'No review decision'}${data.pr.unresolved_comments_checked ? ` · ${data.pr.unresolved_comments ?? 0} unresolved comments` : ''}`;pr.append(review);
    try {const url=new URL(data.pr.url);if(url.protocol==='https:' && url.hostname==='github.com'){const link=document.createElement('a');link.textContent='Open pull request ↗';link.href=url.href;link.target='_blank';link.rel='noopener noreferrer';pr.append(link);}} catch { /* Invalid cached URLs remain plain text. */ }
    const cached=document.createElement('small');const refreshed=new Date(data.pr.refreshed_at);cached.textContent=`Cached${Number.isNaN(refreshed.getTime()) ? '' : ' · '+refreshed.toLocaleString()}`;pr.append(cached);
  }
}
function selectActor(id, focus=false) {
  if (id !== null && !actors.has(id)) return;
  selectedID=id;updateInspector();view?.select(id,focus);
  if(focus)canvas.focus({preventScroll:true});
}
async function openSession(id=selectedID) {
  if (focusPending || !actors.has(id) || !connected) return;
  focusPending=true; $('session-open').disabled=true;$('interaction-status').textContent='Opening session…';
  try {
    const response=await fetch('/api/focus',{method:'POST',headers:{Authorization:`Bearer ${token}`,'Content-Type':'application/json'},body:JSON.stringify({agentId:id}),signal:AbortSignal.timeout(7000)});
    const result=await response.json();$('interaction-status').textContent=response.ok?'Session opened.':result.message || 'Could not open the session.';
  } catch {$('interaction-status').textContent='Could not confirm the session opened. Check the terminal before retrying.';}
  finally {focusPending=false;$('session-open').disabled=!connected;}
}
function updateBubbles(now) {
  if(!view)return;
  const visible=new Set(),items=[];
  for(const actor of orderedAgents.filter(actor=>advanceBubble(actor,now))) {
    const anchor=view.project(actor.data.id,2.45);
    if(!anchor?.visible)continue;
    let node=speechNodes.get(actor.data.id);
    if(!node){
      node=document.createElement('button');node.type='button';node.className='speech-bubble';node.dataset.overlayAgent=actor.data.id;
      const source=document.createElement('span');source.className='bubble-source';source.setAttribute('aria-hidden','true');
      const text=document.createElement('span');text.className='bubble-text';
      const stem=document.createElement('span');stem.className='overlay-stem';stem.setAttribute('aria-hidden','true');node.append(source,text,stem);
      node.addEventListener('click',()=>{selectActor(actor.data.id);if(actor.bubble?.questionId&&actor.data.questions?.some(q=>q.id===actor.bubble.questionId))questions.open(actor.data);});
      $('bubbles').append(node);speechNodes.set(actor.data.id,node);
    }
    if(node.dataset.key!==actor.bubble.key){
      node.dataset.key=actor.bubble.key;node.children[0].textContent=actor.bubble.questionId?'?':'•';node.children[1].textContent=actor.bubble.text;
    }
    node.title=`${identityLabel(actor.data)} · ${actor.bubble.text}`;node.setAttribute('aria-label',node.title);
    node.classList.toggle('selected',actor.data.id===selectedID);node.classList.toggle('has-question',Boolean(actor.bubble.questionId));
    node.hidden=false;visible.add(actor.data.id);items.push({node,anchor});
  }
  const boxes=placeOverlays(items.map(({node,anchor})=>({node,anchor,x:anchor.x,y:anchor.y-node.offsetHeight-10,width:node.offsetWidth,height:node.offsetHeight})),{width:stage.clientWidth,height:stage.clientHeight,occupied:view.overlayBounds(),direction:-1});
  for(const box of boxes)positionOverlay(box.node,box,box.anchor);
  for(const [id,node] of speechNodes)if(!visible.has(id)){node.remove();speechNodes.delete(id);}
}
function showRenderingError(message) {
  renderingError=message;$('connection-warning').hidden=false;$('connection-warning').textContent=message;
}
$('session-open').addEventListener('click',()=>openSession());
canvas.addEventListener('keydown',event=>{
  if(event.ctrlKey||event.metaKey||event.altKey)return;
  if(event.key.toLowerCase()==='f'){event.preventDefault();view?.focusGym();}
  else if(event.key==='Escape'){event.preventDefault();selectActor(null);}
  else if(event.key==='Enter'){event.preventDefault();const data=actors.get(selectedID)?.data;if(data?.questions?.length)questions.open(data);else openSession();}
});
document.addEventListener('visibilitychange',()=>{if(!document.hidden)poll();});
window.addEventListener('pagehide',event=>{clearTimeout(pollTimer);if(!event.persisted)view?.dispose();});

async function start(){
  try {
    const {createGymView}=await import('./assets/gym/view.js');
    view=createGymView({canvas,stage,labels:$('agent-labels'),onSelect:selectActor,onOpen:openSession,onFrame:updateBubbles,onError:showRenderingError,reducedMotion:reducedMotion.matches});
    // Read-only diagnostics help verify live placement without exposing credentials or message contents.
    window.oakGym={getInfo:()=>view.getInfo()};
  } catch(error) {console.error('Gym rendering unavailable:',error);showRenderingError('3D rendering is unavailable. You can still use the agent list, questions, notes and session controls.');}
  if(!token){$('stage-message').textContent='Open this gym with oak-tree office to connect your agents.';$('connection').textContent='Not connected';return;}
  await poll();
}
start();
