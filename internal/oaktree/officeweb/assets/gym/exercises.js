export const gymEventTypes = ['tool_failed', 'question_raised', 'session_ready'];
export const trainingSetDuration = identity => 25 + identity % 5;

function seated(pose) {
  pose.bodyY = -.32; pose.lean = 0;
  pose.leftHip = pose.rightHip = -Math.PI / 2;
  pose.leftKnee = pose.rightKnee = Math.PI / 2;
}
function release(pose) {
  pose.weights = pose.bottle = pose.towel = pose.barHeld = false;
  pose.handTargets = null; pose.machineLift = 0; pose.released = true;
}
function ask(pose, phase) {
  release(pose); pose.headPitch = -.15;
  pose.rightShoulderZ = -2.7 + Math.sin(phase * 15) * .12;
  pose.rightElbow = -.4;
}
function celebrate(pose, phase) {
  release(pose);
  pose.leftShoulderZ = 2.6; pose.rightShoulderZ = -2.6;
  pose.leftElbow = pose.rightElbow = -.5;
  pose.sway = Math.sin(phase * 20) * .08;
}
function strain(pose, phase) {
  release(pose); pose.lean += Math.sin(phase * Math.PI) * .4;
  pose.headPitch = .25; pose.sway = Math.sin(phase * 45) * .07;
  pose.leftElbow = pose.rightElbow = -1.8;
}
function define(id, label, sample, fail, spotter = {x:-1.15,z:.25,facing:Math.PI/2}, equipment = true) {
  return {id,label,equipment,spotter,sample,events:{tool_failed:fail,question_raised:ask,session_ready:celebrate}};
}
export const exercises = Object.fromEntries([
  define('squat-rack','Barbell squat',(pose,{rep})=>{
    const bend = rep * .85;
    pose.leftHip = pose.rightHip = -bend; pose.leftKnee = pose.rightKnee = bend * 2;
    pose.bodyY = -.93 * (1 - Math.cos(bend)); pose.lean = rep * .24;
    pose.handTargets = [[-.75,.69,-.12],[.75,.69,-.12]]; pose.barHeld = true;
  },(pose,phase)=>{
    release(pose); const fall = Math.sin(Math.PI * phase);
    pose.bodyY = -.45 * fall; pose.bodyRoll = .95 * fall; pose.lean = .5 * fall;
    pose.leftKnee = pose.rightKnee = 1.4 * fall;
    pose.leftShoulderZ = 1.5; pose.rightShoulderZ = -1.5;
  },{x:0,z:-1.4,facing:0}),
  define('deadlift-platform','Deadlift',(pose,{rep,resting})=>{
    const bend = resting ? 0 : 1 - rep;
    pose.leftHip = pose.rightHip = -bend * 1.1; pose.leftKnee = pose.rightKnee = bend * 1.5;
    pose.bodyY = .44*Math.cos(bend*1.1)+.49*Math.cos(bend*.4)-.93;
    pose.bodyZ = -.44*Math.sin(bend*1.1)+.49*Math.sin(bend*.4);
    pose.lean = bend*.72; pose.leftShoulderX = pose.rightShoulderX = -pose.lean;
    pose.leftShoulderZ = pose.rightShoulderZ = pose.leftElbow = pose.rightElbow = 0;
    pose.barHeld = !resting;
  },(pose,phase)=>{
    release(pose); pose.lean = .65 + Math.sin(phase * Math.PI) * .35;
    pose.headPitch = -.2; pose.rightShoulderX = .6; pose.rightElbow = -2.2;
    pose.leftShoulderX = -.5; pose.leftElbow = -.2;
    pose.sway = Math.sin(phase * 35) * .06;
  }),
  define('pec-deck','Pec-deck fly',(pose,{rep})=>{
    seated(pose); pose.leftShoulderX = pose.rightShoulderX = -Math.PI/2;
    pose.leftShoulderZ = -1.2+rep*1.35; pose.rightShoulderZ = -pose.leftShoulderZ;
    pose.leftElbow = pose.rightElbow = -.35; pose.machineLift = rep;
  },(pose,phase)=>{
    strain(pose,phase); pose.leftShoulderZ = -1.4; pose.rightShoulderZ = 1.4;
    pose.leftElbow = pose.rightElbow = -.35;
  }),
  define('bench-press','Bench press',(pose,{rep})=>{
    pose.bodyPitch = -Math.PI/2; pose.bodyY = .59;
    pose.leftHip = pose.rightHip = Math.PI/2; pose.leftKnee = pose.rightKnee = Math.PI/2;
    pose.leftShoulderX = pose.rightShoulderX = -Math.PI/2;
    pose.leftShoulderZ = pose.rightShoulderZ = 0;
    pose.leftElbow = pose.rightElbow = -1.3*(1-rep); pose.barHeld = true;
  },(pose,phase)=>{
    pose.barHeld = true; pose.leftElbow = pose.rightElbow = -1.4;
    pose.sway = Math.sin(phase*50)*.055;
    if(phase>.6){release(pose);pose.leftShoulderZ=1.1;pose.rightShoulderZ=-1.1;}
  },{x:0,z:-2.35,facing:0}),
  define('leg-press','Leg press',(pose,{rep})=>{
    pose.bodyY = -.23; pose.bodyPitch = -.3; pose.bodyZ = .25; pose.lean = 0;
    pose.leftHip = pose.rightHip = -1.8+rep*.15;
    pose.leftKnee = pose.rightKnee = 1.6-rep*1.3;
    pose.leftElbow = pose.rightElbow = -1; pose.machineLift = rep;
  },(pose,phase)=>{
    strain(pose,phase); pose.leftKnee = pose.rightKnee = 1.9;
    pose.headPitch = -.3; pose.machineLift = 0;
  }),
  define('lat-pulldown','Lat pulldown',(pose,{rep})=>{
    seated(pose); pose.leftShoulderX = pose.rightShoulderX = -Math.PI+rep*2.1;
    pose.leftShoulderZ = .15; pose.rightShoulderZ = -.15;
    pose.leftElbow = pose.rightElbow = -rep*1.3; pose.barHeld = true; pose.machineLift = rep;
  },(pose,phase)=>{
    strain(pose,phase); pose.leftShoulderX = pose.rightShoulderX = -Math.PI;
    pose.leftElbow = pose.rightElbow = -.1;
    pose.bodyY += Math.sin(phase*Math.PI)*.25;
  }),
  define('seated-row','Seated cable row',(pose,{rep})=>{
    seated(pose); pose.leftHip = pose.rightHip = -1.35;
    pose.leftKnee = pose.rightKnee = .9; pose.lean = -.12*rep;
    pose.leftShoulderX = pose.rightShoulderX = -Math.PI/2+rep*.9;
    pose.leftElbow = pose.rightElbow = -rep*1.6; pose.machineLift = rep;
  },(pose,phase)=>{
    strain(pose,phase); pose.lean = .55; pose.leftShoulderX = pose.rightShoulderX = -1.5;
    pose.leftElbow = pose.rightElbow = -.1;
  }),
  define('leg-extension','Leg extension',(pose,{rep})=>{
    seated(pose); pose.leftKnee = pose.rightKnee = Math.PI/2*(1-rep);
    pose.leftElbow = pose.rightElbow = -.8; pose.machineLift = rep;
  },(pose,phase)=>{
    strain(pose,phase); pose.leftKnee = 1.55; pose.rightKnee = 1.55+Math.sin(phase*40)*.22;
  }),
  define('curls','Dumbbell curls',(pose,{rep,amount=1})=>{
    pose.weights = amount>.15; pose.leftElbow = -.15+(-.15-rep*2)*amount;
    pose.rightElbow = -.15+(-.15-(1-rep)*2)*amount;
  },(pose,phase)=>{strain(pose,phase);pose.leftShoulderZ=.8;pose.rightShoulderZ=-.8;},undefined,false),
  define('presses','Dumbbell overhead press',(pose,{rep,amount=1})=>{
    pose.weights = amount>.15; pose.leftShoulderX = pose.rightShoulderX = (-1.9-rep*1.1)*amount;
    pose.leftElbow = pose.rightElbow = -.15+(-.95+rep*.9)*amount; pose.headPitch = -.08*amount;
  },(pose,phase)=>{strain(pose,phase);pose.leftShoulderX=-2;pose.rightShoulderX=-1;},undefined,false),
  define('squats','Bodyweight squat',(pose,{rep,amount=1})=>{
    const bend = rep*.75*amount;
    pose.leftHip = pose.rightHip = -bend; pose.leftKnee = pose.rightKnee = bend*2;
    pose.bodyY = -.9*(1-Math.cos(bend)); pose.lean = rep*.3*amount;
    pose.leftShoulderX = pose.rightShoulderX = -.9*amount;
    pose.leftElbow = pose.rightElbow = -.15-.65*amount;
  },(pose,phase)=>{
    release(pose);pose.bodyRoll=Math.sin(phase*Math.PI)*.8;pose.leftShoulderZ=1.6;pose.rightShoulderZ=-1.6;
    pose.leftKnee=pose.rightKnee=1.2;
  },undefined,false),
].map(exercise=>[exercise.id,exercise]));

export function validateExercises(registry = exercises, events = gymEventTypes) {
  for(const [id,exercise] of Object.entries(registry)) {
    if(typeof exercise.sample!=='function')throw new Error(`Exercise ${id} has no animation.`);
    for(const event of events)if(typeof exercise.events?.[event]!=='function')throw new Error(`Exercise ${id} has no ${event} handler.`);
  }
}
export const equipmentExercises = Object.values(exercises).filter(exercise=>exercise.equipment);
export const floorExercises = Object.values(exercises).filter(exercise=>!exercise.equipment).map(exercise=>exercise.id);
export function shuffled(values, random = Math.random) {
  const result = [...values];
  for(let i=result.length-1;i>0;i--){const j=Math.floor(random()*(i+1));[result[i],result[j]]=[result[j],result[i]];}
  return result;
}
