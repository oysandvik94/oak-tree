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
	for _, path := range []string{"/assets/gym/view.js", "/assets/gym/art.js", "/assets/gym/state.js", "/assets/gym/motion.js", "/assets/gym/font.js", "/assets/gym/lighting.js", "/assets/gym/vendor/three.module.js", "/assets/gym/vendor/OrbitControls.js", "/assets/gym/vendor/RoomEnvironment.js"} {
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
