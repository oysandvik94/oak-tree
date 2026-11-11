import * as T from './vendor/three.module.js';
import {OrbitControls} from './vendor/OrbitControls.js';
import {RoomEnvironment} from './vendor/RoomEnvironment.js';
import {createGymArt} from './art.js';
import {createTrainingEquipment} from './equipment.js';
import {daylightAt} from './lighting.js';
import {exercises, floorExercises, shuffled, validateExercises} from './exercises.js';
import {createGymEventTracker} from './events.js';
import {gymPose, applyGymPose, trainingSetDuration, gymTravelSpeed} from './motion.js';
import {zones, agentState, hash, layoutFor, stationFor, stationRoute, isEquipment, trainingRotations, todoLoad} from './state.js';
import {recoveryOrder, updatePartners} from './partners.js';
import {placeOverlays, positionOverlay} from './overlays.js';

export function createGymView({canvas, stage, labels, onSelect, onOpen, onFrame, onError, reducedMotion}) {
  validateExercises();
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
  const eventTracker=createGymEventTracker();
  let art, world, layout, equipment, roomLights, cameraGoal, selected;
  let labelBoxes=[],bodyBoxes=[];
  let connected = false, fitted = true;
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
    const decorZ = state === 'ready' ? room.front-1.1 : bottom ? room.front-.7 : room.back+.7;
    const color = zones.find(zone=>zone.id===state).color;
    // Low furniture stays out of the straight approach columns from the central aisle.
    if(state==='ready') {
      box(3.2,.26,.72,room.x,.3,decorZ-.04,cloth,root);
      box(3.6,.55,.16,room.x,.68,decorZ-.4,cloth,root);
      for(const x of [room.x-1.7,room.x+1.7]) box(.2,.36,.72,x,.52,decorZ-.04,cloth,root);
      towel(room.x+1.65,.72,decorZ,.45,root);
      box(.8,.5,.7,room.x-2.4,.25,decorZ,woodLight,root);
      art.bottle(room.x-2.4,.5,decorZ,root);
    } else if(state==='review') {
      box(2.6,.14,.75,room.x,.85,decorZ,woodLight,root);
      for(const x of [room.x-1,room.x+1]) box(.1,.8,.55,x,.4,decorZ,iron,root);
      decal('REVIEW','CHECK THE WORK',room.x,1.08,decorZ+.4,1.3,.4,{parent:root,font:12,small:6});
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
    const training=new T.Group();training.position.x=layout.working.x;world.add(training);art.training(training,layout.working);
    equipment=new Map(layout.equipment.map(station=>[station.id,createTrainingEquipment(art,station,world)]));
    const extra=new T.Group();world.add(extra);
    const [minX,minZ,maxX,maxZ]=layout.bounds, spanX=maxX-minX, centerX=(minX+maxX)/2;
    const hallFront=layout.corridor+1.4;
    art.box(spanX,.42,2.8,centerX,-.44,layout.corridor,art.dark,extra);
    art.box(15,.42,layout.question.depth,7.5,-.44,layout.question.z,art.dark,extra);
    art.box(15,.1,layout.question.depth,7.5,-.18,layout.question.z,art.rubber,extra);
    art.box(15,.5,layout.ready.depth,7.5,-.4,layout.ready.z,art.dark,extra);
    art.box(15,.1,layout.ready.depth,7.5,-.1,layout.ready.z,art.wood,extra);
    // Continuous circulation connects all five zones; no agent walks over a void.
    for(const [x,w] of [[layout.working.x,layout.working.width],[7.5,15]]) art.box(w,.08,.1,x,-.05,hallFront,art.woodLight,extra);
    art.box(spanX,.07,2.5,centerX,0,layout.corridor,art.wood,extra);
    for(let x=minX+1;x<maxX;x+=2) art.box(.5,.01,.035,x,.041,layout.corridor,art.gold,extra);
    art.batch(extra);
    const connectingWalls=new T.Group();world.add(connectingWalls);
    const topBack=layout.question.back;
    art.box(16,3.05,.18,7,1.48,topBack-.05,art.mat('#514a3c'),connectingWalls);
    for(let row=0;row<10;row++)for(let col=0;col<25;col++){
      const x=-.7+col*.64+(row%2)*.3;
      art.box(.6,.275,.1,x,.15+row*.3,topBack+.1,art.mat(['#806449','#927251','#765b46'][(row+col)%3]),connectingWalls);
    }
    art.box(16,.15,.3,7,3.05,topBack,art.woodLight,connectingWalls);
    for(const x of [0,7.5,14.8])art.box(.17,3,.18,x,1.5,topBack+.25,art.wood,connectingWalls);
    art.box(.15,.5,layout.question.depth-1,7.5,.2,layout.question.z+.4,art.wood,connectingWalls);
    art.box(.15,.5,layout.ready.depth-1,7.5,.2,layout.ready.z+.4,art.wood,connectingWalls);
    art.batch(connectingWalls);
    for(const state of ['question','blocked','ready','review']) smallRoom(state,layout[state]);
    for(const x of [layout.working.x-8.2,layout.working.x+.1,layout.working.x+8.3]) {const light=new T.PointLight('#ffc073',11,6,2);light.userData.nightIntensity=11;light.position.set(x,3.35,-4.05);roomLights.add(light);}
    sun.target.position.set(0,0,(minZ+maxZ)/2);
    const shadowSpan=Math.max(25,maxZ-minZ);Object.assign(sun.shadow.camera,{left:-shadowSpan,right:shadowSpan,top:shadowSpan,bottom:-shadowSpan,near:1,far:100});sun.shadow.camera.updateProjectionMatrix();
    updateLighting(true);
    extent=Math.max(12,(maxZ-minZ)*.8);resize();
  }
  function makeActor(data, station) {
    const identity=hash(data.id), position=station.position;
    const model=art.actor(identity);
    const root=model.root;root.scale.setScalar(.85);root.position.set(position.x,.08,position.z);
    root.userData.agentId=data.id; world.add(root);
    const marker=art.box(.18,.18,.18,0,2.8,0,art.mat('#9bcc9c'),root);
    const selection=new T.Group();root.add(selection);
    for(const side of [-1,1]) {art.box(.9,.025,.035,0,0,side*.5,art.gold,selection);art.box(.035,.025,1,side*.47,0,0,art.gold,selection);}
    const band=art.box(.27,.075,.38,0,-.07,0,art.mat('#ffffff'),model.rightArm.shoulder);
    const label=document.createElement('button');label.type='button';label.className='gym-agent-label';label.hidden=true;label.dataset.overlayAgent=data.id;
    const badge=document.createElement('span');badge.className='label-role';badge.setAttribute('aria-hidden','true');
    const name=document.createElement('span');name.className='label-name';
    const stem=document.createElement('span');stem.className='overlay-stem';stem.setAttribute('aria-hidden','true');label.append(badge,name,stem);
    label.addEventListener('click',()=>onSelect(data.id,true));label.addEventListener('dblclick',()=>onOpen(data.id));labels.append(label);
    return {...model,data,identity,station,position:{...position},home:position,path:[],marker,selection,band,label,arrivedAt:time,rotationAt:time,lastEquipment:null,exerciseOrder:[],floorExercise:shuffled(floorExercises)[0],floorChangeAt:0,reactionSince:-Infinity,event:null,eventQueue:[],pendingStation:null,load:todoLoad(data.todo)||{completed:null,level:0}};
  }
  function advanceEvent(actor){
    if(actor.event&&time<actor.event.startedAt+actor.event.duration)return;
    actor.event=null;
    if(actor.eventQueue.length){
      const next=actor.eventQueue.shift();actor.event={...next,startedAt:time,duration:next.type==='tool_failed'?3.3:next.type==='question_raised'?1.8:2.4};
    }else if(actor.pendingStation){const station=actor.pendingStation;actor.pendingStation=null;assignStation(actor,station);}
  }
  function queueEvent(actor,type){
    if(reducedMotion||actor.event?.type===type||actor.eventQueue.some(event=>event.type===type))return;
    const exercise=actor.event?.exercise||(isEquipment(actor.station)&&!actor.path.length?actor.station.kind:actor.floorExercise);
    actor.eventQueue.push({type,exercise});advanceEvent(actor);
  }
  function assignStation(actor, station) {
    if(actor.event){actor.pendingStation=station;return;}
    actor.path=stationRoute(layout,actor.position,station);
    actor.station=station;actor.home=station.position;actor.arrivedAt=time;
    actor.rotationAt=time+(isEquipment(station)?trainingSetDuration(actor.identity):5+actor.identity%4);
    if(isEquipment(station))actor.lastEquipment=station.id;
    if(reducedMotion){actor.position={...actor.home};actor.path=[];}
  }
  function updateTraining(starting) {
    const occupants=[...actors.values()],workers=occupants.filter(actor=>actor.state==='working'&&!actor.data.spotting);
    for(const {worker,station} of trainingRotations(workers,layout.equipment,time,occupants,{starting,paused:reducedMotion}))assignStation(worker,station);
  }
  function updateSpotters(){
    for(const actor of actors.values()){
      if(!actor.data.spotting)continue;
      const lifter=actors.get(actor.data.spotting);if(!lifter)continue;
      const machine=isEquipment(lifter.station)?lifter.station:null;
      const offset=exercises[machine?.kind||lifter.floorExercise].spotter;
      const position={x:lifter.home.x+offset.x,z:lifter.home.z+offset.z};
      const id=`spot:${lifter.data.id}:${lifter.station.id||'floor'}`;
      if(actor.station.id!==id)assignStation(actor,{id,kind:'spotter',position,facing:offset.facing,equipmentId:machine?.id});
    }
  }
  function sync(data) {
    const notices=eventTracker.observe(data),queued=new Set(),byId=new Map(data.map(agent=>[agent.id,agent]));
    function notify(actor,notice){
      const item=byId.get(actor.data.id),partner=byId.get(item.partnerId);
      if(notice.type==='session_ready'&&partner&&agentState(item)==='ready'&&agentState(partner)==='ready'){
        if(!reducedMotion)actor.pendingBump=true;
      }else queueEvent(actor,notice.type);
    }
    dirty=true;
    const counts=Object.fromEntries(zones.map(z=>[z.id,0]));let rebuild=!world;
    for(const item of data){const state=agentState(item);if(state in counts)counts[state]++;}
    for(const state in counts)if(counts[state]>capacity[state]){const columns=state==='working'?4:3;capacity[state]=Math.ceil(counts[state]/columns)*columns;rebuild=true;}
    if(rebuild)buildWorld();
    const incoming=new Set(data.map(a=>a.id));
    for(const [id,actor] of actors)if(!incoming.has(id)){world.remove(actor.root);actor.shirt.map.dispose();actor.shirt.dispose();actor.label.remove();actors.delete(id);}
    for(const notice of notices)if(actors.has(notice.agentId)){notify(actors.get(notice.agentId),notice);queued.add(notice.agentId);}
    const indexes={},starting=new Set();
    for(const item of recoveryOrder(data)){
      const state=agentState(item),index=indexes[state]||0;indexes[state]=index+1;
      let actor=actors.get(item.id);
      const previousState=actor?agentState(actor.data):null;
      const floor=stationFor(layout,state,index);
      const station=item.spotting&&actor?.data.spotting===item.spotting?actor.station
        :state==='working'&&!item.spotting&&previousState==='working'&&!actor.data.spotting&&isEquipment(actor.station)
          ?layout.equipment.find(item=>item.id===actor.station.id):floor;
      if(!actor){actor=makeActor(item,station);actors.set(item.id,actor);}
      if(state==='working')actor.floorStation={...floor,id:`floor:${item.id}`};
      if(Math.hypot(actor.home.x-station.position.x,actor.home.z-station.position.z)>.01||previousState!==state||actor.station.kind!==station.kind)assignStation(actor,station);
      if(previousState!==state||actor.data.spotting!==item.spotting){actor.reactionSince=time;actor.partnerMoment=null;actor.carryingPlate=false;actor.setupUntil=null;}
      if(state==='working'&&!item.spotting&&(previousState!=='working'||actor.data.spotting)){actor.rotationAt=time;starting.add(item.id);}
      actor.data=item;actor.state=state;actor.load=todoLoad(item.todo)||actor.load;
      actor.weights.forEach(weight=>weight.scale.setScalar(.9*(1+actor.load.level*.12)));
      if(reducedMotion){actor.position={...actor.home};actor.path=[];}
      actor.marker.material=art.mat(zones.find(z=>z.id===state)?.color||'#a5a5a5');
      actor.selection.visible=item.id===selected;
      actor.band.visible=Boolean(item.group);if(item.group)actor.band.material=art.mat(new T.Color().setHSL((hash(item.group.id)%360)/360,.4,.6).getHex());
      actor.label.children[0].textContent=(item.role||'Agent').slice(0,1).toUpperCase();
      actor.label.children[1].textContent=item.name||item.branch||item.project;
      actor.label.title=`${item.name||item.project} · ${item.role||'Agent'} · ${item.project} · ${item.branch||'Root session'}${item.group?' · '+item.group.name:''}`;
      actor.label.setAttribute('aria-label',actor.label.title);
      actor.label.dataset.state=state;actor.label.setAttribute('aria-pressed',String(item.id===selected));
    }
    if(starting.size)updateTraining(starting);
    updateSpotters();
    for(const notice of notices)if(!queued.has(notice.agentId)&&actors.has(notice.agentId))notify(actors.get(notice.agentId),notice);
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
    return screenPoint(projected.set(actor.position.x,y,actor.position.z));
  }
  function screenPoint(point){
    point.project(camera);
    return {x:(point.x+1)*width/2,y:(1-point.y)*height/2,visible:point.z>=-1&&point.z<=1&&Math.abs(point.x)<1.05&&Math.abs(point.y)<1.05};
  }
  function hit(event){const rect=canvas.getBoundingClientRect();ndc.set((event.clientX-rect.left)/rect.width*2-1,-(event.clientY-rect.top)/rect.height*2+1);raycaster.setFromCamera(ndc,camera);for(const hit of raycaster.intersectObjects([...actors.values()].map(a=>a.root),true)){let node=hit.object;while(node&&!node.userData.agentId)node=node.parent;if(node)return node.userData.agentId;}return null;}
  function updateLabels(){
    const visible=[];
    for(const actor of actors.values()){
      const anchor=project(actor.data.id,.08);actor.label.hidden=!anchor.visible;
      if(anchor.visible)visible.push({actor,anchor});
    }
    bodyBoxes=visible.map(({actor,anchor})=>{
      const head=screenPoint(actor.head.localToWorld(projected.set(0,.5,0)));
      return {x:(anchor.x+head.x)/2,y:Math.min(head.y,anchor.y)-3,width:Math.abs(head.x-anchor.x)+1.8*height*camera.zoom/(2*extent),height:Math.abs(anchor.y-head.y)+6};
    });
    labelBoxes=placeOverlays(visible.map(({actor,anchor})=>({node:actor.label,anchor,x:anchor.x,y:anchor.y+7,width:actor.label.offsetWidth,height:actor.label.offsetHeight})),{width,height,occupied:bodyBoxes});
    for(const box of labelBoxes)positionOverlay(box.node,box,box.anchor);
  }
  function pointerLeave(){pointer=null;}
  function pointerDown(event){pointer={x:event.clientX,y:event.clientY,id:event.pointerId};}
  function pointerUp(event){if(pointer&&event.pointerId===pointer.id&&Math.hypot(event.clientX-pointer.x,event.clientY-pointer.y)<5&&event.button===0)onSelect(hit(event),false);pointer=null;}
  function doubleClick(event){if(event.button===0){const id=hit(event);if(id)onOpen(id);}}
  function stopCameraTransition(){cameraGoal=null;fitted=false;}
  function contextLost(event){event.preventDefault();lostContext=true;onError('3D rendering stopped. Reload this page to restore it; your agents are still available in the list.');}
  canvas.addEventListener('pointerdown',pointerDown);canvas.addEventListener('pointerup',pointerUp);canvas.addEventListener('dblclick',doubleClick);canvas.addEventListener('webglcontextlost',contextLost);
  stage.addEventListener('pointerleave',pointerLeave);
  controls.addEventListener('start',stopCameraTransition);
  controls.addEventListener('change',()=>dirty=true);
  const observer=new ResizeObserver(resize);observer.observe(stage);

  function animateActor(actor,dt){
    const active=!reducedMotion&&connected;
    let walking=false, facing=actor.root.rotation.y;
    if(active&&!actor.event) {
      let remaining=dt*gymTravelSpeed(actor.state);
      while(actor.path.length&&remaining>0) {
        const p=actor.path[0],dx=p.x-actor.position.x,dz=p.z-actor.position.z,d=Math.hypot(dx,dz);
        walking=true;facing=isEquipment(actor.station)&&actor.path.length===1?0:Math.atan2(dx,dz);
        if(d<=remaining){actor.position={...p};actor.path.shift();remaining-=d;}
        else{actor.position.x+=dx/d*remaining;actor.position.z+=dz/d*remaining;remaining=0;}
      }
    }
    if(walking&&!actor.path.length){
      actor.arrivedAt=time;
      if(isEquipment(actor.station)){actor.setupUntil=time+2.1;actor.arrivedAt=actor.setupUntil;}
      actor.rotationAt=actor.arrivedAt+(isEquipment(actor.station)?trainingSetDuration(actor.identity):5+actor.identity%4);
      if(actor.state==='ready')actor.reactionSince=time;
    }
    if(actor.setupUntil&&time>=actor.setupUntil){actor.setupUntil=null;actor.carryingPlate=false;}
    if(active&&!actor.event&&!actor.partnerMoment&&actor.state==='working'&&!actor.data.spotting&&!isEquipment(actor.station)&&time>=actor.floorChangeAt){
      actor.floorExercise=shuffled(floorExercises.filter(id=>id!==actor.floorExercise))[0];actor.floorChangeAt=time+16;
    }
    const lifter=actors.get(actor.data.spotting);
    const pose=gymPose(actor.state,actor.identity,time,{
      seated:actor.station.kind==='bench'&&!actor.path.length,
      desk:actor.station.kind==='desk'&&!actor.path.length,
      equipment:isEquipment(actor.station)&&!actor.path.length?actor.station.kind:null,equipmentTime:time-actor.arrivedAt,
      walking,still:reducedMotion,eventAge:time-actor.reactionSince,floorExercise:actor.floorExercise,
      spotting:lifter?{facing:actor.station.facing,helping:lifter.event?.type==='tool_failed'}:null,reaction:actor.event,
      partnerMoment:actor.partnerMoment,company:actor.company,carryingPlate:actor.carryingPlate?actor.carryHand:null,
      setup:actor.setupUntil?1-(actor.setupUntil-time)/2.1:undefined,
    });
    actor.routine=pose.routine;actor.localWalking=active&&!actor.event&&!walking&&pose.stride>.1;
    if(!walking&&!actor.path.length&&active&&!actor.event)actor.position={x:actor.home.x+pose.offsetX,z:actor.home.z};
    if(active) {
      const target=walking?facing:pose.facing;
      const turn=Math.atan2(Math.sin(target-actor.root.rotation.y),Math.cos(target-actor.root.rotation.y));
      actor.root.rotation.y+=turn*(1-Math.exp(-dt*8));
    } else if(reducedMotion)actor.root.rotation.y=pose.facing;
    const platform=layout.equipment.find(station=>station.surface&&Math.abs(actor.position.x-station.position.x)<1.6&&Math.abs(actor.position.z-station.position.z)<1.4);
    const elevation=platform?platform.surface*Math.min(1,(1.4-Math.abs(actor.position.z-platform.position.z))/.3):0;
    actor.root.position.set(actor.position.x,.08+elevation,actor.position.z);
    applyGymPose(actor,pose,active?1-Math.exp(-dt*10):1);
    return pose;
  }
  function frame(now){if(disposed)return;frameID=requestAnimationFrame(frame);const dt=Math.min((now-lastTime)/1000,.06);lastTime=now;if(document.hidden||lostContext)return;
    updateLighting();
    controls.update();
    if((reducedMotion||!connected||!actors.size)&&!dirty&&!cameraGoal){onFrame(now);return;}
    if(!reducedMotion&&connected)time+=dt;
    if(!reducedMotion&&connected){actors.forEach(advanceEvent);updateTraining();updateSpotters();updatePartners(actors,time);}
    if(connected||reducedMotion){
      equipment.forEach(model=>model.update(null));
      actors.forEach(actor=>{
        const pose=animateActor(actor,dt);
        if(isEquipment(actor.station)&&!actor.path.length&&(pose.routine===actor.station.kind||actor.event?.exercise===actor.station.kind||actor.setupUntil))equipment.get(actor.station.id).update(actor,pose);
      });
    }
    if(cameraGoal){const k=1-Math.exp(-dt*6);camera.position.lerp(cameraGoal.position,k);controls.target.lerp(cameraGoal.target,k);camera.zoom=T.MathUtils.lerp(camera.zoom,cameraGoal.zoom,k);camera.updateProjectionMatrix();if(camera.position.distanceTo(cameraGoal.position)<.01&&Math.abs(camera.zoom-cameraGoal.zoom)<.005)cameraGoal=null;}
    camera.lookAt(controls.target);renderer.render(scene,camera);
    updateLabels();
    dirty=false;onFrame(now);
  }
  buildWorld();camera.position.set(25,25,35);focusGym();frameID=requestAnimationFrame(frame);
  return {
    sync,select,project,focusGym,overlayBounds:()=>[...bodyBoxes,...labelBoxes],
    setConnected(value){connected=value;dirty=true;},
    getInfo(){
      return {
        agents:actors.size,zones:zones.length,daylight,animationTime:time,capacity:{...capacity},
        positions:Object.fromEntries([...actors].map(([id,actor])=>[id,{
          ...actor.position,state:actor.state,routine:actor.routine,station:actor.station.kind,stationId:actor.station.id||null,spotting:actor.data.spotting||null,status:actor.data.status,event:actor.event?.type||null,eventExercise:actor.event?.exercise||null,
          completedTodos:actor.load.completed,loadLevel:actor.load.level,interaction:actor.partnerMoment?.type||null,company:Boolean(actor.company),carryingPlate:Boolean(actor.carryingPlate),
          moving:actor.path.length>0||Boolean(actor.localWalking),screen:screenPoint(actor.torso.localToWorld(projected.set(0,.35,.23))),
        }])),
        equipment:layout.equipment.map(({id,kind,position})=>({id,kind,...position,loadLevel:equipment.get(id).loadLevel,occupant:[...actors.values()].find(actor=>actor.station.id===id)?.data.id||null})),
        drawCalls:renderer.info.render.calls,
      };
    },
    dispose(){stage.removeEventListener('pointerleave',pointerLeave);disposed=true;cancelAnimationFrame(frameID);observer.disconnect();controls.dispose();canvas.removeEventListener('pointerdown',pointerDown);canvas.removeEventListener('pointerup',pointerUp);canvas.removeEventListener('dblclick',doubleClick);canvas.removeEventListener('webglcontextlost',contextLost);disposeTree(world);envMap.dispose();renderer.dispose();labels.replaceChildren();},
  };
}
