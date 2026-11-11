package oaktree

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestPiSessionNaming(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimPrefix(PiExtensionSource, piOfficeBridgeSource), "\n") {
		if !strings.HasPrefix(line, "import ") {
			lines = append(lines, line)
		}
	}
	source := strings.Replace(strings.Join(lines, "\n"), "export default function (pi)", "function install(pi)", 1)
	script := `
import assert from 'node:assert/strict';
const createOfficeQuestionBridge=()=>({stop:async()=>{}});
` + source + `
const tick=()=>new Promise(setImmediate);
const current={provider:'example',id:'large-model'};
function fixture({role='architect',entries=[],scoped=[],models=[],result,stream,name=''}={}) {
  process.env.OAK_TREE_SESSION_ID='test';process.env.OAK_TREE_AGENT_ROLE=role;process.env.TMUX_PANE='%fake';
  const handlers=new Map(),calls=[],hooks=[],warnings=[];
  const state={name};
  const pi={on:(event,handler)=>handlers.set(event,handler),events:{on(){}},registerCommand(){},registerShortcut(){},
    getSessionName:()=>state.name,exec:async(_command,args)=>{hooks.push(args);return {code:0};}};
  install(pi);
  const ctx={mode:'tui',hasUI:true,cwd:'/fixture',model:current,scopedModels:scoped.map(model=>({model})),
    sessionManager:{getEntries:()=>entries,getSessionId:()=> 'pi-fixture',getSessionFile:()=> 'fixture.jsonl'},
    ui:{notify:message=>warnings.push(message)},modelRegistry:{getAvailable:()=>models,
      streamSimple:(model,context,options)=>{calls.push({model,context,options});return {result:()=>stream?stream(options):Promise.resolve(result||{stopReason:'stop',content:[{type:'text',text:'Useful Session Title'}]})};}}};
  return {calls,hooks,warnings,state,ctx,fire:(event,data)=>handlers.get(event)(data,ctx),input(text='Fix the bug',source='interactive'){return handlers.get('input')({text,source},ctx);}};
}
const named=f=>f.hooks.filter(args=>args[args.indexOf('--event')+1]==='session_name_auto');
let f=fixture({scoped:[current]});
assert.equal(f.input(),undefined,'naming blocked prompt processing');await tick();
assert.equal(f.calls[0].model,current,'scoped list without a small model lost naming');
assert.equal(named(f).length,1);assert.equal(named(f)[0].at(-1),'Useful Session Title');
f.input('A second prompt');await tick();assert.equal(f.calls.length,1,'later prompt renamed session');
const small={provider:'example',id:'tiny-small',cost:{input:1,output:1}};
f=fixture({scoped:[current,small,{...small,id:'expensive-small',cost:{input:5,output:5}},{...small,provider:'other',cost:{input:0,output:0}}]});
f.input();await tick();assert.equal(f.calls[0].model,small,'small same-provider scoped model not preferred');
f=fixture({models:[small]});f.input();await tick();assert.equal(f.calls[0].model,small,'unscoped catalogue ignored');
for(const options of [{role:'junior'},{name:'Manual name'},{entries:[{type:'message',message:{role:'user'}}]}]){
  f=fixture(options);f.input();await tick();assert.equal(f.calls.length,0,'junior, manual or existing session was named');
}
f=fixture();f.input(' ');f.input('Injected','extension');await tick();assert.equal(f.calls.length,0);
f=fixture({name:'oak-test-architect'});f.input();await tick();assert.equal(named(f).length,1,'routing name blocked real title');
let resolve;
f=fixture({stream:()=>new Promise(done=>{resolve=done;})});f.input();
f.state.name='My manual name';f.fire('session_info_changed',{name:f.state.name});
assert.equal(f.calls[0].options.signal.aborted,true,'manual rename did not cancel request');
resolve({stopReason:'stop',content:[{type:'text',text:'Late automatic name'}]});await tick();
assert.equal(named(f).length,0,'late title overwrote manual name');assert.equal(f.warnings.length,0);
assert.ok(f.hooks.some(args=>args.includes('session_name_manual')));
for(const options of [{result:{stopReason:'error',content:[]}},{result:{stopReason:'stop',content:[]}},{stream:()=>{throw new Error('mock failure');}}]){
  f=fixture(options);f.input();await tick();assert.equal(named(f).length,0);assert.equal(f.warnings.length,1,'naming failure was silent');
}
f=fixture({stream:()=>new Promise(done=>{resolve=done;})});f.input();await f.fire('session_shutdown',{});
resolve({stopReason:'aborted',content:[]});await tick();assert.equal(named(f).length,0);assert.equal(f.warnings.length,0,'shutdown showed spurious failure');
const nativeTimeout=globalThis.setTimeout;
let timeoutReached;
const aborted=new Promise(done=>{timeoutReached=done;});
try {
  globalThis.setTimeout=(fn,delay,...args)=>nativeTimeout(fn,delay===15000?0:delay,...args);
  f=fixture({stream:({signal})=>new Promise(done=>signal.addEventListener('abort',()=>{done({stopReason:'aborted',content:[]});timeoutReached();},{once:true}))});
  f.input();await aborted;await tick();assert.equal(f.warnings.length,1,'timeout was silent');assert.equal(named(f).length,0);
} finally {globalThis.setTimeout=nativeTimeout;}
`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, node, "--input-type=module", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("session naming: %v\n%s", err, output)
	}
}

func TestSessionNamingPersistsMetadataAndPreservesManualName(t *testing.T) {
	store := NewStore(t.TempDir())
	session := Session{ID: "naming", Root: "/repo/example", Branch: "feature/title", RightPaneID: "%fake", Agents: map[string]PairedAgent{"architect": {}, "junior": {}}}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Paths{StateDir: store.StateDir}, store, &stubRunner{})
	send := func(event, name string) {
		t.Helper()
		if err := svc.HandleAgentEvent(context.Background(), AgentEvent{OakSessionID: session.ID, AgentRole: "architect", Event: event, SessionName: name}); err != nil {
			t.Fatal(err)
		}
	}
	send("session_name_auto", "First Prompt Title")
	got, _ := store.LoadSession(session.ID)
	if got.Name != "First Prompt Title" || got.NameManual {
		t.Fatalf("automatic metadata = %+v", got)
	}
	send("session_name_manual", "My Title")
	send("session_name_auto", "Late Title")
	got, _ = store.LoadSession(session.ID)
	if got.Name != "My Title" || !got.NameManual {
		t.Fatalf("manual name was overwritten: %+v", got)
	}
	send("session_name_manual", "")
	send("session_name_auto", "Another Late Title")
	got, _ = store.LoadSession(session.ID)
	if got.Name != "" || !got.NameManual || sessionName(got) != session.Branch {
		t.Fatalf("cleared manual name lost useful fallback: %+v", got)
	}
}
