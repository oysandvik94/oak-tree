import * as T from './vendor/three.module.js';
import {exercises} from './exercises.js';

export function createTrainingEquipment(art, station, parent) {
  const root=new T.Group(),structure=new T.Group(),kind=station.kind;
  root.position.set(station.position.x,0,station.position.z);root.add(structure);parent.add(root);
  const {iron,edge,dark,gold,woodLight,rubber,cloth}=art;
  const box=(w,h,d,x,y,z,material=iron,group=structure)=>art.box(w,h,d,x,y,z,material,group);
  const arms=[],up=new T.Vector3(0,1,0);
  let bar,stack,footplate,roller,cable;
  function barbell(y,width=2.8){
    const group=new T.Group();root.add(group);group.position.y=y;
    box(width,.055,.055,0,0,0,edge,group);
    for(const side of [-1,1]){
      art.plate(side*(width/2-.38),0,0,.3,'#506b59',group);
      art.plate(side*(width/2-.21),0,0,.25,'#aa8f57',group);
      box(.08,.12,.12,side*(width/2-.08),0,0,gold,group);
    }
    art.batch(group);return group;
  }
  function seat(z=0){
    box(.75,.14,.72,0,.45,z,cloth);box(.14,.4,.14,0,.24,z);
    box(.75,.9,.16,0,1.02,z-.32,cloth);
    for(const x of [-.7,.7]){box(.1,.1,1.5,x,.16,z);box(.1,.8,.1,x,.52,z-.45);}
  }
  function weights(z=-.9){
    box(.74,1.8,.18,1.1,1.1,z,dark);
    for(const x of [.86,1.34])box(.035,1.8,.035,x,1.2,z+.13,edge);
    stack=new T.Group();root.add(stack);stack.userData.z=z+.14;
    for(let i=0;i<7;i++){box(.59,.055,.28,0,i*.065,0,i%2?iron:edge,stack);box(.08,.025,.025,.24,i*.065,.15,gold,stack);}
    art.batch(stack);
  }
  function frame(height=2.5){
    for(const x of [-.8,.8]){box(.12,height,.12,x,height/2+.15,-.8);box(.22,.08,2,x,.16,0);}
    box(1.72,.12,.12,0,height+.15,-.8);
  }
  const platform=['squat-rack','deadlift-platform','bench-press'].includes(kind);
  box(platform?3.25:2.8,.1,platform?2.65:3,0,.03,platform?-.1:0,dark);
  if(platform){
    for(const side of [-1,1])box(.85,.045,2.55,side*1.15,.095,-.1,rubber);
    for(let i=0;i<5;i++)box(.29,.045,2.55,-.6+i*.3,.095,-.1,i%2?art.wood:woodLight);
  }
  if(kind==='squat-rack'){
    for(const x of [-1.22,1.22]){
      for(const z of [-1.05,.45]){box(.13,2.6,.13,x,1.43,z);box(.35,.08,.4,x,.18,z);for(let y=.55;y<2.5;y+=.18)box(.04,.04,.01,x,y,z+.07,dark);}
      box(.13,.13,1.65,x,2.69,-.3);box(.1,.1,1.6,x,.85,-.3);box(.14,.09,.65,x,1.49,.15);
    }
    for(const z of [-1.05,.45])box(2.57,.13,.13,0,2.69,z);
    bar=barbell(1.54);
  }else if(kind==='deadlift-platform'){
    bar=barbell(.415);
  }else if(kind==='bench-press'){
    box(.75,.17,2.1,0,.49,-1.08,cloth);
    for(const z of [-1.8,-.25]){box(.14,.4,.14,0,.25,z);box(1,.1,.12,0,.12,z);}
    for(const x of [-1.1,1.1]){box(.13,1.35,.13,x,.77,-1.4);box(.3,.1,.35,x,1.41,-1.3);}
    bar=barbell(1.45);
  }else if(kind==='pec-deck'){
    seat();frame(2.25);weights();
    for(const side of [-1,1]){
      const hinge=new T.Vector3(side*.4,1.23,-.12);
      box(.18,.18,.18,hinge.x,hinge.y,hinge.z,edge);
      const beam=box(.08,1,.08,0,0,0,iron,root),grip=new T.Group();root.add(grip);
      box(.08,.28,.08,0,0,0,dark,grip);box(.13,.045,.13,0,-.16,0,gold,grip);
      art.batch(grip);arms.push({side,hinge,beam,grip});
    }
  }else if(kind==='leg-press'){
    seat(-.15);
    for(const x of [-.8,.8]){
      const rail=box(.1,.1,2.4,x,.65,1.05,edge);rail.rotation.x=-.35;
      box(.13,1.45,.13,x,.75,1.95);box(.28,.1,.5,x,.12,1.95);
    }
    footplate=new T.Group();root.add(footplate);
    box(1.45,.85,.12,0,0,0,rubber,footplate);
    for(const x of [-.95,.95]){box(.35,.08,.08,x,0,0,edge,footplate);art.plate(x,0,0,.32,'#506b59',footplate);}
    art.batch(footplate);
  }else if(kind==='lat-pulldown'){
    seat();frame(2.55);weights(-.95);bar=new T.Group();root.add(bar);
    box(1.5,.06,.06,0,0,0,edge,bar);
    for(const side of [-1,1]){const grip=box(.55,.085,.085,side*.9,-.08,0,dark,bar);grip.rotation.z=-side*.3;}
    art.batch(bar);box(.07,.07,1.7,0,2.65,0,edge);
    for(const x of [-.35,.35])box(.33,.16,.34,x,.83,.4,cloth);
    cable=box(.025,1,.025,0,0,0,edge,root);
  }else if(kind==='seated-row'){
    seat(-.2);weights(1.25);
    for(const x of [-.4,.4]){const pad=box(.35,.45,.12,x,.3,1.1,rubber);pad.rotation.x=-.3;}
    bar=new T.Group();root.add(bar);box(.65,.06,.06,0,0,0,edge,bar);
    for(const x of [-.25,.25])box(.06,.25,.06,x,0,.07,dark,bar);
    art.batch(bar);cable=box(.025,1,.025,0,0,0,edge,root);
  }else if(kind==='leg-extension'){
    seat();weights();roller=new T.Group();root.add(roller);
    box(.95,.22,.28,0,0,0,cloth,roller);box(1.2,.06,.06,0,0,0,edge,roller);art.batch(roller);
    for(const x of [-.55,.55])box(.06,.06,.6,x,.67,0,edge);
  }
  art.decal(exercises[kind].label.toUpperCase(),'',0,.15,kind==='bench-press'?1.1:1.65,2.5,.25,{parent:structure,rotation:[-Math.PI/2,0,0],font:12});
  art.batch(structure);
  const left=new T.Vector3(),right=new T.Vector3(),middle=new T.Vector3();
  function update(actor,pose){
    if(actor){
      actor.root.updateMatrixWorld(true);
      actor.leftArm.elbow.localToWorld(left.set(0,-.41,.025));actor.rightArm.elbow.localToWorld(right.set(0,-.41,.025));
      root.worldToLocal(left);root.worldToLocal(right);
    }
    if(bar){
      if(actor&&!pose.released&&(pose.barHeld||kind==='seated-row')){
        bar.position.copy(middle.copy(left).add(right).multiplyScalar(.5));bar.rotation.y=actor.root.rotation.y;
      }else{
        const y=kind==='squat-rack'?1.54:kind==='bench-press'?1.45:kind==='lat-pulldown'?2.15:kind==='seated-row'? .95 : .415;
        bar.position.set(0,y,kind==='bench-press'?-1.4:kind==='seated-row'?.9:0);bar.rotation.y=0;
      }
    }
    if(stack)stack.position.set(1.1,.35+(actor?pose.machineLift*.3:0),stack.userData.z);
    for(const arm of arms){
      const hand=actor&&!pose.released?(arm.side<0?left:right):middle.set(arm.side*1.02,1.34,.3);
      arm.grip.position.copy(hand);const direction=hand.clone().sub(arm.hinge);
      arm.beam.position.copy(arm.hinge).add(hand).multiplyScalar(.5);arm.beam.scale.y=direction.length();
      arm.beam.quaternion.setFromUnitVectors(up,direction.normalize());
    }
    if(footplate||roller){
      if(actor&&!pose.released){
        actor.leftLeg.knee.localToWorld(left.set(0,-.42,.12));actor.rightLeg.knee.localToWorld(right.set(0,-.42,.12));
        root.worldToLocal(left);root.worldToLocal(right);middle.copy(left).add(right).multiplyScalar(.5);
      }else middle.set(0,footplate ? .85 : .25,footplate ? 1.5 : .8);
      (footplate||roller).position.copy(middle);if(footplate)footplate.rotation.x=-.3;
    }
    if(cable){
      const anchor=kind==='lat-pulldown'?left.set(0,2.65,.1):left.set(0,.35,1.35);
      const direction=right.copy(bar.position).sub(anchor);
      cable.position.copy(anchor).add(bar.position).multiplyScalar(.5);cable.scale.y=direction.length();
      cable.quaternion.setFromUnitVectors(up,direction.normalize());
    }
  }
  update(null);return {root,update};
}
