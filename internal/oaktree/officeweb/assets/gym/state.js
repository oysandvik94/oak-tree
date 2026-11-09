export const zones = [
  {id: 'working', name: 'Training', color: '#e5be66'},
  {id: 'question', name: 'Coaching', color: '#83d2da'},
  {id: 'ready', name: 'Recovery', color: '#9bcc9c'},
  {id: 'review', name: 'Review', color: '#c5a4d8'},
  {id: 'blocked', name: 'Help bay', color: '#ed9992'},
];

// Manual/group parking wins; otherwise show the individual, not its busy colleague.
export function agentState(data) {
  if (data.sessionState === 'review' || data.sessionState === 'blocked') return data.sessionState;
  return ['working', 'question', 'ready'].includes(data.status) ? data.status : 'unknown';
}

export function hash(value) {
  let result = 2166136261;
  for (const ch of value) result = Math.imul(result ^ ch.charCodeAt(0), 16777619);
  return result >>> 0;
}

export function layoutFor(capacity) {
  const trainingDepth = 10 + Math.max(0, Math.ceil(capacity.working / 3) - 2) * 2;
  const topDepth = Math.max(7, 3 + Math.ceil(Math.max(capacity.question, capacity.blocked) / 3) * 2);
  const corridor = Math.max(-5 + trainingDepth, -5 + topDepth) + 1.4;
  const bottomDepth = Math.max(7, 3 + Math.ceil(Math.max(capacity.ready, capacity.review) / 3) * 2);
  return {
    corridor,
    bounds: [-13, -5, 15, corridor + 1.4 + bottomDepth],
    working: {x: -7, z: 0, width: 12, depth: trainingDepth, back: -5, front: -5 + trainingDepth},
    question: {x: 3.5, z: -5 + topDepth / 2, width: 7, depth: topDepth, back: -5, front: -5 + topDepth},
    blocked: {x: 11.5, z: -5 + topDepth / 2, width: 7, depth: topDepth, back: -5, front: -5 + topDepth},
    ready: {x: 3.5, z: corridor + 1.4 + bottomDepth / 2, width: 7, depth: bottomDepth, back: corridor + 1.4, front: corridor + 1.4 + bottomDepth},
    review: {x: 11.5, z: corridor + 1.4 + bottomDepth / 2, width: 7, depth: bottomDepth, back: corridor + 1.4, front: corridor + 1.4 + bottomDepth},
  };
}

export function slotFor(layout, state, index) {
  if (state === 'unknown') return {x: -11 + (index % 12) * 2, z: layout.corridor + Math.floor(index / 12) * .6};
  const room = layout[state];
  return {x: room.x - 2.2 + (index % 3) * 2.2, z: room.back + (state === 'working' ? 7.3 : 3.1) + Math.floor(index / 3) * 2};
}

export function stationFor(layout, state, index) {
  const room = layout[state];
  if (state === 'ready' && index < 2) return {kind: 'bench', position: {x: room.x + (index ? .65 : -.65), z: room.front - 1.1}};
  if (state === 'review' && index < 2) return {kind: 'desk', position: {x: room.x + (index ? .7 : -.7), z: room.front - 1.5}};
  return {kind: 'floor', position: slotFor(layout, state, index - (state === 'ready' || state === 'review' ? 2 : 0))};
}

// Each slot has a clear column to the shared corridor. No path crosses the rack or benches.
export function routeTo(start, target, corridor) {
  const path = [{x: start.x, z: corridor}, {x: target.x, z: corridor}, target];
  return path.filter((point, index) => Math.hypot(point.x - (index ? path[index - 1].x : start.x), point.z - (index ? path[index - 1].z : start.z)) > .01);
}
