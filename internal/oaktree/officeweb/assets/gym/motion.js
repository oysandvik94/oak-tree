import {exercises, trainingSetDuration} from './exercises.js';
export {trainingSetDuration} from './exercises.js';

const lerp = (a, b, amount) => a + (b - a) * amount;
const smooth = value => { const t = Math.max(0, Math.min(1, value)); return t * t * (3 - 2 * t); };
const routines = {
  working: ['curls', 'presses', 'squats', 'towel'],
  question: ['hand-raise', 'pace', 'look-around'],
  ready: ['stretch', 'drink', 'pace', 'rest'],
  review: ['read', 'turn-page', 'consider'],
  blocked: ['puzzled', 'pace', 'fold-arms'],
  unknown: ['look-around', 'rest'],
};

export function gymPose(state, identity, time, {seated = false, desk = false, equipment, equipmentTime = 0, walking = false, still = false, eventAge = Infinity, floorExercise, spotting, reaction} = {}) {
  const clock = time * (.85 + identity % 11 * .03) + identity % 97 * .19;
  const duration = 14 + (identity >>> 4) % 5;
  const choices = seated ? ['drink', 'seated-stretch', 'rest'] : routines[state] || routines.unknown;
  const phase = clock % duration;
  let amount = smooth(phase / 1.2) * smooth((duration - 2 - phase) / 1.2);
  const pose = {
    routine: choices[(Math.floor(clock / duration) + (identity >>> 8)) % choices.length],
    bodyY: seated ? -.32 : 0, bodyZ: 0, bodyPitch: 0, bodyRoll: 0, breath: Math.sin(clock * 1.8) * .006,
    sway: Math.sin(clock * .7) * .012, lean: desk ? .12 : .015,
    headYaw: Math.sin(clock * .38) * .16, headPitch: Math.sin(clock * .6) * .025,
    leftShoulderX: 0, rightShoulderX: 0, leftShoulderZ: .045, rightShoulderZ: -.045,
    leftElbow: -.15, rightElbow: -.15,
    leftHip: seated ? -Math.PI / 2 : 0, rightHip: seated ? -Math.PI / 2 : 0,
    leftKnee: seated ? Math.PI / 2 : 0, rightKnee: seated ? Math.PI / 2 : 0,
    offsetX: 0, facing: 0, stride: 0, weights: false, bottle: false, towel: false,
    clipboard: false, pageTurn: 0, machineLift: 0, handTargets: null, barHeld: false, released: false,
  };
  if(reaction){if(exercises[reaction.exercise].equipment)equipment=reaction.exercise;else floorExercise=reaction.exercise;}
  function finish(){
    if(reaction){
      const phase=Math.max(0,Math.min(1,(time-reaction.startedAt)/reaction.duration));
      exercises[reaction.exercise].events[reaction.type](pose,phase);
      pose.event=reaction.type;pose.routine=`${reaction.exercise}:${reaction.type}`;
    }
    return pose;
  }
  function blend(name, value) { pose[name] = lerp(pose[name], value, amount); }
  function walk(stride) {
    const step = clock * 6;
    pose.stride = stride;
    pose.bodyY = Math.abs(Math.sin(step)) * .025 * stride;
    pose.leftHip = Math.sin(step) * .45 * stride;
    pose.rightHip = -pose.leftHip;
    pose.leftKnee = Math.max(0, Math.sin(step)) * .7 * stride;
    pose.rightKnee = Math.max(0, -Math.sin(step)) * .7 * stride;
    pose.leftShoulderX = -Math.sin(step) * .4 * stride;
    pose.rightShoulderX = -pose.leftShoulderX;
  }
  if (walking) {
    pose.routine = 'walk'; pose.lean = .04; walk(1);
    if (state === 'question' && eventAge < 3) { pose.rightShoulderZ = -2.7; pose.rightElbow = -.4; }
    return finish();
  }
  if(spotting){
    pose.routine='spotting';pose.facing=spotting.facing;
    pose.leftShoulderX=pose.rightShoulderX=-.8;pose.leftElbow=pose.rightElbow=-1.1;
    pose.lean=spotting.helping ? .28 : .1;pose.headYaw=0;
    if(spotting.helping){pose.leftShoulderX=pose.rightShoulderX=-1.3;pose.sway=Math.sin(clock*8)*.035;}
    return finish();
  }
  if (equipment) {
    const workTime = trainingSetDuration(identity) - 3;
    const rep = .5 - .5 * Math.cos(Math.min(Math.max(0, equipmentTime), workTime) / workTime * 8 * Math.PI);
    pose.routine = equipment; pose.headYaw *= .25; pose.sway = 0;
    exercises[equipment].sample(pose,{rep,resting:equipmentTime>=workTime});
    return finish();
  }
  if ((state === 'question' || state === 'ready') && eventAge < 3) {
    pose.routine = state === 'question' ? 'hand-raise' : seated ? 'seated-stretch' : 'stretch';
    amount = Math.max(amount, smooth(eventAge / .3) * smooth((3 - eventAge) / .5));
  }
  const lift = .5 + .5 * Math.sin(clock * 2.1);
  if(state==='working'&&pose.routine!=='towel'&&floorExercise)pose.routine=floorExercise;
  if(exercises[pose.routine]){exercises[pose.routine].sample(pose,{rep:lift,amount});return finish();}
  switch (pose.routine) {
    case 'towel':
      pose.towel = amount > .15;
      blend('leftShoulderX', -1.2);
      blend('leftShoulderZ', .5);
      blend('leftElbow', -1.7 + Math.sin(clock * 3) * .12);
      blend('headPitch', .1);
      break;
    case 'drink':
      pose.bottle = amount > .15;
      blend('rightShoulderX', -.9);
      blend('rightShoulderZ', -.5);
      blend('rightElbow', -2);
      blend('headPitch', -.12);
      break;
    case 'stretch':
    case 'seated-stretch':
      blend('leftShoulderZ', 2.8);
      blend('rightShoulderZ', -2.8);
      blend('leftElbow', -.3);
      blend('rightElbow', -.3);
      blend('sway', Math.sin(clock * .8) * .07);
      break;
    case 'hand-raise':
      blend('rightShoulderZ', -2.85 + Math.sin(clock * 3) * .12);
      blend('rightElbow', -.4);
      blend('headYaw', Math.sin(clock * .9) * .3);
      break;
    case 'pace':
      if (!still) {
        pose.offsetX = Math.sin(clock * .65) * .45 * amount;
        pose.facing = Math.cos(clock * .65) >= 0 ? Math.PI / 2 : -Math.PI / 2;
        walk(amount * .75);
      }
      break;
    case 'read':
    case 'turn-page':
      pose.clipboard = true;
      blend('leftShoulderX', -.6);
      blend('leftElbow', -1.7);
      blend('rightShoulderX', desk ? -.5 : -.7);
      blend('rightElbow', desk ? -.4 : -1.6);
      blend('headPitch', .25 + Math.sin(clock * .7) * .04);
      blend('headYaw', Math.sin(clock * .8) * .08);
      blend('lean', desk ? .2 : .06);
      if (pose.routine === 'turn-page') pose.pageTurn = lift * Math.PI * amount;
      break;
    case 'consider':
    case 'puzzled':
      blend('rightShoulderX', -.8);
      blend('rightShoulderZ', -.45);
      blend('rightElbow', -2.1);
      blend('headYaw', Math.sin(clock * .8) * .25);
      blend('headPitch', .12);
      break;
    case 'fold-arms':
      blend('leftShoulderX', -.5);
      blend('rightShoulderX', -.5);
      blend('leftShoulderZ', .7);
      blend('rightShoulderZ', -.7);
      blend('leftElbow', -1.6);
      blend('rightElbow', -1.6);
      break;
    case 'look-around':
      blend('headYaw', Math.sin(clock * .65) * .5);
      break;
    case 'rest':
      blend('lean', seated ? -.06 : .035);
      break;
  }
  return finish();
}

