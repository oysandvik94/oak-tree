import {equipmentExercises, exercises, shuffled} from './exercises.js';

export const zones = [
  {id: 'working', name: 'Training', color: '#e5be66'},
  {id: 'question', name: 'Coaching', color: '#83d2da'},
  {id: 'ready', name: 'Recovery', color: '#9bcc9c'},
  {id: 'review', name: 'Review', color: '#c5a4d8'},
  {id: 'blocked', name: 'Help bay', color: '#ed9992'},
];

export function gymAgents(data) {
  const sessions = new Map();
  for(const agent of data)if(agent.sessionId){if(!sessions.has(agent.sessionId))sessions.set(agent.sessionId,[]);sessions.get(agent.sessionId).push(agent);}
  return data.map(agent=>{
    const peers=sessions.get(agent.sessionId)||[],active=peers.find(peer=>peer.id!==agent.id&&['architect','junior'].includes(peer.role)&&agentState(peer)==='working');
    const spotting=agentState(agent)==='ready'&&['architect','junior'].includes(agent.role)&&active?active.id:null;
    return {...agent,spotting,sessionBusy:peers.some(peer=>agentState(peer)==='working')};
  });
}

// Preserve attention/parking states; an idle partner supports the active lifter.
export function agentState(data) {
  if (data.sessionState === 'review' || data.sessionState === 'blocked') return data.sessionState;
  if(data.spotting)return 'working';
  return ['working', 'question', 'ready'].includes(data.status) ? data.status : 'unknown';
}

export function hash(value) {
  let result = 2166136261;
  for (const ch of value) result = Math.imul(result ^ ch.charCodeAt(0), 16777619);
  return result >>> 0;
}

export function layoutFor(capacity) {
  const trainingDepth = 18 + Math.max(0, Math.ceil(capacity.working / 4) - 2) * 2.2;
  const topDepth = Math.max(7, 3 + Math.ceil(Math.max(capacity.question, capacity.blocked) / 3) * 2);
  const topFront = Math.max(-5 + trainingDepth, -5 + topDepth), corridor = topFront + 1.4;
  const bottomDepth = Math.max(7, 3 + Math.ceil(Math.max(capacity.ready, capacity.review) / 3) * 2);
  const layout = {
    corridor,
    bounds: [-25, -5, 15, corridor + 1.4 + bottomDepth],
    working: {x: -13, z: (topFront - 5) / 2, width: 24, depth: topFront + 5, back: -5, front: topFront},
    question: {x: 3.5, z: topFront - topDepth / 2, width: 7, depth: topDepth, back: topFront - topDepth, front: topFront},
    blocked: {x: 11.5, z: topFront - topDepth / 2, width: 7, depth: topDepth, back: topFront - topDepth, front: topFront},
    ready: {x: 3.5, z: corridor + 1.4 + bottomDepth / 2, width: 7, depth: bottomDepth, back: corridor + 1.4, front: corridor + 1.4 + bottomDepth},
    review: {x: 11.5, z: corridor + 1.4 + bottomDepth / 2, width: 7, depth: bottomDepth, back: corridor + 1.4, front: corridor + 1.4 + bottomDepth},
  };
  layout.equipment = equipmentExercises.map(({id}, index) => {
    const position = {x: layout.working.x - 9 + index % 4 * 6, z: -2 + Math.floor(index / 4) * 6};
    return {id,kind:id,position,surface:['squat-rack','deadlift-platform','bench-press'].includes(id) ? .09 : 0,
      front:{x:position.x,z:position.z+2.4},approach:{x:position.x+2.3,z:position.z+2.4}};
  });
  return layout;
}

export function slotFor(layout, state, index) {
  if (state === 'unknown') return {x: -11 + (index % 12) * 2, z: layout.corridor + Math.floor(index / 12) * .6};
  const room = layout[state];
  return state === 'working'
    ? {x: room.x - 9 + index % 4 * 6, z: 9.3 + Math.floor(index / 4) * 2.2}
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
  return Boolean(exercises[station.kind]?.equipment);
}

export function stationRoute(layout, start, station) {
  const path = [], sofa = layout.ready, sofaFront = sofa.front - .3;
  let cursor = start;
  function visit(point) {
    if (Math.hypot(point.x - cursor.x, point.z - cursor.z) > .01) path.push(point);
    cursor = point;
  }
  // Use actual coordinates: an agent can change status again before arriving.
  const leaving = layout.equipment.find(item => Math.abs(cursor.x-item.position.x)<1.7 && cursor.z>=item.position.z-(item.kind==='bench-press'?2.6:1.6) && cursor.z<=item.front.z+.01);
  if(leaving){
    if(cursor.z<leaving.position.z-.1||Math.abs(cursor.x-leaving.position.x)>.7){
      const side=cursor.x<leaving.position.x?-1:1;
      visit({x:leaving.position.x+side*2.3,z:cursor.z});
      visit({x:cursor.x,z:leaving.front.z});
    }else{visit({x:cursor.x,z:leaving.front.z});visit(leaving.approach);}
  }
  if (Math.abs(cursor.x - sofa.x) < 1.9 && cursor.z > sofa.front - 1.6) {
    visit({x: cursor.x, z: sofaFront});
    visit({x: sofa.x + (cursor.x < sofa.x ? -2.2 : 2.2), z: sofaFront});
  }
  if(station.kind==='spotter'&&station.equipmentId){
    const machine=layout.equipment.find(item=>item.id===station.equipmentId),side=station.position.x<machine.position.x?-1:1;
    const aisle=machine.position.x+side*2.3;
    routeTo(cursor,{x:aisle,z:machine.front.z},layout.corridor).forEach(visit);
    visit({x:aisle,z:station.position.z});visit(station.position);return path;
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
  const due = workers.filter(worker => !worker.event && (!worker.path.length || starting.has(worker.data.id)) && time >= worker.rotationAt && (!paused || starting.has(worker.data.id)))
    .sort((a,b) => a.rotationAt - b.rotationAt || a.data.id.localeCompare(b.data.id));
  for (const worker of due) {
    if (isEquipment(worker.station)) {
      changes.push({worker, station: worker.floorStation});
      continue;
    }
    if(!worker.exerciseOrder?.length)worker.exerciseOrder=shuffled(equipment.map(item=>item.id));
    const ordered=worker.exerciseOrder.map(id=>equipment.find(item=>item.id===id));
    const station = ordered.filter(item=>item.id!==worker.lastEquipment).find(item => !occupied.has(item.id) && !occupants.some(actor =>
      Math.abs(actor.position.x - item.position.x) < 1.7 && actor.position.z >= item.position.z - .3 && actor.position.z <= item.front.z + .3));
    if(station){worker.exerciseOrder=worker.exerciseOrder.filter(id=>id!==station.id);occupied.add(station.id);changes.push({worker,station});}
  }
  return changes;
}
