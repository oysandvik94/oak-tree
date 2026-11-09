import * as T from './vendor/three.module.js';
import {drawPixelLine} from './font.js';

export function createGymArt() {
let seed=832;const random=()=>{seed=(seed*1664525+1013904223)>>>0;return seed/4294967296};const unit=new T.BoxGeometry(1,1,1),materials=new Map();
function mat(color,roughness=.83,metalness=0,emissive=0){const key=[color,roughness,metalness,emissive].join();if(!materials.has(key))materials.set(key,new T.MeshStandardMaterial({color,roughness,metalness,emissive:color,emissiveIntensity:emissive}));return materials.get(key)}
const iron=mat('#334b49',.5,.65),edge=mat('#72847b',.45,.7),dark=mat('#17292c',.8,.3),gold=mat('#bca06b',.48,.5),ivory=mat('#dfd5b1'),skin=mat('#c5966c'),skinLight=mat('#ddb282'),hair=mat('#292b2d'),warm=mat('#ffcd7f',.5,0,2.5),cool=mat('#85a8bf',.6,0,.25);
const staticRoot=new T.Group(),wallRoot=new T.Group(),dynamic=new T.Group();
function box(w,h,d,x,y,z,m=iron,parent=staticRoot){const o=new T.Mesh(unit,m);o.scale.set(w,h,d);o.position.set(x,y,z);o.castShadow=true;o.receiveShadow=true;parent.add(o);return o}
function group(x=0,y=0,z=0,parent=staticRoot){const g=new T.Group();g.position.set(x,y,z);parent.add(g);return g}
function texture(palette,size=32,kind='grain'){const c=document.createElement('canvas');c.width=c.height=size;const a=c.getContext('2d');a.fillStyle=palette[0];a.fillRect(0,0,size,size);for(let i=0;i<size*size*(kind==='wood'?.3:kind==='cloth'?.08:.16);i++){a.fillStyle=palette[1+Math.floor(random()*(palette.length-1))];const x=Math.floor(random()*size),y=Math.floor(random()*size);a.fillRect(x,y,kind==='wood'?1+Math.floor(random()*12):1,1);}if(kind==='cloth'){a.fillStyle=palette[1];for(let y=0;y<size;y+=4)for(let x=0;x<size;x+=4)a.fillRect(x,y,1,1)}const t=new T.CanvasTexture(c);t.colorSpace=T.SRGBColorSpace;t.magFilter=T.NearestFilter;t.minFilter=T.LinearMipmapLinearFilter;return t}
function textured(palette,kind='grain',roughness=.9){return new T.MeshStandardMaterial({map:texture(palette,kind==='wood'?64:32,kind),roughness})}
const wood=textured(['#876a4b','#9c8058','#746044','#b49a66'],'wood'),woodLight=textured(['#af8b58','#b99866','#896e47','#c3a772'],'wood'),rubber=textured(['#263537','#2f4040','#4a5550','#1f2e32']),cloth=textured(['#637c66','#758c72','#516959','#8b9977'],'cloth'),brickMats=Array.from({length:6},(_,i)=>textured([['#7c5a43','#957152','#6e5345','#a17d57'][i%4],'#805b46','#a08261','#5f4940']));
function decal(text,sub,x,y,z,w,h,opts={}) {
 const c=document.createElement('canvas');c.width=opts.px||512;c.height=Math.round(c.width*h/w);
 const a=c.getContext('2d'),scale=opts.px?1:4;
 a.fillStyle=opts.bg||'#243b39';a.fillRect(0,0,c.width,c.height);
 a.strokeStyle=opts.border||'#84927a';a.lineWidth=scale;a.strokeRect(3*scale,3*scale,c.width-6*scale,c.height-6*scale);
 drawPixelLine(a,text,c.width,c.height*(sub?.44:.7),(opts.font||14)*scale,opts.fg||'#e1cf9a');
 if(sub)drawPixelLine(a,sub,c.width,c.height*.8,(opts.small||7)*scale,'#bbc2a2');
 const t=new T.CanvasTexture(c);t.colorSpace=T.SRGBColorSpace;t.magFilter=T.NearestFilter;t.minFilter=T.LinearMipmapLinearFilter;t.generateMipmaps=true;
 const material=new T.MeshStandardMaterial({map:t,roughness:.9});
 const mesh=box(w,h,.025,x,y,z,material,opts.parent||staticRoot);
 if(opts.rotation)mesh.rotation.set(...opts.rotation);
 return mesh;
}
function pixelSign(x,y,z,w,h,parent) {
 const c=document.createElement('canvas');c.width=512;c.height=120;const a=c.getContext('2d');
 a.fillStyle='#243b39';a.fillRect(0,0,512,120);a.fillStyle='#84927a';a.fillRect(8,8,496,104);a.fillStyle='#243b39';a.fillRect(11,11,490,98);
 drawPixelLine(a,'OAK ATHLETIC',512,58,35,'#e1cf9a');drawPixelLine(a,'SHOW UP. DO THE WORK.',512,98,21,'#bbc2a2');
 const t=new T.CanvasTexture(c);t.colorSpace=T.SRGBColorSpace;t.magFilter=T.NearestFilter;t.minFilter=T.LinearMipmapLinearFilter;t.generateMipmaps=true;
 box(w,h,.025,x,y,z,new T.MeshStandardMaterial({map:t,roughness:.9}),parent);
}
function plate(x,y,z,r=.42,color='#465e57',parent=staticRoot){const g=group(x,y,z,parent),s=r/4,m=mat(color,.7,.3);for(let iy=-4;iy<4;iy++)for(let iz=-4;iz<4;iz++){const d=Math.hypot(iy+.5,iz+.5);if(d>4.1||d<1.3)continue;const hole=(Math.abs(iy+.5)<1.1&&Math.abs(iz+.5)>2.1&&Math.abs(iz+.5)<3.1);if(!hole)box(.13,s,s,0,(iy+.5)*s,(iz+.5)*s,m,g)}box(.15,.14,.14,0,0,0,edge,g);box(.155,.07,.07,0,0,0,dark,g);box(.135,.05,.14,0,r*.73,0,ivory,g);return g}
function dumbbell(parent,x=0,y=0,z=0,scale=1){const g=group(x,y,z,parent);g.scale.setScalar(scale);box(.65,.065,.065,0,0,0,edge,g);for(const side of [-1,1]){box(.18,.29,.22,side*.26,0,0,dark,g);box(.18,.22,.29,side*.26,0,0,dark,g);box(.185,.1,.1,side*.26,0,0,gold,g);for(let i=0;i<4;i++)box(.025,.074,.074,-.1+i*.06,0,0,mat('#586a61'),g)}return g}
function plant(x,z,scale=1,parent=staticRoot){const g=group(x,0,z,parent);g.scale.setScalar(scale);box(.55,.16,.55,0,.1,0,mat('#7e654b'),g);box(.7,.45,.7,0,.38,0,mat('#a88a62'),g);box(.77,.08,.77,0,.64,0,mat('#c0a77a'),g);box(.61,.03,.61,0,.69,0,mat('#343d2b'),g);for(let i=0;i<11;i++){const angle=i*2.4,height=.55+random()*.55;box(.055,height,.055,Math.sin(angle)*.15,.7+height/2,Math.cos(angle)*.15,mat('#506b41'),g);for(let k=0;k<4;k++){const xx=Math.sin(angle)*(.12+k*.09),zz=Math.cos(angle)*(.12+k*.09);box(.2,.1,.23,xx,.9+height-k*.12,zz,mat(['#58764a','#718b54','#415e43'][i%3]),g);box(.12,.07,.12,xx+.035,.97+height-k*.12,zz,mat('#7a9257'),g)}}}
function bottle(x,y,z,parent=staticRoot){box(.14,.28,.14,x,y+.14,z,mat('#9caf99',.35,.25),parent);box(.11,.04,.11,x,y+.3,z,dark,parent);box(.145,.09,.145,x,y+.17,z,ivory,parent);box(.04,.09,.01,x,y+.17,z+.077,mat('#627665'),parent)}
function towel(x,y,z,w=.65,parent=staticRoot){for(let i=0;i<4;i++)box(w-i*.015,.045,.37,x,y+i*.045,z,mat(i%2?'#d3c6a4':'#e4d8b6'),parent);for(let i=0;i<8;i++)box(.025,.08,.03,x-w/2+.04+i*w/8,y+.1,z+.2,ivory,parent);box(w,.015,.025,x,y+.19,z+.1,mat('#65817b'),parent)}
function architecture(room){
 const {width,depth,front}=room,left=-width/2,right=width/2;
 box(width+.3,.55,depth+.3,0,-.36,room.z,mat('#263a40'));box(width+.5,.13,depth+.5,0,-.65,room.z,mat('#111e29'));box(width+.35,.07,depth+.35,0,-.1,room.z,mat('#9c8461'));
 for(let ix=0;ix<width;ix++)for(let iz=0;iz<depth;iz++){const x=left+.5+ix,z=-4.5+iz;box(.98,.1,.98,x,-.02,z,ix>width-3?wood:rubber);if(ix<width-3)for(let k=0;k<6;k++)box(.012,.005,.045,x+(random()-.5)*.85,.033,z+(random()-.5)*.85,mat('#6c7567'));}
 box(width+.1,4.3,.22,0,2.1,-5.05,mat('#463f37'),wallRoot);box(.22,4.3,depth,left-.05,2.1,room.z,mat('#463f37'),wallRoot);
 for(let row=0;row<14;row++){
  for(let col=0;col<Math.ceil(width/.64);col++){const x=left+.3+col*.64+(row%2)*.32;if(x>right-.05)continue;box(.605,.276,.12,x,.15+row*.3,-4.89,brickMats[(col+row*3)%6],wallRoot);}
  for(let col=0;col<Math.ceil(depth/.64);col++){const z=-4.7+col*.64+(row%2)*.32;if(z>front-.1)continue;box(.12,.276,.605,left+.11,.15+row*.3,z,brickMats[(col+row)%6],wallRoot);}
 }
 box(width+.3,.16,.38,0,4.29,-5,woodLight,wallRoot);box(.38,.16,depth,left,4.29,room.z,woodLight,wallRoot);box(width,.14,.17,0,.16,-4.73,dark,wallRoot);box(.17,.14,depth,left+.27,.16,room.z,dark,wallRoot);
 for(const x of [left+.35,-.65,right-.35]){box(.22,4.2,.2,x,2.12,-4.68,wood,wallRoot);for(const y of [.5,3.85]){box(.27,.16,.23,x,y,-4.65,iron,wallRoot);box(.045,.045,.02,x,y,-4.52,gold,wallRoot);}}
 for(let z=-4.4;z<front;z+=4.7)box(.22,4.2,.22,left+.32,2.12,z,wood,wallRoot);
 // A deep blue window with a pixel skyline, inset frame and chunky muntins.
 const c=document.createElement('canvas');c.width=128;c.height=96;const a=c.getContext('2d');a.fillStyle='#33495d';a.fillRect(0,0,128,96);for(let y=0;y<96;y+=8){a.fillStyle=['#263d55','#2b4259','#344e61','#426070','#4b6d78','#577b82'][Math.min(5,Math.floor(y/16))];a.fillRect(0,y,128,8)}for(let i=0;i<35;i++){a.fillStyle='#82979b';a.fillRect(Math.floor(random()*128),Math.floor(random()*50),1,1)}for(let i=0;i<12;i++){const h=10+random()*28;a.fillStyle=i%2?'#223c48':'#29434c';a.fillRect(i*12,96-h,9,h);for(let yy=96-h+5;yy<94;yy+=6){a.fillStyle='#ac9963';a.fillRect(i*12+3,yy,2,2)}}const t=new T.CanvasTexture(c);t.colorSpace=T.SRGBColorSpace;t.magFilter=T.NearestFilter;const glass=new T.MeshStandardMaterial({map:t,emissiveMap:t,emissive:'#ffffff',emissiveIntensity:.55,roughness:.4});box(3.35,2.2,.08,3.2,2.7,-4.71,glass,wallRoot);
 for(const x of [1.48,3.2,4.92])box(.11,2.42,.18,x,2.7,-4.6,iron,wallRoot);for(const y of [1.52,2.7,3.88])box(3.55,.1,.18,3.2,y,-4.6,iron,wallRoot);box(3.8,.16,.53,3.2,1.46,-4.52,woodLight,wallRoot);
 pixelSign(-3.1,3.65,-4.63,3.65,.85,wallRoot);
 // Conduit and warm wall lamps keep the expanded wing in the existing oak-and-iron palette.
 box(width-1,.045,.045,0,3.05,-4.63,iron,wallRoot);for(const x of [left+.8,.1,right-.7]){box(.045,.6,.045,x,3.32,-4.58,iron,wallRoot);box(.42,.16,.37,x,3.64,-4.47,dark,wallRoot);box(.29,.07,.23,x,3.52,-4.42,warm,wallRoot);}
 box(width-.1,.07,.18,0,-.015,front,woodLight);box(.18,.07,depth,right,-.015,room.z,woodLight);
}
function equipment(room){
 const left=-room.width/2,right=room.width/2;
 // Free weights live against the back wall, clear of equipment approach lanes.
 for(const x of [-1.45,1.45]){box(.1,1.2,.1,x,.65,-4.1,iron);box(.6,.09,.65,x,.1,-4.1,iron);}
 for(const y of [.55,1.1]){box(3.4,.08,.64,0,y,-4.1,iron);box(3.4,.07,.04,0,y+.06,-3.78,edge);for(let i=0;i<5;i++)dumbbell(staticRoot,-1.28+i*.64,y+.21,-4.1,.85+i*.055);}
 for(let i=0;i<3;i++){const x=3.35+i*.74;box(.7,2.25,.65,x,1.16,-4.1,mat(i%2?'#536f67':'#627a6a'));box(.64,2.12,.025,x,1.17,-3.76,mat('#6c8370'));for(let j=0;j<4;j++)box(.35,.025,.02,x,1.93+j*.07,-3.735,dark);box(.07,.23,.035,x+.22,1.12,-3.72,gold);decal('0'+(i+1),'',x,1.65,-3.73,.21,.15,{font:12,px:32,bg:'#c0ab76',fg:'#293f3d',border:'#c0ab76'});box(.73,.08,.7,x,2.32,-4.1,woodLight);}
 const cx=left+.65;
 box(.5,.08,.5,cx,.08,-.1,iron);box(.07,.8,.07,cx,.5,-.1,iron);box(.55,.08,.55,cx,.94,-.1,edge);box(.43,.035,.43,cx,.99,-.1,ivory);bottle(cx,.99,-.1);
 decal('TODAY','MOVE WITH INTENT',left+.28,2.3,1.7,1.4,1.8,{font:16,small:8,px:128,rotation:[0,Math.PI/2,0],parent:wallRoot});
 const clock=group(left+.29,3.4,-1.3,wallRoot);clock.rotation.y=Math.PI/2;box(.65,.65,.08,0,0,0,ivory,clock);box(.55,.55,.09,0,0,0,dark,clock);for(let i=0;i<12;i++){const a=i*Math.PI/6;box(.035,.035,.015,Math.sin(a)*.22,Math.cos(a)*.22,.06,ivory,clock);}box(.03,.18,.02,0,.08,.075,gold,clock);box(.16,.03,.02,.065,0,.075,gold,clock);
 plant(left+.7,room.front-.8,.7);plant(right-.3,room.front-.6,.55);
 decal('LEAVE IT BETTER','RE-RACK YOUR WEIGHTS',0,.055,6.25,2.8,.7,{font:12,small:7,rotation:[-Math.PI/2,0,0],bg:'#263d3a'});
}
function cozyDetails(room){
 const left=-room.width/2;
 const lx=room.width/2-.3,lz=.7;
 box(.5,.09,.5,lx,.1,lz,dark);box(.065,1.95,.065,lx,1.1,lz,gold);
 const linen=mat('#d3b584',.95,0,.35);
 for(let i=0;i<5;i++){const w=.8-i*.075,y=1.97+i*.09;
  for(const side of [-1,1]){box(w,.1,.045,lx,y,lz+side*w/2,linen);box(.045,.1,w,lx+side*w/2,y,lz,linen);}
 }
 box(.24,.13,.24,lx,2.06,lz,warm);box(.04,.13,.04,lx,2.48,lz,gold);
 // A hanging pot and trailing leaves tucked against the left wall.
 const x=left+.65,z=3.35;
 box(.65,.07,.08,left+.4,3.94,z,iron,wallRoot);
 for(const dx of [-.2,.2])box(.022,.7,.022,x+dx,3.58,z,ivory,wallRoot);
 box(.53,.35,.53,x,3.12,z,mat('#aa7656'),wallRoot);box(.59,.08,.59,x,3.31,z,mat('#c28c63'),wallRoot);
 for(let i=0;i<6;i++){const angle=i*Math.PI/3;
  for(let k=0;k<5;k++){const px=x+Math.cos(angle)*(.2+k*.02),pz=z+Math.sin(angle)*(.2+k*.02);box(.035,.15,.035,px,3.3-k*.18,pz,mat('#4a6442'),wallRoot);box(.17,.08,.16,px+.04,3.3-k*.18,pz,mat(k%2?'#65814f':'#819258'),wallRoot);}
 }
 // Two tiny postcards pinned to the existing board.
 const cards=group(left+.31,2.1,1.7,wallRoot);cards.rotation.y=Math.PI/2;
 for(const side of [-1,1]){box(.35,.3,.025,side*.29,0,0,ivory,cards);box(.29,.18,.015,side*.29,.02,.02,mat('#76918b'),cards);box(.19,.07,.02,side*.29,-.035,.03,mat('#64744f'),cards);box(.07,.06,.02,side*.29+.07,.065,.03,mat('#dec592'),cards);box(.035,.035,.03,side*.29,.12,.04,gold,cards);}
}
let athlete,body,torso,head,leftArm,rightArm,leftLeg,rightLeg;
let characterCloth=cloth,characterSkin=skin,characterSkinLight=skinLight,characterHair=hair;
function makeArm(side,parent){const shoulder=group(side*.43,.73,0,parent);box(.24,.28,.36,0,-.1,0,characterCloth,shoulder);box(.22,.3,.25,0,-.32,.015,characterSkin,shoulder);box(.12,.19,.03,side*.05,-.31,.155,characterSkinLight,shoulder);const elbow=group(0,-.49,0,shoulder);box(.19,.35,.23,0,-.15,.01,characterSkinLight,elbow);box(.2,.075,.24,0,-.31,.01,mat('#ddd2ac'),elbow);box(.21,.12,.24,0,-.41,.025,characterSkin,elbow);const weight=dumbbell(elbow,0,-.41,.09,.9);return {shoulder,elbow,weight}}
function makeLeg(side,parent){
 const hip=group(side*.21,.92,0,parent);
 box(.33,.37,.38,0,-.13,0,mat('#344c4b'),hip);box(.29,.19,.3,0,-.32,.015,characterSkin,hip);
 const knee=group(0,-.44,0,hip);
 box(.27,.16,.28,0,-.05,.015,characterSkin,knee);box(.22,.27,.25,0,-.21,0,mat('#cfccaf'),knee);box(.23,.045,.26,0,-.13,0,mat('#6c8c76'),knee);box(.29,.14,.45,0,-.39,.085,mat('#b6c1aa'),knee);box(.31,.07,.48,0,-.46,.09,mat('#ddd7bd'),knee);box(.21,.11,.2,0,-.34,.04,mat('#5d8076'),knee);
 for(let i=0;i<3;i++)box(.17,.02,.025,0,-.33,.12+i*.045,ivory,knee);
 return {hip,knee};
}
function character(){
 athlete=group(0,0,0,dynamic);body=group(0,0,0,athlete);torso=group(0,.92,0,body);
 box(.73,.6,.4,0,.42,0,characterCloth,torso);box(.65,.23,.37,0,.07,0,characterCloth,torso);box(.73,.05,.41,0,.14,0,mat('#a0ae8c'),torso);for(const x of [-.33,.33])box(.025,.5,.025,x,.42,.215,mat('#9eae8f'),torso);box(.3,.18,.3,0,.8,0,characterSkin,torso);
 head=group(0,.8,0,torso);
 // Face and hair share a neck pivot; the torso and knees articulate independently.
 box(.59,.57,.48,0,.3,0,characterSkinLight,head);box(.49,.1,.44,0,0,0,characterSkin,head);box(.065,.18,.18,-.325,.29,0,characterSkin,head);box(.065,.18,.18,.325,.29,0,characterSkin,head);box(.1,.1,.08,0,.25,.265,characterSkin,head);for(const side of [-1,1]){box(.13,.045,.025,side*.15,.42,.25,characterHair,head);box(.11,.065,.025,side*.15,.345,.25,ivory,head);box(.045,.065,.03,side*.13,.345,.265,characterHair,head);box(.1,.045,.025,side*.2,.17,.25,mat('#bd8766'),head);}box(.18,.025,.025,0,.1,.245,mat('#815c48'),head);box(.66,.16,.55,0,.62,-.025,characterHair,head);box(.57,.09,.47,-.04,.74,-.05,characterHair,head);box(.2,.18,.1,-.23,.53,.23,characterHair,head);box(.1,.35,.42,-.29,.49,-.08,characterHair,head);box(.1,.2,.43,.29,.57,-.08,characterHair,head);for(let i=0;i<8;i++)box(.055,.04,.18,-.25+i*.07,.76+Math.floor(random()*2)*.035,-.08+random()*.08,mat(i%2?'#454039':'#373534'),head);
 const glyph=['011000110','100101001','100101111','100101001','011001001'];glyph.forEach((row,y)=>[...row].forEach((v,x)=>{if(v==='1')box(.038,.038,.016,-.16+x*.04,.58-y*.04,.211,ivory,torso)}));box(.32,.024,.016,0,.31,.211,mat('#bcb48a'),torso);
 leftArm=makeArm(-1,torso);rightArm=makeArm(1,torso);leftLeg=makeLeg(-1,body);rightLeg=makeLeg(1,body);
}
// Batch static cubes by material. Thousands of hand-placed details do not need thousands of draw calls.
function batch(root){root.updateMatrixWorld(true);const buckets=new Map(),inverse=new T.Matrix4().copy(root.matrixWorld).invert();root.traverse(o=>{if(!o.isMesh)return;if(!buckets.has(o.material))buckets.set(o.material,[]);buckets.get(o.material).push(new T.Matrix4().multiplyMatrices(inverse,o.matrixWorld));});root.clear();for(const [material,matrices] of buckets){const inst=new T.InstancedMesh(unit,material,matrices.length);matrices.forEach((m,i)=>inst.setMatrixAt(i,m));inst.castShadow=true;inst.receiveShadow=true;inst.instanceMatrix.needsUpdate=true;root.add(inst);}return [...buckets.values()].reduce((n,v)=>n+v.length,0)}

 function training(parent, room) {
  architecture(room);equipment(room);cozyDetails(room);
  batch(staticRoot);batch(wallRoot);
  parent.add(staticRoot,wallRoot);
 }
 function actor(identity) {
  const previousSeed=seed;seed=identity;
  const colors=['#637c66','#8d7056','#61778c','#87718d','#a07865','#5e8582'];
  const skins=['#ddb282','#ba8665','#916548','#e4c3a0'];
  characterCloth=textured([colors[identity%colors.length],colors[identity%colors.length],'#a0a58b'],'cloth');
  characterSkinLight=mat(skins[(identity>>>5)%skins.length]);
  characterSkin=characterSkinLight;
  characterHair=mat(['#292b2d','#6b4d36','#ad956b','#44372f'][(identity>>>9)%4]);
  character();
  const root=athlete;dynamic.remove(root);root.position.set(0,0,0);root.rotation.y=0;
  const weights=[leftArm.weight,rightArm.weight];
  const drink=group(0,-.41,.09,rightArm.elbow);bottle(0,-.14,0,drink);
  const handTowel=group(0,-.41,.09,leftArm.elbow);towel(0,-.08,0,.45,handTowel);
  const clipboard=group(0,-.41,.18,leftArm.elbow);clipboard.rotation.x=-.6;
  box(.54,.045,.38,0,0,0,woodLight,clipboard);box(.46,.01,.3,0,.03,0,ivory,clipboard);box(.16,.025,.05,0,.045,-.15,iron,clipboard);
  const page=group(0,.04,-.14,clipboard);box(.44,.008,.28,0,0,.14,ivory,page);
  for(let i=0;i<4;i++)box(.3-i*.025,.004,.012,0,.008,.06+i*.045,mat('#849180'),page);
  // Batch only rigid siblings so the head, torso, joints and held props keep their pivots.
  const groups=[];root.traverse(o=>{if(o.isGroup)groups.push(o)});
  for(const part of groups){
   const bins=new Map();
   for(const mesh of [...part.children]){if(!mesh.isMesh)continue;mesh.updateMatrix();if(!bins.has(mesh.material))bins.set(mesh.material,[]);bins.get(mesh.material).push(mesh.matrix.clone());part.remove(mesh);}
   for(const [material,matrices] of bins){const mesh=new T.InstancedMesh(unit,material,matrices.length);matrices.forEach((m,i)=>mesh.setMatrixAt(i,m));mesh.castShadow=true;mesh.receiveShadow=true;part.add(mesh);}
  }
  seed=previousSeed;
  return {root,body,torso,head,leftArm,rightArm,leftLeg,rightLeg,weights,bottle:drink,towel:handTowel,clipboard,page,shirt:characterCloth};
 }
 return {training,actor,box,group,mat,textured,decal,plant,towel,bottle,plate,dumbbell,batch,wood,woodLight,rubber,cloth,iron,dark,edge,gold,ivory,warm,unit};
}
