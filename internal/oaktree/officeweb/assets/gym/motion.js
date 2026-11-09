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

export function gymPose(state, identity, time, {seated = false, desk = false, walking = false, still = false, eventAge = Infinity} = {}) {
  const clock = time * (.85 + identity % 11 * .03) + identity % 97 * .19;
  const duration = 14 + (identity >>> 4) % 5;
  const choices = seated ? ['drink', 'seated-stretch', 'rest'] : routines[state] || routines.unknown;
  const phase = clock % duration;
  let amount = smooth(phase / 1.2) * smooth((duration - 2 - phase) / 1.2);
  const pose = {
    routine: choices[(Math.floor(clock / duration) + (identity >>> 8)) % choices.length],
    bodyY: seated ? -.32 : 0, breath: Math.sin(clock * 1.8) * .006,
    sway: Math.sin(clock * .7) * .012, lean: desk ? .12 : .015,
    headYaw: Math.sin(clock * .38) * .16, headPitch: Math.sin(clock * .6) * .025,
    leftShoulderX: 0, rightShoulderX: 0, leftShoulderZ: .045, rightShoulderZ: -.045,
    leftElbow: -.15, rightElbow: -.15,
    leftHip: seated ? -Math.PI / 2 : 0, rightHip: seated ? -Math.PI / 2 : 0,
    leftKnee: seated ? Math.PI / 2 : 0, rightKnee: seated ? Math.PI / 2 : 0,
    offsetX: 0, facing: 0, stride: 0, weights: false, bottle: false, towel: false,
    clipboard: false, pageTurn: 0,
  };
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
    return pose;
  }
  if ((state === 'question' || state === 'ready') && eventAge < 3) {
    pose.routine = state === 'question' ? 'hand-raise' : seated ? 'seated-stretch' : 'stretch';
    amount = Math.max(amount, smooth(eventAge / .3) * smooth((3 - eventAge) / .5));
  }
  const lift = .5 + .5 * Math.sin(clock * 2.1);
  switch (pose.routine) {
    case 'curls':
      pose.weights = amount > .15;
      blend('leftElbow', -.3 - lift * 2);
      blend('rightElbow', -.3 - (1 - lift) * 2);
      break;
    case 'presses':
      pose.weights = amount > .15;
      blend('leftShoulderX', -1.9 - lift * 1.1);
      blend('rightShoulderX', -1.9 - lift * 1.1);
      blend('leftElbow', -1.1 + lift * .9);
      blend('rightElbow', -1.1 + lift * .9);
      blend('headPitch', -.08);
      break;
    case 'squats': {
      const bend = lift * .75 * amount;
      pose.leftHip = pose.rightHip = -bend;
      pose.leftKnee = pose.rightKnee = bend * 2;
      pose.bodyY = -.9 * (1 - Math.cos(bend));
      blend('lean', lift * .3);
      blend('leftShoulderX', -.9);
      blend('rightShoulderX', -.9);
      blend('leftElbow', -.8);
      blend('rightElbow', -.8);
      break;
    }
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
  return pose;
}

export function applyGymPose(model, pose, amount = 1) {
  function rotate(part, x = 0, y = 0, z = 0) {
    part.rotation.set(lerp(part.rotation.x, x, amount), lerp(part.rotation.y, y, amount), lerp(part.rotation.z, z, amount));
  }
  model.body.position.y = lerp(model.body.position.y, pose.bodyY, amount);
  model.body.scale.y = 1 + pose.breath;
  rotate(model.body, 0, 0, pose.sway);
  rotate(model.torso, pose.lean);
  rotate(model.head, pose.headPitch, pose.headYaw);
  rotate(model.leftArm.shoulder, pose.leftShoulderX, 0, pose.leftShoulderZ);
  rotate(model.rightArm.shoulder, pose.rightShoulderX, 0, pose.rightShoulderZ);
  rotate(model.leftArm.elbow, pose.leftElbow);
  rotate(model.rightArm.elbow, pose.rightElbow);
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
