// Run against an isolated `oak-tree office --serve --state-dir <tempdir>` instance.
// Every API request is intercepted: no real agent receives an action.
const {chromium} = require('playwright');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');

(async () => {
  const stateDir = process.argv[2];
  if (!stateDir) throw new Error('Pass the isolated preview state directory.');
  const record = JSON.parse(fs.readFileSync(path.join(stateDir, 'office.json'), 'utf8'));
  const browser = await chromium.launch({headless:true, executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE || undefined, args:['--no-sandbox','--enable-unsafe-swiftshader']});
  const errors=[], calls=[];
  const now=new Date().toISOString();
  const todo=completed=>({total:completed+1,pending:0,in_progress:1,completed,tasks:[...Array.from({length:completed},(_,i)=>({subject:`Finished step ${i+1}`,status:'completed'})),{subject:'Integrate the live gym',status:'in_progress'}]});
  const make=(id,status,sessionState=status)=>({id,sessionId:id.split(':')[0],project:'sample-project',name:id.split(':')[0],branch:'feature/gym',role:id.split(':')[1]||'architect',status,sessionState,activity:'Integrate the live gym',activitySource:'task',updatedAt:now,notes:[],questions:[],canReply:true,latestAnswer:'The latest answer is ready.',todo:todo(1),pr:{found:true,number:42,state:'OPEN',checks_state:'success',review_decision:'APPROVED',url:'https://github.com/example/example/pull/42',refreshed_at:now}});
  let agents=[make('Atlas:architect','working'),make('Atlas:junior','ready','working'),make('Cedar:architect','question'),make('Fern:architect','ready'),make('Moss:architect','working','review'),make('Sage:architect','ready','blocked'),make('Pine:architect','working'),make('Willow:architect','unknown','working')];
  agents[0].group=agents[6].group={id:'group-one',name:'Actual fixture group'};
  agents[2].questions=[{id:'fixture-question',title:'Choose a direction',source:'tool',questions:[{id:'choice',prompt:'Which direction?',type:'single',options:[{value:'yes',label:'Continue'}]}]}];
  let history=[], offline=false;
  async function fixtures(page) {
    page.on('pageerror',e=>{errors.push(e.message);console.error('PAGE ERROR:',e.message);});
    page.on('console',m=>{if(m.type()==='error')errors.push(m.text());});
    await page.route('**/api/**',async route=>{
      const request=route.request(),endpoint=new URL(request.url()).pathname;
      if(offline&&endpoint==='/api/state')return route.fulfill({status:503,body:'offline'});
      if(endpoint==='/api/state')return route.fulfill({json:{agents,history,updatedAt:now}});
      calls.push({endpoint,body:request.postDataJSON()});
      if(endpoint==='/api/notes')return route.fulfill({json:{note:{id:'note-one',text:request.postDataJSON().text,created_at:now}}});
      if(endpoint==='/api/activity/clear')history=[];
      if(endpoint==='/api/answer')agents[2].questions=[];
      return route.fulfill({json:{ok:true,status:'answered'}});
    });
  }
  const url=`http://127.0.0.1:${record.port}/#token=${record.token}`;
  try {
    const page=await browser.newPage({viewport:{width:1680,height:1050},reducedMotion:'reduce',timezoneId:'Europe/Oslo'});
    page.setDefaultTimeout(15000);await fixtures(page);await page.goto(url);
    await page.waitForFunction(()=>window.oakGym?.getInfo().agents===8);
    console.log('Loaded live fixture; checking state mapping and camera controls.');
    assert.equal(await page.locator('#connection').textContent(),'Live');
    assert.equal((await page.evaluate(()=>window.oakGym.getInfo())).positions['Atlas:junior'].spotting,'Atlas:architect');
    assert.match(await page.locator('[data-agent-id="Atlas:junior"] .agent-role').textContent(),/Spotting.*Session busy/);
    assert.equal((await page.evaluate(()=>window.oakGym.getInfo())).positions['Sage:architect'].state,'blocked');
    const initialMotion=await page.evaluate(()=>window.oakGym.getInfo());
    assert.equal(initialMotion.animationTime,0,'reduced-motion preference should pause routines');
    assert.equal(initialMotion.equipment.length,8);
    assert.equal(new Set(initialMotion.equipment.map(station=>station.kind)).size,8);
    const occupied=initialMotion.equipment.filter(station=>station.occupant);
    assert.equal(occupied.length,2);
    assert.equal(new Set(occupied.map(station=>station.occupant)).size,2,'two machines share an occupant');
    assert.ok(initialMotion.equipment.some(station=>station.id===initialMotion.positions['Atlas:architect'].stationId));
    assert.equal(initialMotion.positions['Atlas:junior'].station,'spotter');
    assert.equal(initialMotion.positions['Fern:architect'].station,'bench');
    assert.equal(initialMotion.positions['Atlas:architect'].loadLevel,1);
    agents[0].todo=todo(3);
    await page.waitForFunction(()=>window.oakGym.getInfo().positions['Atlas:architect'].loadLevel===3);
    const loaded=await page.evaluate(()=>window.oakGym.getInfo());
    assert.equal(loaded.positions['Atlas:junior'].loadLevel,1,'loads must remain role-specific');
    assert.equal(loaded.equipment.find(station=>station.occupant==='Atlas:architect').loadLevel,3);
    agents[0].todo=todo(0);
    await page.waitForFunction(()=>window.oakGym.getInfo().positions['Atlas:architect'].loadLevel===0);
    assert.equal(await page.evaluate(()=>window.oakGym.getInfo().animationTime),0,'todo progress must update with reduced motion');
    agents[0].todo=todo(1);
    assert.equal(initialMotion.positions['Moss:architect'].station,'desk');
    assert.equal(await page.locator('.agent-row').count(),8);
    assert.equal(await page.locator('.gym-agent-label:visible').count(),8,'onscreen labels must not depend on focus');
    for(const label of await page.locator('.gym-agent-label:visible').all()){
      const box=await label.boundingBox();assert.ok(box.height<=24&&box.width<=144,'nameplate is not compact');
    }
    assert.equal(await page.locator('.topbar,.scene-toolbar,.gym-options,#room-navigation,.scene-footer').count(),0);
    const stageBounds=await page.locator('#stage').boundingBox();
    assert.equal(stageBounds.y,0);assert.equal(stageBounds.height,1050);
    await page.locator('#office').focus();await page.keyboard.press('f');
    await page.screenshot({path:process.env.OAK_GYM_SCREENSHOT || '/tmp/oak-gym-integrated.png'});
    const atlas = (await page.evaluate(()=>window.oakGym.getInfo())).positions['Atlas:architect'].screen;
    const canvasBounds=await page.locator('#office').boundingBox();
    await page.mouse.move(canvasBounds.x+atlas.x,canvasBounds.y+atlas.y);
    await page.waitForFunction(()=>document.querySelector('.gym-agent-label[data-overlay-agent="Atlas:architect"]:not([hidden])'));
    assert.equal(await page.locator('.gym-agent-label:visible').count(),8,'hover must not hide other nameplates');
    await page.locator('#office').click({position:{x:atlas.x,y:atlas.y}});
    assert.equal(await page.locator('[data-agent-id="Atlas:architect"]').getAttribute('aria-pressed'),'true','3D raycast selected the wrong agent');
    await page.click('[data-agent-id="Atlas:architect"]');
    assert.equal(await page.locator('#inspector').isVisible(),true,'clicking the selected agent should keep it selected');
    await page.mouse.move(canvasBounds.x+12,canvasBounds.y+12);await page.mouse.down();
    await page.mouse.move(canvasBounds.x+42,canvasBounds.y+12,{steps:5});await page.mouse.up();
    assert.equal(await page.locator('#inspector').isVisible(),true,'camera dragging should preserve selection');
    await page.locator('#office').click({position:{x:12,y:12}});
    assert.equal(await page.locator('#inspector').isVisible(),false,'empty scene click should deselect');
    assert.equal(await page.locator('.agent-row[aria-pressed="true"]').count(),0);
    const originalName=agents[0].name;agents[0].name='Atlas refreshed';
    await page.waitForFunction(()=>document.querySelector('[data-agent-id="Atlas:architect"] .agent-name')?.textContent==='Atlas refreshed');
    assert.equal(await page.locator('#inspector').isVisible(),false,'polling should preserve deselection');
    assert.ok(await page.locator('.gym-agent-label:visible').count()>0,'deselection hid permanent nameplates');
    assert.equal(await page.locator('.gym-agent-label[data-overlay-agent="Atlas:architect"] .label-name').textContent(),'Atlas refreshed');
    agents[0].name=originalName;
    await page.click('[data-agent-id="Atlas:architect"]');
    await page.keyboard.press('Escape');
    assert.equal(await page.locator('#inspector').isVisible(),false,'scene-focused Escape should deselect');
    assert.equal(await page.locator('.agent-row[aria-pressed="true"]').count(),0);
    await page.click('[data-agent-id="Atlas:architect"]');
    assert.match(await page.locator('#selected-activity').textContent(),/Integrate/);
    assert.match(await page.locator('#selected-pr').textContent(),/42.*success/);
    await page.click('#session-open');await page.waitForFunction(()=>document.querySelector('#interaction-status').textContent==='Session opened.');
    await page.click('#notes-open');await page.fill('#notes-text','A fixture-only note');await page.click('#notes-add');await page.waitForFunction(()=>document.querySelector('#notes-feedback').textContent==='Saved.');await page.click('#notes-close');
    await page.click('#conversation-open');await page.fill('#conversation-reply','A fixture-only reply');await page.click('#conversation-send');await page.waitForFunction(()=>document.querySelector('#conversation-feedback').textContent==='Sent or queued in Pi.');await page.click('#conversation-close');
    await page.click('[data-agent-id="Cedar:architect"]');await page.click('#question-launcher button');await page.check('input[value="yes"]');await page.click('#question-submit');await page.click('#question-close');
    console.log('Focus, notes, replies and question submission checked.');
    await page.locator('#office').focus();await page.keyboard.press('Escape');await page.keyboard.press('f');
    history=[{key:'one',agentId:'Atlas:architect',author:'Atlas',source:'CAMPFIRE UPDATE',text:'A real-shaped Campfire fixture.',time:now}];
    await page.waitForFunction(()=>document.querySelector('#chat-log').textContent.includes('Campfire fixture'));
    await page.waitForFunction(()=>document.querySelector('.speech-bubble[data-overlay-agent="Atlas:architect"]'));
    assert.equal(await page.locator('#inspector').isVisible(),false,'Campfire stole selection');
    const ribbon=await page.locator('.speech-bubble').boundingBox();assert.ok(ribbon.width<=250&&ribbon.height<=42,'Campfire is not a slim ribbon');
    history.push({key:'two',agentId:'Atlas:junior',author:'Atlas junior',source:'CAMPFIRE UPDATE',text:'Junior has a separate update.',time:now});
    await page.waitForFunction(()=>document.querySelectorAll('.speech-bubble').length===2);
    await page.screenshot({path:process.env.OAK_GYM_BUBBLES_SCREENSHOT || '/tmp/oak-gym-notifications.png'});
    await page.click('[data-agent-id="Cedar:architect"]');await page.locator('#office').focus();await page.keyboard.press('f');
    await page.waitForFunction(()=>document.querySelector('.speech-bubble[data-overlay-agent="Atlas:architect"]'));
    await page.waitForFunction(()=>document.querySelectorAll('.speech-bubble').length===0);
    assert.match(await page.locator('#chat-log').textContent(),/Campfire fixture/,'expired ribbon removed the full log message');
    await page.click('#chat-clear');await page.waitForFunction(()=>document.querySelector('#chat-log').children.length===0);
    agents[0].status='question';
    await page.waitForFunction(()=>window.oakGym.getInfo().positions['Atlas:architect'].state==='question');
    assert.equal((await page.evaluate(()=>window.oakGym.getInfo())).positions['Atlas:architect'].moving,false,'reduced-motion transition should snap');
    console.log('Activity and state transition checked; checking capacity changes.');
    // Exercise growth, then removal, without retaining ghost actors or labels.
    agents.push(...Array.from({length:8},(_,i)=>make(`Extra-${i}:architect`,'working')));
    await page.waitForFunction(()=>window.oakGym.getInfo().agents===16);
    console.log('Expanded to 16 agents.');
    const expanded=await page.evaluate(()=>window.oakGym.getInfo());
    assert.ok(expanded.capacity.working>=9);
    assert.equal(expanded.equipment.filter(station=>station.occupant).length,8);
    assert.equal(new Set(expanded.equipment.map(station=>station.occupant)).size,8,'growth double-booked a station');
    assert.equal(expanded.animationTime,0,'paused equipment rotations advanced');
    agents=agents.slice(0,3);await page.waitForFunction(()=>window.oakGym.getInfo().agents===3);
    console.log('Removed agents; checking scene controls.');
    assert.equal(await page.locator('.gym-agent-label').count(),3);
    assert.ok((await page.evaluate(()=>window.oakGym.getInfo())).equipment.every(station=>!station.occupant),'removed agents retained equipment reservations');
    await page.locator('#office').focus();await page.keyboard.press('Space');
    await page.keyboard.press('f');
    console.log('Capacity changes and presentation controls checked; checking reconnect.');
    offline=true;await page.waitForFunction(()=>document.querySelector('#connection').textContent==='Disconnected');
    assert.equal(await page.locator('#session-open').isDisabled(),true);offline=false;
    await page.waitForFunction(()=>document.querySelector('#connection').textContent==='Live');
    assert.equal(await page.locator('#session-open').isDisabled(),false);
    agents=[];await page.waitForFunction(()=>window.oakGym.getInfo().agents===0);assert.equal(await page.locator('#inspector').isVisible(),false);
    assert.equal(await page.locator('#stage-message').isVisible(),true);
    agents=[make('Mobile:architect','working')];
    await page.setViewportSize({width:700,height:900});await page.waitForFunction(()=>window.oakGym.getInfo().agents===1);
    assert.equal(await page.locator('#agent-list').isVisible(),true);
    assert.equal(await page.locator('.chat-panel').isVisible(),true);
    assert.equal(await page.locator('#roster-toggle').count(),0);
    assert.ok(calls.some(c=>c.endpoint==='/api/focus'));
    assert.ok(calls.some(c=>c.endpoint==='/api/message'&&c.body.text==='A fixture-only reply'));
    assert.ok(calls.some(c=>c.endpoint==='/api/answer'));
    for(const [time,expected] of [['2026-12-21T08:00:00+01:00',0],['2026-06-21T08:00:00+02:00',1],['2026-12-21T17:00:00+01:00',0],['2026-06-21T12:00:00+02:00',1]]){
      await page.clock.setFixedTime(new Date(time));
      await page.waitForFunction(expected=>Math.abs(window.oakGym.getInfo().daylight-expected)<.001,expected);
    }
    assert.equal(await page.evaluate(()=>window.oakGym.getInfo().animationTime),0,'Space should not override reduced motion');
    console.log('Bergen seasonal lighting updates while animation is paused.');
    await page.close();
    console.log('Offline, empty and mobile states checked; checking normal-motion travel.');
    const animated=await browser.newPage({viewport:{width:1200,height:900},reducedMotion:'no-preference',timezoneId:'Europe/Oslo'});
    animated.setDefaultTimeout(15000);await fixtures(animated);await animated.goto(url);await animated.waitForFunction(()=>window.oakGym?.getInfo().agents===1);
    const before=(await animated.evaluate(()=>window.oakGym.getInfo())).positions['Mobile:architect'];
    agents[0].status='question';
    await animated.waitForFunction(()=>window.oakGym.getInfo().positions['Mobile:architect'].state==='question');
    await animated.waitForFunction(({x,z})=>{const p=window.oakGym.getInfo().positions['Mobile:architect'];return p.routine==='run'&&p.moving&&Math.hypot(p.x-x,p.z-z)>.05;},before);
    const running=await animated.evaluate(async()=>{
      let last=window.oakGym.getInfo(),distance=0,elapsed=0;
      while(elapsed<.35){
        await new Promise(requestAnimationFrame);
        const info=window.oakGym.getInfo(),p=info.positions['Mobile:architect'],previous=last.positions['Mobile:architect'];
        if(info.animationTime>last.animationTime){
          if(p.routine!=='run')throw new Error('Question stopped running before arrival');
          distance+=Math.hypot(p.x-previous.x,p.z-previous.z);elapsed+=info.animationTime-last.animationTime;
        }
        last=info;
      }
      return distance/elapsed;
    });
    assert.ok(running>4.5&&running<=5.51,`question speed is not a run: ${running}`);
    await animated.locator('#office').focus();await animated.keyboard.press('Space');
    const afterSpace=await animated.evaluate(()=>window.oakGym.getInfo().animationTime);
    await animated.waitForFunction(time=>window.oakGym.getInfo().animationTime>time+.05,afterSpace);
    // Software WebGL advances the capped animation clock much slower than wall time.
    await animated.waitForFunction(()=>{const p=window.oakGym.getInfo().positions['Mobile:architect'];return !p.event&&p.routine!=='run';},null,{timeout:45000});
    assert.equal((await animated.evaluate(()=>window.oakGym.getInfo())).positions['Mobile:architect'].state,'question','running changed the real question state');
    await animated.close();
    // A failed WebGL initialization must not take the functional DOM UI down with it.
    console.log('Normal-motion travel checked; checking WebGL fallback.');
    const fallback=await browser.newPage();fallback.setDefaultTimeout(15000);await fixtures(fallback);
    await fallback.addInitScript(()=>{const original=HTMLCanvasElement.prototype.getContext;HTMLCanvasElement.prototype.getContext=function(kind,...args){if(kind.startsWith('webgl'))return null;return original.call(this,kind,...args);};});
    await fallback.goto(url);await fallback.waitForFunction(()=>document.querySelector('#connection').textContent==='Live');
    assert.equal(await fallback.locator('.agent-row').count(),1);
    assert.match(await fallback.locator('#connection-warning').textContent(),/3D rendering is unavailable/);
    const unexpected=errors.filter(e=>!e.includes('503')&&!e.includes('WebGL context')&&!e.includes('Gym rendering unavailable'));
    assert.deepEqual(unexpected,[]);
    console.log('PASS: five zones, equipment reservations, role states, inspector, focus, notes, replies, questions, activity, growth/removal, lighting, offline/recovery, empty/mobile and WebGL fallback. All mutations intercepted.');
  } catch (error) {
    console.error('Browser check failed before teardown:',error.message);
    for(const page of browser.pages())console.error('Movement diagnostic:',await page.evaluate(()=>({clock:window.oakGym?.getInfo().animationTime,positions:window.oakGym?.getInfo().positions,connection:document.getElementById('connection')?.textContent})).catch(()=>null));
    throw error;
  } finally {await browser.close();}
})().catch(error=>{console.error(error);process.exitCode=1;});