export function applyGymPose(model, pose, amount = 1) {
  function rotate(part, x = 0, y = 0, z = 0) {
    part.rotation.set(lerp(part.rotation.x, x, amount), lerp(part.rotation.y, y, amount), lerp(part.rotation.z, z, amount));
  }
  model.body.position.y = lerp(model.body.position.y, pose.bodyY, amount);
  model.body.position.z = lerp(model.body.position.z, pose.bodyZ, amount);
  model.body.scale.y = 1 + pose.breath;
  rotate(model.body, pose.bodyPitch, 0, pose.sway + pose.bodyRoll);
  rotate(model.torso, pose.lean);
  rotate(model.head, pose.headPitch, pose.headYaw);
  if (pose.handTargets) {
    // Solve both arm joints so the squat grips stay on the bar, with elbows behind it.
    const upper = .49, lower = .41;
    [model.leftArm, model.rightArm].forEach((arm, index) => {
      const down = arm.shoulder.position.clone().set(0,-1,0);
      const target = down.clone().fromArray(pose.handTargets[index]).sub(arm.shoulder.position);
      const distance = target.length(), direction = target.clone().normalize();
      const reach = (upper ** 2 - lower ** 2 + distance ** 2) / (2 * distance);
      const pole = down.clone().set(0,-1,-.35);
      pole.addScaledVector(direction,-pole.dot(direction)).normalize();
      const elbow = direction.clone().multiplyScalar(reach).addScaledVector(pole,Math.sqrt(upper ** 2 - reach ** 2));
      const shoulder = arm.shoulder.quaternion.clone().setFromUnitVectors(down,elbow.clone().normalize());
      const forearm = target.clone().sub(elbow).applyQuaternion(shoulder.clone().invert()).normalize();
      const joint = arm.elbow.quaternion.clone().setFromUnitVectors(down,forearm);
      arm.shoulder.quaternion.slerp(shoulder,amount); arm.elbow.quaternion.slerp(joint,amount);
    });
  } else {
    rotate(model.leftArm.shoulder, pose.leftShoulderX, 0, pose.leftShoulderZ);
    rotate(model.rightArm.shoulder, pose.rightShoulderX, 0, pose.rightShoulderZ);
    rotate(model.leftArm.elbow, pose.leftElbow);
    rotate(model.rightArm.elbow, pose.rightElbow);
  }
  rotate(model.leftLeg.hip, pose.leftHip);
  rotate(model.rightLeg.hip, pose.rightHip);
  rotate(model.leftLeg.knee, pose.leftKnee);
  rotate(model.rightLeg.knee, pose.rightKnee);
  model.weights.forEach(weight => { weight.visible = pose.weights; });
  model.bottle.visible = pose.bottle;
  model.towel.visible = pose.towel;
  model.clipboard.visible = pose.clipboard;
  model.page.rotation.x = -pose.pageTurn;
}
