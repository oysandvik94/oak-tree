package oaktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Set PI_ASK_REMOTE_SOURCE to the installed pi-ask src/remote-ask.ts to run the
// contract harness without a Pi session or model call.
func TestPiOfficeInstalledAskContract(t *testing.T) {
	source := os.Getenv("PI_ASK_REMOTE_SOURCE")
	if source == "" {
		t.Skip("PI_ASK_REMOTE_SOURCE not set")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = os.WriteFile(filepath.Join(dir, "remote-ask.ts"), data, 0600); err != nil {
		t.Fatal(err)
	}
	script := `
import { createRemoteAskRuntime, applyRemoteAskResponse } from './remote-ask.ts';
const handlers=new Map(); const events={on(name,fn){const a=handlers.get(name)||[];a.push(fn);handlers.set(name,a);return ()=>{}},emit(name,data){for(const fn of handlers.get(name)||[])fn(data)}};
const results=[]; let started, completed, state={questions:[{id:'q',type:'single',options:[{value:'yes',label:'Yes'}]}],answers:{}};
events.on('@eko24ive/pi-ask:started', e=>started=e); events.on('@eko24ive/pi-ask:completed',e=>{completed=e;results.push('completed')}); events.on('@eko24ive/pi-ask:submit-result',e=>results.push(e));
const runtime=createRemoteAskRuntime(events); let handle;
handle=runtime.startFlow({source:'tool',toolCallId:'call',questions:state.questions,onSubmit(response){const r=applyRemoteAskResponse(state,response);if(!r.ok)return r;state=r.state;handle.complete({cancelled:false});return {ok:true}}});
await Promise.resolve(); if (!started || started.toolCallId!=='call') throw Error('started missing');
const send=(requestId,value)=>events.emit('@eko24ive/pi-ask:submit',{version:1,requestId,flowId:started.flowId,response:{kind:'answer',answers:{q:{values:[value]}}}});
send('bad','unknown'); if (results[0].error!=='invalid_answer') throw Error('invalid answer accepted');
send('good','yes'); if (!completed || results[1]!=='completed' || !results[2].ok) throw Error('completion ordering incorrect');
if (state.answers.q.selected[0].label!=='Yes' || state.answers.q.selected[0].index!==1) throw Error('normalization incorrect');
send('late','yes'); if (results[3].error!=='flow_not_found') throw Error('late duplicate accepted');
runtime.disposeAll();
`
	file := filepath.Join(dir, "contract.mjs")
	if err = os.WriteFile(file, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, file)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("installed pi-ask contract harness: %v: %s", err, output)
	}
}

func TestPiOfficeToolFailureEvents(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	dir, err := os.MkdirTemp("", "oak-failure-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	source := PiExtensionSource
	for _, line := range []string{`import { truncateHead } from "@earendil-works/pi-coding-agent";`, `import { StringEnum } from "@earendil-works/pi-ai";`, `import { Type } from "typebox";`} {
		source = strings.Replace(source, line, "", 1)
	}
	source = strings.Replace(source, "export default function (pi)", "function install(pi)", 1)
	script := source + `
const handlers = new Map();
const events = {on(){},emit(){}};
const pi = {events, on(name,fn){handlers.set(name,fn)}, registerCommand(){},registerShortcut(){},getSessionName:()=>'',exec:async()=>({code:0,stdout:''}),getAllTools:()=>['campfire_update','campfire_read','oak_coordination','ask_user'].map(name=>({name}))};
install(pi);
const ctx={mode:'tui',hasUI:false,cwd:'/tmp',sessionManager:{getSessionId:()=> 'pi',getSessionFile:()=>'/tmp/pi',getBranch:()=>[]}};
await handlers.get('session_start')({},ctx);
const dir=path.join(process.env.OAK_TREE_STATE_DIR,'office-bridges');
const socket=path.join(dir,(await fs.promises.readdir(dir))[0]);
async function list() { return await new Promise((resolve,reject)=>{let text='';const c=net.createConnection(socket,()=>c.write('{"op":"list"}\n'));c.on('data',d=>text+=d);c.on('end',()=>resolve(JSON.parse(text)));c.on('error',reject)}); }
for (const isError of [true,false,true,undefined,true]) await handlers.get('tool_execution_end')({toolName:'fake',toolCallId:'x',isError},ctx);
await handlers.get('tool_execution_end')({toolName:'fake',toolCallId:'x',isError:true},{...ctx,mode:'rpc'});
const reply=await list(); if (reply.toolFailures.count!==3) throw Error('wrong finalized error count');
await handlers.get('session_shutdown')({},ctx);
`
	file := filepath.Join(dir, "failure-test.mjs")
	if err := os.WriteFile(file, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, file)
	cmd.Env = append(os.Environ(), "OAK_TREE_STATE_DIR="+dir, "OAK_TREE_SESSION_ID=s", "TMUX_PANE=%1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failure event harness: %v: %s", err, output)
	}
}

