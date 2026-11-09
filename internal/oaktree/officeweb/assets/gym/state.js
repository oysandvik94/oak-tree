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
  const trainingDepth = 16 + Math.max(0, Math.ceil(capacity.working / 3) - 2) * 2.2;
  const topDepth = Math.max(7, 3 + Math.ceil(Math.max(capacity.question, capacity.blocked) / 3) * 2);
  const topFront = Math.max(-5 + trainingDepth, -5 + topDepth), corridor = topFront + 1.4;
  const bottomDepth = Math.max(7, 3 + Math.ceil(Math.max(capacity.ready, capacity.review) / 3) * 2);
  const layout = {
    corridor,
    bounds: [-19, -5, 15, corridor + 1.4 + bottomDepth],
    working: {x: -10, z: (topFront - 5) / 2, width: 18, depth: topFront + 5, back: -5, front: topFront},
    question: {x: 3.5, z: topFront - topDepth / 2, width: 7, depth: topDepth, back: topFront - topDepth, front: topFront},
    blocked: {x: 11.5, z: topFront - topDepth / 2, width: 7, depth: topDepth, back: topFront - topDepth, front: topFront},
    ready: {x: 3.5, z: corridor + 1.4 + bottomDepth / 2, width: 7, depth: bottomDepth, back: corridor + 1.4, front: corridor + 1.4 + bottomDepth},
    review: {x: 11.5, z: corridor + 1.4 + bottomDepth / 2, width: 7, depth: bottomDepth, back: corridor + 1.4, front: corridor + 1.4 + bottomDepth},
  };
  layout.equipment = Array.from({length: 6}, (_, index) => {
    const kind = ['squat-rack', 'deadlift-platform', 'pec-deck'][index % 3];
    const position = {x: layout.working.x - 6 + index % 3 * 6, z: -2 + Math.floor(index / 3) * 5};
    return {id: `${kind}-${Math.floor(index / 3)}`, kind, position, surface: kind === 'pec-deck' ? 0 : .09,
      front: {x: position.x, z: position.z + 1.8}, approach: {x: position.x + 2, z: position.z + 1.8}};
  });
  return layout;
}

export function slotFor(layout, state, index) {
  if (state === 'unknown') return {x: -11 + (index % 12) * 2, z: layout.corridor + Math.floor(index / 12) * .6};
  const room = layout[state];
  return state === 'working'
    ? {x: room.x - 6 + index % 3 * 6, z: 7.7 + Math.floor(index / 3) * 2.2}
    : {x: room.x - 2.2 + (index % 3) * 2.2, z: room.back + 3.1 + Math.floor(index / 3) * 2};
}

export function stationFor(layout, state, index) {
  const room = layout[state];
  if (state === 'ready' && index < 2) return {kind: 'bench', position: {x: room.x + (index ? .65 : -.65), z: room.front - 1.1}};
  if (state === 'review' && index < 2) return {kind: 'desk', position: {x: room.x + (index ? .7 : -.7), z: room.front - 1.5}};
  return {kind: 'floor', position: slotFor(layout, state, index - (state === 'ready' || state === 'review' ? 2 : 0))};
}

// Room-to-room travel uses the shared corridor; furniture approaches are added below.
export function routeTo(start, target, corridor) {
  const path = [{x: start.x, z: corridor}, {x: target.x, z: corridor}, target];
  return path.filter((point, index) => Math.hypot(point.x - (index ? path[index - 1].x : start.x), point.z - (index ? path[index - 1].z : start.z)) > .01);
}

export function isEquipment(station) {
  return ['squat-rack', 'deadlift-platform', 'pec-deck'].includes(station.kind);
}

export function stationRoute(layout, start, station) {
  const path = [], sofa = layout.ready, sofaFront = sofa.front - .3;
  let cursor = start;
  function visit(point) {
    if (Math.hypot(point.x - cursor.x, point.z - cursor.z) > .01) path.push(point);
    cursor = point;
  }
  // Use actual coordinates: an agent can change status again before arriving.
  const leaving = layout.equipment.find(item => Math.abs(cursor.x - item.position.x) < 1.7 && cursor.z >= item.position.z - .1 && cursor.z <= item.front.z + .01);
  if (leaving) {
    visit({x: cursor.x, z: leaving.front.z});
    visit(leaving.approach);
  }
  if (Math.abs(cursor.x - sofa.x) < 1.9 && cursor.z > sofa.front - 1.6) {
    visit({x: cursor.x, z: sofaFront});
    visit({x: sofa.x + (cursor.x < sofa.x ? -2.2 : 2.2), z: sofaFront});
  }
  const entry = isEquipment(station) ? station.approach : station.kind === 'bench'
    ? {x: sofa.x + (station.position.x < sofa.x ? -2.2 : 2.2), z: sofaFront} : station.position;
  routeTo(cursor, entry, layout.corridor).forEach(visit);
  if (isEquipment(station)) visit(station.front);
  if (station.kind === 'bench') visit({x: station.position.x, z: sofaFront});
  visit(station.position);
  return path;
}

export function trainingRotations(workers, equipment, time, occupants = workers, {starting = new Set(), paused = false} = {}) {
  const occupied = new Set(workers.map(worker => worker.station.id));
  const changes = [];
  const due = workers.filter(worker => (!worker.path.length || starting.has(worker.data.id)) && time >= worker.rotationAt && (!paused || starting.has(worker.data.id)))
    .sort((a,b) => a.rotationAt - b.rotationAt || a.data.id.localeCompare(b.data.id));
  for (const worker of due) {
    if (isEquipment(worker.station)) {
      changes.push({worker, station: worker.floorStation});
      continue;
    }
    const previous = equipment.findIndex(item => item.id === worker.lastEquipment);
    const ordered = equipment.map((_, index) => equipment[(previous + 1 + index) % equipment.length]);
    const station = ordered.find(item => !occupied.has(item.id) && !occupants.some(actor =>
      Math.abs(actor.position.x - item.position.x) < 1.7 && actor.position.z >= item.position.z - .3 && actor.position.z <= item.front.z + .3));
    if (station) {occupied.add(station.id); changes.push({worker, station});}
  }
  return changes;
}
