import * as T from './vendor/three.module.js';

export function createTrainingEquipment(art, station, parent) {
  const root = new T.Group(), structure = new T.Group();
  root.position.set(station.position.x, 0, station.position.z);
  root.add(structure); parent.add(root);
  const {iron, edge, dark, gold, woodLight, rubber, cloth} = art;
  const box = (w,h,d,x,y,z,material=iron,group=structure) => art.box(w,h,d,x,y,z,material,group);
  let bar, stack;
  const arms = [], up = new T.Vector3(0,1,0);
  function barbell(y) {
    const group = new T.Group(); root.add(group); group.position.y = y;
    box(2.8,.055,.055,0,0,0,edge,group);
    for (let i=0;i<15;i++) box(.018,.06,.06,-.6+i*.085,0,0,iron,group);
    for (const side of [-1,1]) {
      art.plate(side*1.02,0,0,.3,'#506b59',group);
      art.plate(side*1.19,0,0,.25,'#aa8f57',group);
      box(.08,.12,.12,side*1.32,0,0,gold,group);
    }
    art.batch(group); return group;
  }
  if (station.kind !== 'pec-deck') {
    box(3.25,.1,2.65,0,.03,-.1,dark);
    for (const side of [-1,1]) box(.85,.045,2.55,side*1.15,.095,-.1,rubber);
    for (let i=0;i<5;i++) box(.29,.045,2.55,-.6+i*.3,.095,-.1,i%2?art.wood:woodLight);
    art.decal(station.kind==='squat-rack'?'SQUAT':'DEADLIFT','',0,.122,1.02,1.2,.26,{parent:structure,rotation:[-Math.PI/2,0,0],font:12});
    if (station.kind === 'squat-rack') {
      for (const x of [-1.22,1.22]) {
        for (const z of [-1.05,.45]) {
          box(.13,2.6,.13,x,1.43,z); box(.35,.08,.4,x,.18,z);
          for (let y=.55;y<2.5;y+=.18) box(.04,.04,.01,x,y,z+.07,dark);
          for (const dx of [-.1,.1]) box(.035,.025,.035,x+dx,.23,z,edge);
        }
        box(.13,.13,1.65,x,2.69,-.3);
        box(.1,.1,1.6,x,.85,-.3);
        box(.14,.09,.65,x,1.49,.15);
        box(.2,.16,.12,x,1.55,-.12,edge);
      }
      for (const z of [-1.05,.45]) box(2.57,.13,.13,0,2.69,z);
      box(2.4,.045,.045,0,2.5,.45,edge);
    }
    bar = barbell(station.kind==='squat-rack'?1.54:.415);
  } else {
    box(1.7,.12,1.8,0,.08,-.2,dark);
    for (const x of [-.8,.8]) {
      box(.12,2.35,.12,x,1.22,-.65);
      box(.22,.08,1.7,x,.16,-.1);
      box(.12,.12,.72,x,1.26,-.3);
    }
    box(1.72,.12,.12,0,2.4,-.65);
    box(.62,.13,.6,0,.43,.04,cloth);
    box(.12,.32,.12,0,.24,0);
    box(.66,.9,.16,0,1.02,-.25,cloth);
    for (const x of [-.29,.29]) box(.025,.8,.015,x,1.02,-.16,woodLight);
    box(.74,1.7,.18,1,1.12,-.87,dark);
    for (const x of [.76,1.24]) box(.035,1.7,.035,x,1.22,-.73,edge);
    box(.025,1.3,.025,1,1.55,-.73,edge);
    stack = new T.Group(); root.add(stack);
    for (let i=0;i<6;i++) {
      box(.59,.055,.28,0,i*.065,0,i%2?iron:edge,stack);
      box(.08,.025,.025,.24,i*.065,.15,gold,stack);
    }
    for (const side of [-1,1]) {
      const hinge = new T.Vector3(side*.4,1.23,-.12);
      box(.18,.18,.18,hinge.x,hinge.y,hinge.z,edge);
      const beam = box(.08,1,.08,0,0,0,iron,root);
      const grip = new T.Group(); root.add(grip);
      box(.08,.28,.08,0,0,0,dark,grip);
      box(.13,.045,.13,0,-.16,0,gold,grip);
      art.batch(grip); arms.push({side,hinge,beam,grip});
    }
    art.decal('PEC DECK','',0,.16,.76,1.1,.24,{parent:structure,font:12});
  }
  art.batch(structure);
  const left = new T.Vector3(), right = new T.Vector3(), middle = new T.Vector3();
  function update(actor, pose) {
    if (actor) {
      actor.root.updateMatrixWorld(true);
      actor.leftArm.elbow.localToWorld(left.set(0,-.41,.025));
      actor.rightArm.elbow.localToWorld(right.set(0,-.41,.025));
      root.worldToLocal(left); root.worldToLocal(right);
    }
    if (bar) {
      if (actor && pose.barHeld) {
        bar.position.copy(middle.copy(left).add(right).multiplyScalar(.5));
        bar.rotation.y = actor.root.rotation.y;
      } else {bar.position.set(0,station.kind==='squat-rack'?1.54:.415,station.kind==='squat-rack'?-.1:0);bar.rotation.y=0;}
    }
    if (stack) stack.position.set(1,.4+(actor?pose.machineLift*.3:0),-.73);
    for (const arm of arms) {
      const hand = actor ? (arm.side<0?left:right) : middle.set(arm.side*1.02,1.34,.3);
      arm.grip.position.copy(hand);
      const direction = hand.clone().sub(arm.hinge);
      arm.beam.position.copy(arm.hinge).add(hand).multiplyScalar(.5);
      arm.beam.scale.y = direction.length();
      arm.beam.quaternion.setFromUnitVectors(up,direction.normalize());
    }
  }
  update(null); return {root,update};
}