func TestPiOfficeBridgeMockEvents(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	dir, err := os.MkdirTemp("", "oak-bridge-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	script := piOfficeBridgeSource + `
const listeners = new Map();
const events = { on(name, fn) { const list = listeners.get(name) || []; list.push(fn); listeners.set(name, list); }, emit(name, data) { for (const fn of listeners.get(name) || []) fn(data); } };
const bridge = createOfficeQuestionBridge({events}, () => ({session_id:'pi', tmux_pane:'%1', session_file:'/tmp/pi'}));
await bridge.start();
const started = {version:1, flowId:'tool:one', toolCallId:'one', source:'tool', createdAt:1, questions:[{id:'q',prompt:'Question',type:'single',options:[{value:'x',label:'X'}]}]};
events.emit('@eko24ive/pi-ask:started', started);
const dir=path.join(process.env.OAK_TREE_STATE_DIR,'office-bridges');
let socket=path.join(dir,(await fs.promises.readdir(dir))[0]);
async function call(payload) { return await new Promise((resolve,reject) => { let text=''; const c=net.createConnection(socket,()=>c.write(JSON.stringify(payload)+'\n')); c.on('data',d=>text+=d); c.on('end',()=>resolve(JSON.parse(text))); c.on('error',reject); }); }
const listed=await call({op:'list'}); if (!listed.ok || listed.flows.length!==1) throw Error('flow missing');
if (!listed.toolFailures?.generation || listed.toolFailures.count!==0) throw Error('failure baseline missing');
bridge.recordToolFailure(); bridge.recordToolFailure(); bridge.recordToolFailure();
await bridge.start();
const burst=await call({op:'list'}); if (burst.toolFailures.generation!==listed.toolFailures.generation || burst.toolFailures.count!==3) throw Error('burst or redundant start failed');
const oldId=listed.flows[0].id;
events.emit('@eko24ive/pi-ask:started', started);
const replacement=await call({op:'list'}); const id=replacement.flows[0].id;
if (replacement.flows.length!==1 || id===oldId) throw Error('replay did not replace published identity');
let attempts=0;
events.on('@eko24ive/pi-ask:submit', event => { attempts++; if (attempts===1) { events.emit('@eko24ive/pi-ask:submit-result',{version:1,requestId:event.requestId,flowId:event.flowId,ok:false,error:'invalid_answer'}); return; } if (attempts===2) events.emit('@eko24ive/pi-ask:completed',{version:1,flowId:event.flowId}); events.emit('@eko24ive/pi-ask:submit-result',{version:1,requestId:event.requestId,flowId:event.flowId,ok:true}); });
const request={op:'submit',id:oldId,requestId:'stale',identity:listed.identity,response:{kind:'answer',answers:{q:{values:['x']}}}};
if ((await call(request)).error!=='closed' || attempts!==0) throw Error('stale replay answered new flow');
request.id=id; request.requestId='first';
if ((await call(request)).error!=='invalid') throw Error('invalid answer not reported');
request.requestId='second'; if (!(await call(request)).ok) throw Error('completion-before-ack failed');
request.requestId='third'; if ((await call(request)).error!=='closed' || attempts!==2) throw Error('duplicate was delivered');
events.emit('@eko24ive/pi-ask:started', {...started, flowId:'tool:two', toolCallId:'two'});
const next=await call({op:'list'}); request.id=next.flows[0].id; request.requestId='ack-only';
if (!(await call(request)).ok || (await call({op:'list'})).flows.length!==0) throw Error('successful ack did not clear publication');
events.emit('@eko24ive/pi-ask:started', {...started,version:2});
if ((await call({op:'list'})).flows.length!==0) throw Error('unsupported event version was published');
await bridge.stop();
bridge.recordToolFailure();
await bridge.start();
socket=path.join(dir,(await fs.promises.readdir(dir))[0]);
const restarted=await call({op:'list'});
if (restarted.toolFailures.count!==0 || restarted.toolFailures.generation===listed.toolFailures.generation) throw Error('bridge generation not reset');
await bridge.stop();
`
	file := filepath.Join(dir, "bridge-test.mjs")
	if err := os.WriteFile(file, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, file)
	cmd.Env = append(os.Environ(), "OAK_TREE_STATE_DIR="+dir, "OAK_TREE_SESSION_ID=s", "OAK_TREE_AGENT_ROLE=architect")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bridge harness: %v: %s", err, output)
	}
}
