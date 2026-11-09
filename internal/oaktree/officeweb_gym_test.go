package oaktree

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"
)

func TestOfficeGymAssets(t *testing.T) {
	handler := officeHandler(NewStore(t.TempDir()), 4321, "token", nil)
	for _, path := range []string{"/assets/gym/view.js", "/assets/gym/art.js", "/assets/gym/state.js", "/assets/gym/motion.js", "/assets/gym/equipment.js", "/assets/gym/font.js", "/assets/gym/lighting.js", "/assets/gym/vendor/three.module.js", "/assets/gym/vendor/OrbitControls.js", "/assets/gym/vendor/RoomEnvironment.js"} {
		t.Run(path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4321"+path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusOK || w.Body.Len() == 0 {
				t.Fatalf("asset response: %d", w.Code)
			}
			if w.Header().Get("Content-Security-Policy") == "" {
				t.Fatal("missing CSP")
			}
		})
	}
}

func TestOfficeGymPlacement(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	source, err := officeAssets.ReadFile("officeweb/assets/gym/state.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(source) + `
const assert = (condition, message) => { if (!condition) throw new Error(message); };
assert(agentState({status:'ready',sessionState:'working'}) === 'ready', 'idle junior incorrectly trains');
assert(agentState({status:'working',sessionState:'question'}) === 'working', 'busy peer incorrectly asks');
for (const state of ['review','blocked']) assert(agentState({status:'working',sessionState:state}) === state, 'parking override lost');
assert(agentState({status:'unknown',sessionState:'working'}) === 'unknown', 'unknown state fabricated');
const capacity={working:30,question:15,ready:24,review:12,blocked:9};
const layout=layoutFor(capacity);
for (const state of Object.keys(capacity)) {
 const room=layout[state], seen=new Set(), stations=new Set();
 for(let i=0;i<capacity[state];i++) {
  const p=slotFor(layout,state,i), key=JSON.stringify(p);
  assert(!seen.has(key),'duplicate slot');seen.add(key);
  assert(p.x>room.x-room.width/2 && p.x<room.x+room.width/2 && p.z>room.back && p.z<room.front,'slot outside room');
  const station=stationFor(layout,state,i), point=station.position, stationKey=JSON.stringify(point);
  assert(!stations.has(stationKey),'duplicate furniture station');stations.add(stationKey);
  assert(point.x>room.x-room.width/2 && point.x<room.x+room.width/2 && point.z>room.back && point.z<room.front,'station outside room');
  assert(station.kind===(state==='ready'&&i<2?'bench':state==='review'&&i<2?'desk':'floor'),'incorrect furniture assignment');
 }
}
assert(layout.equipment.length===6,'training wing lacks six equipment stations');
for(const kind of ['squat-rack','deadlift-platform','pec-deck'])assert(layout.equipment.filter(station=>station.kind===kind).length===2,'incorrect equipment count');
assert(new Set(layout.equipment.map(station=>station.id)).size===6,'duplicate equipment ids');
for(const station of layout.equipment){
 const {x,z}=station.position,room=layout.working;
 assert(x-1.7>room.x-room.width/2&&x+1.7<room.x+room.width/2&&z-1.4>room.back&&z+1.8<room.front,'equipment lacks clearance');
 for(const target of layout.equipment){
  const path=stationRoute(layout,station.position,target);
  assert(path.at(-1)===target.position,'equipment route misses its destination');
  let start=station.position;
  for(const end of path){
   const steps=Math.ceil(Math.hypot(end.x-start.x,end.z-start.z)/.2);
   for(let i=0;i<=steps;i++){
    const x=start.x+(end.x-start.x)*i/steps,z=start.z+(end.z-start.z)*i/steps;
    for(const obstacle of layout.equipment.filter(item=>item!==station&&item!==target))assert(Math.abs(x-obstacle.position.x)>=1.7||Math.abs(z-obstacle.position.z)>=1.4,'route crosses another machine');
   }
   start=end;
  }
 }
}
const workers=Array.from({length:8},(_,index)=>{
 const floorStation={kind:'floor',id:'floor-'+index,position:slotFor(layout,'working',index)};
 return {data:{id:'worker-'+index},station:floorStation,floorStation,position:{...floorStation.position},path:[],rotationAt:0,lastEquipment:null};
});
const apply=(changes,time)=>changes.forEach(({worker,station})=>{
 worker.station=station;worker.position={...station.position};worker.rotationAt=time+(isEquipment(station)?25:5);
 if(isEquipment(station))worker.lastEquipment=station.id;
});
const first=trainingRotations(workers,layout.equipment,0);
assert(first.length===6&&new Set(first.map(change=>change.station.id)).size===6,'initial reservations overlap');
apply(first,0);
assert(trainingRotations(workers,layout.equipment,25,workers,{paused:true}).length===0,'paused workers rotate');
const exits=trainingRotations(workers,layout.equipment,25);
assert(exits.length===6&&exits.every(change=>change.station.kind==='floor'),'set does not end with a rest');
apply(exits,25);
const waiting=trainingRotations(workers,layout.equipment,26);
assert(waiting.length===2&&waiting.every(change=>['worker-6','worker-7'].includes(change.worker.data.id)),'overflow agents starve');
apply(waiting,26);
const next=trainingRotations(workers,layout.equipment,30);
assert(next.length===4&&new Set([...waiting,...next].map(change=>change.station.id)).size===6,'rotation double-books equipment');
const departed=waiting[0];
const survivors=workers.filter(worker=>worker!==departed.worker);
assert(trainingRotations(survivors,layout.equipment,30).some(change=>change.station.id===departed.station.id),'removed agent keeps its reservation');
const leaving=workers[0];leaving.station=leaving.floorStation;leaving.position={...layout.equipment[2].position};leaving.rotationAt=100;
assert(!trainingRotations(workers,layout.equipment,30).some(change=>change.station.id===layout.equipment[2].id),'station reused before departing actor cleared it');
const a=slotFor(layout,'working',0),b=slotFor(layout,'ready',0);
const path=routeTo(a,b,layout.corridor);
assert(path[0].z===layout.corridor && path[path.length-1]===b,'transition bypasses corridor');
assert(zones.length===5,'unexpected extra zones');
assert(hash('stable-id')===hash('stable-id'),'identity unstable');
`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, node, "--input-type=module", "-e", script)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("gym placement: %v\n%s", err, output)
	}
}

func TestOfficeGymMotion(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	source, err := officeAssets.ReadFile("officeweb/assets/gym/motion.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(source) + `
const assert = (condition, message) => { if (!condition) throw new Error(message); };
for (const state of ['working','question','ready','review','blocked','unknown']) {
 const observed=new Set();
 for(let identity=0;identity<12;identity++)for(let time=0;time<180;time++) {
  const pose=gymPose(state,identity,time);
  observed.add(pose.routine);
  for(const value of Object.values(pose))if(typeof value==='number')assert(Number.isFinite(value),'non-finite pose');
  assert(Math.abs(pose.offsetX)<=.45,'pacing leaves its floor slot');
  assert(pose.bodyY>=-.35,'body sinks through the floor');
  assert(gymPose(state,identity,time,{still:true}).offsetX===0,'paused actor wanders');
 }
 assert(observed.size>=2,'state lacks varied routines');
}
for(let time=0;time<120;time++) {
 const pose=gymPose('ready',42,time,{seated:true});
 assert(pose.leftHip===-Math.PI/2&&pose.rightHip===-Math.PI/2,'seated thighs are not bent');
 assert(pose.leftKnee===Math.PI/2&&pose.rightKnee===Math.PI/2,'seated knees are not bent');
 assert(pose.offsetX===0,'seated actor wanders off the bench');
}
for(const equipment of ['squat-rack','deadlift-platform','pec-deck'])for(let elapsed=0;elapsed<35;elapsed+=.25){
 const pose=gymPose('working',42,elapsed,{equipment,equipmentTime:elapsed});
 assert(pose.routine===equipment&&pose.offsetX===0,'equipment user leaves its station');
 for(const value of Object.values(pose))if(typeof value==='number')assert(Number.isFinite(value),'non-finite equipment pose');
 assert(pose.bodyY>=-.35,'equipment user sinks through the floor');
 if(equipment==='squat-rack')assert(pose.handTargets.length===2,'squat lacks barbell grips');
 if(equipment==='deadlift-platform')assert(pose.leftElbow===0&&pose.rightElbow===0,'deadlift bends its arms');
 if(equipment==='pec-deck')assert(pose.leftHip===-Math.PI/2&&pose.leftKnee===Math.PI/2&&pose.machineLift>=0&&pose.machineLift<=1,'pec deck is not seated and synchronized');
 assert(gymPose('working',42,elapsed,{equipment,walking:true}).routine==='walk','travelling agent lifts equipment');
}
assert(gymPose('question',42,10,{eventAge:1}).routine==='hand-raise','new question is not acknowledged');
assert(gymPose('ready',42,10,{eventAge:1}).routine==='stretch','completion has no reaction');
assert(gymPose('working',42,10,{walking:true}).routine==='walk','transition does not walk');
assert(JSON.stringify(gymPose('working',42,10))===JSON.stringify(gymPose('working',42,10)),'animation is nondeterministic');
`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, node, "--input-type=module", "-e", script)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("gym motion: %v\n%s", err, output)
	}
}

func TestOfficeGymCachedInspectorData(t *testing.T) {
	todo := &TodoSummary{Total: 1, InProgress: 1, Tasks: []TodoTask{{Subject: "Current work", Status: "in_progress"}}}
	pr := &PRInfo{Found: true, Number: 42, ChecksState: "success"}
	snapshot := officeSnapshotFrom([]Session{{ID: "gym", Root: "/project", AgentStatus: AgentStatusWorking, Todo: todo, PR: pr}}, nil, nil, time.Now())
	if len(snapshot.Agents) != 1 || snapshot.Agents[0].Todo != todo || snapshot.Agents[0].PR != pr {
		t.Fatalf("missing cached inspector data: %+v", snapshot.Agents)
	}
}

func TestOfficeGymLocalLighting(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	source, err := officeAssets.ReadFile("officeweb/assets/gym/lighting.js")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TZ", "Pacific/Honolulu")
	script := string(source) + `
const assert = (condition, message) => { if (!condition) throw new Error(message); };
const at = value => daylightAt(new Date(value));
assert(at('2026-12-21T08:00:00+01:00')===0, 'Bergen winter morning should be dark');
assert(at('2026-12-21T17:00:00+01:00')===0, 'Bergen winter evening should be dark');
assert(at('2026-12-21T12:30:00+01:00')>.95, 'Bergen winter noon should have daylight');
assert(at('2026-06-21T08:00:00+02:00')===1, 'Bergen summer morning should be bright');
assert(at('2026-06-21T21:00:00+02:00')>.9, 'Bergen summer evening should remain bright');
const twilight=at('2026-12-21T09:30:00+01:00');
assert(twilight>0&&twilight<.5, 'winter dawn should blend gradually');
assert(at('2026-06-21T21:00:00+02:00')===at('2026-06-21T19:00:00Z'), 'timezone changed the same instant');
const tromso={latitude:69.6492,longitude:18.9553};
assert(solarElevationAt(new Date('2026-06-21T00:00:00+02:00'),tromso)>0, 'missing midnight sun');
assert(solarElevationAt(new Date('2026-12-21T12:00:00+01:00'),tromso)<0, 'missing polar night');
for(const latitude of [-90,90])for(const month of [0,5])for(let hour=0;hour<24;hour++){
 const value=daylightAt(new Date(Date.UTC(2026,month,21,hour)),{latitude,longitude:0});
 assert(Number.isFinite(value)&&value>=0&&value<=1, 'invalid polar lighting');
}
for(const day of ['2026-03-29','2026-10-25','2028-02-29']){
 let previous;
 for(let minute=0;minute<1440;minute++){
  const date=new Date(day+'T00:00:00Z');date.setUTCMinutes(minute);
  const value=daylightAt(date);
  assert(Number.isFinite(value)&&value>=0&&value<=1, 'lighting outside range');
  if(previous!==undefined)assert(Math.abs(value-previous)<.02, 'lighting discontinuity');
  previous=value;
 }
}

`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, node, "--input-type=module", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("gym lighting: %v\n%s", err, output)
	}
}
