package oaktree

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestOfficeGymOverlays(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	script := officeGymScript(t, "overlays") + `
const assert=(condition,message)=>{if(!condition)throw new Error(message);};
const overlap=(a,b)=>Math.abs(a.x-b.x)<(a.width+b.width)/2&&a.y<b.y+b.height&&b.y<a.y+a.height;
const items=Array.from({length:10},(_,id)=>({id,x:120,y:220,width:140,height:18}));
const labels=placeOverlays(items,{width:360,height:500});
assert(labels.length===items.length,'overlapping labels were hidden');
for(let i=0;i<labels.length;i++){
  const a=labels[i];
  assert(a.x>=a.width/2+8&&a.x+a.width/2<=352&&a.y>=8&&a.y+a.height<=492,'nameplate left viewport');
  for(const b of labels.slice(i+1))assert(!overlap(a,b),'nameplates overlap');
}
const bubbles=placeOverlays([{x:120,y:220,width:240,height:40},{x:125,y:220,width:240,height:40}],{width:360,height:500,occupied:labels,direction:-1});
assert(bubbles.length===2&&!overlap(bubbles[0],bubbles[1]),'simultaneous notifications overlap');
for(const bubble of bubbles)for(const label of labels)assert(!overlap(bubble,label),'bubble covered a nameplate');
const edge=placeOverlays([{x:900,y:900,width:140,height:18}],{width:360,height:500})[0];
assert(edge.x+edge.width/2<=352&&edge.y+edge.height<=492,'edge label was clipped');
const node={style:{},lastElementChild:{style:{}}};
positionOverlay(node,edge,{x:300,y:450});
assert(!JSON.stringify(node).includes('NaN'),'invalid tether geometry');
`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, node, "--input-type=module", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("gym overlays: %v\n%s", err, output)
	}
}
