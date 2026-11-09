import * as T from './vendor/three.module.js';
import {OrbitControls} from './vendor/OrbitControls.js';
import {RoomEnvironment} from './vendor/RoomEnvironment.js';
import {createGymArt} from './art.js';
import {daylightAt} from './lighting.js';
import {zones, agentState, hash, layoutFor, slotFor, routeTo} from './state.js';

export function createGymView({canvas, stage, labels, onSelect, onOpen, onFrame, onError, reducedMotion}) {
  const renderer = new T.WebGLRenderer({canvas, antialias: true, powerPreference: 'high-performance'});
  renderer.setPixelRatio(Math.min(devicePixelRatio || 1, 1.5));
  renderer.shadowMap.enabled = true;
  renderer.shadowMap.type = T.PCFSoftShadowMap;
  renderer.toneMapping = T.ACESFilmicToneMapping;
  const scene = new T.Scene();
  scene.background = new T.Color('#1c2533');
  const generator = new T.PMREMGenerator(renderer), environment = new RoomEnvironment();
  const envMap = generator.fromScene(environment, .06);
  scene.environment = envMap.texture;
  scene.environmentIntensity = .3;
  environment.dispose(); generator.dispose();
  const camera = new T.OrthographicCamera(-20, 20, 20, -20, .1, 250);
  const controls = new OrbitControls(camera, canvas);
  controls.enableDamping = !reducedMotion;
  controls.minZoom = .5; controls.maxZoom = 6;
  controls.maxPolarAngle = Math.PI * .47;
  controls.mouseButtons.LEFT = T.MOUSE.ROTATE;
  controls.mouseButtons.RIGHT = T.MOUSE.PAN;
  const hemisphere = new T.HemisphereLight('#91a0b0', '#66523b', .75);
  const sun = new T.DirectionalLight('#ffd09b', 1.75);
  sun.position.set(-12, 24, 20); sun.castShadow = true;
  sun.shadow.mapSize.set(2048, 2048);
  sun.shadow.normalBias = .025;
  scene.add(hemisphere, sun, sun.target);
  const fill = new T.DirectionalLight('#729dbd', .85); fill.position.set(15, 10, -8); scene.add(fill);
  const actors = new Map(), capacity = {working: 6, question: 6, ready: 6, review: 6, blocked: 6};
  let art, world, layout, roomLights, cameraGoal, selected, hovered;
  let paused = reducedMotion, connected = false, fitted = true;
  let width = 1, height = 1, extent = 20, lastTime = performance.now(), time = 0, pointer;
  let disposed = false, lostContext = false, dirty = true, frameID;
  const raycaster = new T.Raycaster(), ndc = new T.Vector2();
  const projected = new T.Vector3();
  const daylightColor = new T.Color('#fff0d2');
  let lightingMinute, daylight;
  function updateLighting(force=false){
    const date=new Date(), minute=Math.floor(date.getTime()/60000);
    if(!force&&minute===lightingMinute)return;
    lightingMinute=minute;daylight=daylightAt(date);dirty=true;
    sun.intensity=T.MathUtils.lerp(1.1,3.8,daylight);
    sun.color.set('#ffbd78').lerp(daylightColor,daylight);
    hemisphere.intensity=T.MathUtils.lerp(.55,1.65,daylight);
    fill.intensity=T.MathUtils.lerp(.5,.85,daylight);
    scene.environmentIntensity=T.MathUtils.lerp(.25,.4,daylight);
    roomLights.children.forEach(light=>light.intensity=T.MathUtils.lerp(light.userData.nightIntensity,3,daylight));
  }

  function disposeTree(root) {
    const geometries = new Set(), materials = new Set(), textures = new Set();
    root.traverse(o => {if (o.geometry) geometries.add(o.geometry); if (o.material) for (const m of [].concat(o.material)) {materials.add(m); if (m.map) textures.add(m.map);}});
    textures.forEach(t => t.dispose()); materials.forEach(m => m.dispose()); geometries.forEach(g => g.dispose());
  }
  function lamp(x, z, root) {
    const {box, gold, dark, mat} = art;
    box(.45,.08,.45,x,.1,z,dark,root); box(.05,1.85,.05,x,1,z,gold,root);
    for(let i=0;i<4;i++) box(.7-i*.1,.1,.7-i*.1,x,1.95+i*.1,z,mat('#d3b584',.95,0,.3),root);
    const light = new T.PointLight('#ffbd78',10,5,2);light.userData.nightIntensity=10;light.position.set(x,1.9,z);roomLights.add(light);
  }
  function smallRoom(state, room) {
    const root = new T.Group(); world.add(root);
    const {box,woodLight,rubber,wood,iron,dark,ivory,cloth,mat,decal,plant,towel,batch} = art;
    const bottom = state === 'ready' || state === 'review';
    for(let x=room.x-room.width/2+.5;x<room.x+room.width/2;x++) for(let z=room.back+.5;z<room.front;z++) box(.98,.07,.98,x,-.02,z,bottom?wood:rubber,root);
    for(const x of [room.x-room.width/2,room.x+room.width/2]) box(.1,.12,room.depth,x,.02,room.z,woodLight,root);
    const decorZ = bottom ? room.front-.7 : room.back+.7;
    const color = zones.find(zone=>zone.id===state).color;
    // Low furniture stays out of the straight approach columns from the central aisle.
    if(state==='ready') {
      box(3.2,.38,.72,room.x,.4,decorZ-.04,cloth,root);
      box(3.6,.55,.16,room.x,.75,decorZ+.4,cloth,root);
      for(const x of [room.x-1.7,room.x+1.7]) box(.2,.42,.72,x,.63,decorZ-.04,cloth,root);
      towel(room.x+1,.63,decorZ,.55,root);
      box(.8,.5,.7,room.x-2.4,.25,decorZ,woodLight,root);
      art.bottle(room.x-2.4,.5,decorZ,root);
    } else if(state==='review') {
      box(2.6,.14,.75,room.x,.85,decorZ,woodLight,root);
      for(const x of [room.x-1,room.x+1]) box(.1,.8,.55,x,.4,decorZ,iron,root);
      box(.08,.75,.08,room.x,1.25,decorZ,iron,root);
      decal('REVIEW','CHECK THE WORK',room.x,1.7,decorZ-.1,1.9,.9,{parent:root,font:14,small:7});
      for(let i=0;i<3;i++) box(.55,.025,.35,room.x-.8,.94+i*.03,decorZ,ivory,root);
    } else if(state==='question') {
      box(3.1,1.6,.1,room.x,2,decorZ,woodLight,root);
      decal('COACHING','YOUR ANSWER IS NEEDED',room.x,2,decorZ+.065,2.9,1.4,{parent:root,font:14,small:7});
      for(const x of [room.x-1.2,room.x+1.2]) box(.08,1.4,.08,x,.7,decorZ,iron,root);
      box(.8,.12,.7,room.x-2.35,.55,decorZ,woodLight,root);box(.12,.5,.45,room.x-2.35,.25,decorZ,iron,root);
    } else {
      box(3.1,.75,.7,room.x,.4,decorZ,mat('#657f74'),root);
      box(3.3,.12,.9,room.x,.85,decorZ,woodLight,root);
      for(const x of [room.x-.9,room.x,room.x+.9]){box(.8,.55,.03,x,.44,decorZ+.37,iron,root);box(.22,.035,.04,x,.55,decorZ+.4,art.gold,root);}
      decal('HELP BAY','BLOCKED / NEEDS SUPPORT',room.x,1.65,decorZ,2.9,.9,{parent:root,font:14,small:6});
      box(.45,.22,.3,room.x+.8,1.02,decorZ,mat('#b78166'),root);
      box(.3,.05,.05,room.x+.8,1.2,decorZ,iron,root);
    }
    plant(room.x-2.7,decorZ,.6,root); lamp(room.x+2.8,decorZ,root);
    decal(zones.find(z=>z.id===state).name.toUpperCase(),state.toUpperCase(),room.x,.04,bottom?room.back+.65:room.front-.5,2.6,.6,{parent:root,rotation:[-Math.PI/2,0,0],font:13,small:7,fg:color});
    batch(root);
  }
  function buildWorld() {
    if(world) {scene.remove(world,roomLights);disposeTree(world); actors.forEach(a=>a.label.remove());actors.clear();}
    art=createGymArt();world=new T.Group();roomLights=new T.Group();scene.add(world,roomLights);
    layout=layoutFor(capacity);
    const training=new T.Group();training.position.x=-7;world.add(training);art.training(training);
    // The original slice remains intact; extra capacity grows along clear floor, not into equipment.
    const extra=new T.Group();world.add(extra);
    const hallFront=layout.corridor+1.4;
    art.box(28,.42,hallFront+5,1,-.44,(hallFront-5)/2,art.dark,extra);
    art.box(15,.5,layout.ready.depth,7.5,-.4,layout.ready.z,art.dark,extra);
    art.box(28,.1,hallFront+5,1,-.18,(hallFront-5)/2,art.mat('#263537'),extra);
    art.box(15,.1,layout.ready.depth,7.5,-.1,layout.ready.z,art.wood,extra);
    // Continuous circulation connects all five zones; no agent walks over a void.
    for(const [x,w] of [[-7,12],[7.5,15]]) art.box(w,.08,.1,x,-.05,hallFront,art.woodLight,extra);
    for(let z=5.5;z<layout.working.front;z++) for(let x=-12.5;x< -1;x++) art.box(.98,.1,.98,x,-.02,z,art.rubber,extra);
    art.box(28,.07,2.5,1,-.095,layout.corridor,art.wood,extra);
    for(let x=-12;x<15;x+=2) art.box(.5,.01,.035,x,-.05,layout.corridor,art.gold,extra);
    art.batch(extra);
    const connectingWalls=new T.Group();world.add(connectingWalls);
    art.box(16,3.05,.18,7,1.48,-5.05,art.mat('#514a3c'),connectingWalls);
    for(let row=0;row<10;row++)for(let col=0;col<25;col++){
      const x=-.7+col*.64+(row%2)*.3;
      art.box(.6,.275,.1,x,.15+row*.3,-4.9,art.mat(['#806449','#927251','#765b46'][(row+col)%3]),connectingWalls);
    }
    art.box(16,.15,.3,7,3.05,-5,art.woodLight,connectingWalls);
    for(const x of [0,7.5,14.8])art.box(.17,3,.18,x,1.5,-4.75,art.wood,connectingWalls);
    art.box(.15,.5,6,7.5,.2,-1.7,art.wood,connectingWalls);
    art.box(.15,.5,layout.ready.depth-1,7.5,.2,layout.ready.z+.4,art.wood,connectingWalls);
    art.batch(connectingWalls);
    for(const state of ['question','blocked','ready','review']) smallRoom(state,layout[state]);
    for(const x of [-12.2,-6.9,-1.7]) {const light=new T.PointLight('#ffc073',11,6,2);light.userData.nightIntensity=11;light.position.set(x,3.35,-4.05);roomLights.add(light);}
    const [,minZ,,maxZ]=layout.bounds;
    sun.target.position.set(0,0,(minZ+maxZ)/2);
    Object.assign(sun.shadow.camera,{left:-25,right:25,top:25,bottom:-25,near:1,far:100});sun.shadow.camera.updateProjectionMatrix();
    updateLighting(true);
    extent=Math.max(12,(maxZ-minZ)*.8);resize();
  }
  function makeActor(data, position) {
    const model=art.actor(hash(data.id));
    const root=model.root;root.scale.setScalar(.85);root.position.set(position.x,.08,position.z);
    root.userData.agentId=data.id; world.add(root);
    const marker=art.box(.18,.18,.18,0,2.8,0,art.mat('#9bcc9c'),root);
    const selection=new T.Group();root.add(selection);
    for(const side of [-1,1]) {art.box(.9,.025,.035,0,0,side*.5,art.gold,selection);art.box(.035,.025,1,side*.47,0,0,art.gold,selection);}
    const band=art.box(.27,.075,.38,.43,1.58,0,art.mat('#ffffff'),root);
    const label=document.createElement('button');label.type='button';label.className='gym-agent-label';label.hidden=true;label.dataset.overlayAgent=data.id;
    label.addEventListener('click',()=>onSelect(data.id,true));label.addEventListener('dblclick',()=>onOpen(data.id));labels.append(label);
    return {...model,data,position:{...position},home:position,path:[],marker,selection,band,label,walk:0,failureBaseline:null,flinchUntil:0};
  }
  function sync(data) {
    dirty=true;
    const counts=Object.fromEntries(zones.map(z=>[z.id,0]));let rebuild=!world;
    for(const item of data){const state=agentState(item);if(state in counts)counts[state]++;}
    for(const state in counts) if(counts[state]>capacity[state]) {capacity[state]=Math.ceil(counts[state]/3)*3;rebuild=true;}
    if(rebuild)buildWorld();
    const incoming=new Set(data.map(a=>a.id));
    for(const [id,actor] of actors)if(!incoming.has(id)){world.remove(actor.root);actor.shirt.map.dispose();actor.shirt.dispose();actor.label.remove();actors.delete(id);}
    const indexes={};
    for(const item of [...data].sort((a,b)=>a.id.localeCompare(b.id))){
      const state=agentState(item),index=indexes[state]||0;indexes[state]=index+1;
      const destination=slotFor(layout,state,index);let actor=actors.get(item.id);
      if(!actor){actor=makeActor(item,destination);actors.set(item.id,actor);}
      const previousState=agentState(actor.data);
      if(Math.hypot(actor.home.x-destination.x,actor.home.z-destination.z)>.01){actor.path=routeTo(actor.position,destination,layout.corridor,previousState===state);actor.home=destination;}
      actor.data=item;actor.state=state;
      if(paused){actor.position={...destination};actor.path=[];}
      actor.marker.material=art.mat(zones.find(z=>z.id===state)?.color||'#a5a5a5');
      actor.selection.visible=item.id===selected;
      actor.band.visible=Boolean(item.group);if(item.group)actor.band.material=art.mat(new T.Color().setHSL((hash(item.group.id)%360)/360,.4,.6).getHex());
      actor.label.textContent=`${item.name||item.project} · ${item.role||'Agent'}`;
      actor.label.title=`${item.branch||'Root session'}${item.group?' · '+item.group.name:''}`;
      actor.label.dataset.state=state;actor.label.setAttribute('aria-pressed',String(item.id===selected));
      const failure=item.toolFailures;
      if(failure&&Number.isSafeInteger(failure.count)&&failure.count>=0){if(actor.failureBaseline?.generation===failure.generation&&failure.count>actor.failureBaseline.count)actor.flinchUntil=time+1;actor.failureBaseline=failure;}
    }
    if(rebuild&&fitted)focusGym();
  }
  function select(id, focus=false) {
    dirty=true;selected=id;actors.forEach(a=>{a.selection.visible=a.data.id===id;a.label.setAttribute('aria-pressed',String(a.data.id===id));});
    if(focus&&actors.has(id)) {fitted=false;const p=actors.get(id).position;moveCamera(new T.Vector3(p.x,1.1,p.z),2.7);}
  }
  function moveCamera(target, zoom) {
    dirty=true;
    const direction=new T.Vector3(20,18,25).normalize().multiplyScalar(45);
    cameraGoal={target,position:target.clone().add(direction),zoom};
    if(reducedMotion){camera.position.copy(cameraGoal.position);controls.target.copy(target);camera.zoom=zoom;camera.updateProjectionMatrix();cameraGoal=null;}
  }
  function focusGym(){
    fitted=true;
    const bounds=new T.Box3().setFromObject(world),target=bounds.getCenter(new T.Vector3());
    const fitCamera=camera.clone();fitCamera.position.copy(target).add(new T.Vector3(20,18,25).normalize().multiplyScalar(45));
    fitCamera.lookAt(target);fitCamera.zoom=1;fitCamera.updateProjectionMatrix();fitCamera.updateMatrixWorld();
    let span=0;
    for(const x of [bounds.min.x,bounds.max.x])for(const y of [bounds.min.y,bounds.max.y])for(const z of [bounds.min.z,bounds.max.z]){
      const point=new T.Vector3(x,y,z).project(fitCamera);span=Math.max(span,Math.abs(point.x),Math.abs(point.y));
    }
    moveCamera(target,.9/span);
  }
  function resize(){dirty=true;const r=stage.getBoundingClientRect();width=Math.max(1,r.width);height=Math.max(1,r.height);const a=width/height;camera.left=-extent*a;camera.right=extent*a;camera.top=extent;camera.bottom=-extent;camera.updateProjectionMatrix();renderer.setSize(width,height,false);if(layout&&fitted)focusGym();}
  function project(id, y=2.7) {
    const actor=actors.get(id);if(!actor)return null;
    projected.set(actor.position.x,y,actor.position.z).project(camera);
    return {x:(projected.x+1)*width/2,y:(1-projected.y)*height/2,visible:projected.z>=-1&&projected.z<=1&&Math.abs(projected.x)<1.05&&Math.abs(projected.y)<1.05};
  }
  function hit(event){const rect=canvas.getBoundingClientRect();ndc.set((event.clientX-rect.left)/rect.width*2-1,-(event.clientY-rect.top)/rect.height*2+1);raycaster.setFromCamera(ndc,camera);for(const hit of raycaster.intersectObjects([...actors.values()].map(a=>a.root),true)){let node=hit.object;while(node&&!node.userData.agentId)node=node.parent;if(node)return node.userData.agentId;}return null;}
  function overlayAgent(){return actors.has(hovered)?hovered:selected;}
  function pointerMove(event){
    const overlay=event.target.closest?.('.gym-agent-label, .speech-bubble');
    const id=event.buttons?null:overlay?.dataset.overlayAgent||(event.target===canvas?hit(event):null);
    if(id!==hovered){hovered=id;dirty=true;}
  }
  function pointerLeave(){hovered=null;pointer=null;dirty=true;}
  function pointerDown(event){pointer={x:event.clientX,y:event.clientY,id:event.pointerId};}
  function pointerUp(event){if(pointer&&event.pointerId===pointer.id&&Math.hypot(event.clientX-pointer.x,event.clientY-pointer.y)<5&&event.button===0){const id=hit(event);if(id)onSelect(id,false);}pointer=null;}
  function doubleClick(event){if(event.button===0){const id=hit(event);if(id)onOpen(id);}}
  function stopCameraTransition(){cameraGoal=null;fitted=false;}
  function contextLost(event){event.preventDefault();lostContext=true;onError('3D rendering stopped. Reload this page to restore it; your agents are still available in the list.');}
  canvas.addEventListener('pointerdown',pointerDown);canvas.addEventListener('pointerup',pointerUp);canvas.addEventListener('dblclick',doubleClick);canvas.addEventListener('webglcontextlost',contextLost);
  stage.addEventListener('pointermove',pointerMove);stage.addEventListener('pointerleave',pointerLeave);
  controls.addEventListener('start',stopCameraTransition);
  controls.addEventListener('change',()=>dirty=true);
  const observer=new ResizeObserver(resize);observer.observe(stage);

  function animateActor(actor,dt){
    const model=actor,phase=hash(actor.data.id)%100;let walking=false;
    if(!paused&&connected){let remaining=dt*2.2;while(actor.path.length&&remaining>0){const p=actor.path[0],dx=p.x-actor.position.x,dz=p.z-actor.position.z,d=Math.hypot(dx,dz);walking=true;actor.root.rotation.y=Math.atan2(dx,dz);if(d<=remaining){actor.position={...p};actor.path.shift();remaining-=d;}else{actor.position.x+=dx/d*remaining;actor.position.z+=dz/d*remaining;remaining=0;}}}
    if(!walking)actor.root.rotation.y=0;
    actor.root.position.set(actor.position.x,.08,actor.position.z);
    const t=time+phase;
    model.leftLeg.rotation.x=model.rightLeg.rotation.x=0;
    model.leftArm.shoulder.rotation.set(0,0,.045);model.rightArm.shoulder.rotation.set(0,0,-.045);
    model.leftArm.elbow.rotation.x=model.rightArm.elbow.rotation.x=-.15;
    model.weights.forEach(weight=>weight.visible=actor.state==='working'&&!walking);
    if(walking){model.leftLeg.rotation.x=Math.sin(t*8)*.3;model.rightLeg.rotation.x=-Math.sin(t*8)*.3;model.leftArm.shoulder.rotation.x=-Math.sin(t*8)*.4;model.rightArm.shoulder.rotation.x=Math.sin(t*8)*.4;}
    else if(actor.state==='working'){model.leftArm.elbow.rotation.x=-.15-1.7*(.5+.5*Math.sin(t*1.7));model.rightArm.elbow.rotation.x=-.15-1.7*(.5+.5*Math.sin(t*1.7+Math.PI));}
    else if(actor.state==='question'){model.rightArm.shoulder.rotation.z=2.8+Math.sin(t*2)*.1;model.rightArm.elbow.rotation.x=-.4;}
    else if(actor.state==='review'){model.leftArm.elbow.rotation.x=-1.5;model.rightArm.elbow.rotation.x=-1.2;}
    else if(actor.state==='ready'){model.leftArm.shoulder.rotation.z=.25+Math.sin(t*.6)*.08;model.rightArm.shoulder.rotation.z=-.25-Math.sin(t*.6)*.08;}
    else if(actor.state==='blocked'){model.rightArm.elbow.rotation.x=-1.4;}
    actor.root.rotation.z=actor.flinchUntil>time?Math.sin(t*15)*.06:0;
  }
  function frame(now){if(disposed)return;frameID=requestAnimationFrame(frame);const dt=Math.min((now-lastTime)/1000,.06);lastTime=now;if(document.hidden||lostContext)return;
    updateLighting();
    controls.update();
    if((paused||!connected||!actors.size)&&!dirty&&!cameraGoal){onFrame(now);return;}
    if(!paused&&connected)time+=dt;
    actors.forEach(a=>animateActor(a,dt));
    if(cameraGoal){const k=1-Math.exp(-dt*6);camera.position.lerp(cameraGoal.position,k);controls.target.lerp(cameraGoal.target,k);camera.zoom=T.MathUtils.lerp(camera.zoom,cameraGoal.zoom,k);camera.updateProjectionMatrix();if(camera.position.distanceTo(cameraGoal.position)<.01&&Math.abs(camera.zoom-cameraGoal.zoom)<.005)cameraGoal=null;}
    camera.lookAt(controls.target);renderer.render(scene,camera);
    actors.forEach(actor=>{const p=project(actor.data.id,2.6);actor.label.hidden=!p.visible||actor.data.id!==overlayAgent();actor.label.style.left=`${p.x}px`;actor.label.style.top=`${p.y}px`;});
    dirty=false;onFrame(now);
  }
  buildWorld();camera.position.set(25,25,35);focusGym();frameID=requestAnimationFrame(frame);
  return {
    sync,select,project,focusGym,overlayAgent,
    setConnected(value){connected=value;dirty=true;},
    setPaused(value){dirty=true;paused=value;if(value)actors.forEach(a=>{a.position={...a.home};a.path=[];});},
    getInfo(){return {agents:actors.size,zones:zones.length,daylight,capacity:{...capacity},positions:Object.fromEntries([...actors].map(([id,a])=>[id,{...a.position,state:a.state,moving:a.path.length>0,screen:project(id,1.1)}])),drawCalls:renderer.info.render.calls};},
    dispose(){stage.removeEventListener('pointermove',pointerMove);stage.removeEventListener('pointerleave',pointerLeave);disposed=true;cancelAnimationFrame(frameID);observer.disconnect();controls.dispose();canvas.removeEventListener('pointerdown',pointerDown);canvas.removeEventListener('pointerup',pointerUp);canvas.removeEventListener('dblclick',doubleClick);canvas.removeEventListener('webglcontextlost',contextLost);disposeTree(world);envMap.dispose();renderer.dispose();labels.replaceChildren();},
  };
}
