import {agentState} from './state.js';

export function createGymEventTracker(){
  let agents=new Map(),sessions=new Map();
  return {observe(data){
    const events=[],nextAgents=new Map(),groups=new Map(),nextSessions=new Map();
    for(const agent of data){
      const previous=agents.get(agent.id),failure=agent.toolFailures,questions=new Set((agent.questions||[]).map(question=>question.id));
      const valid=failure&&typeof failure.generation==='string'&&Number.isSafeInteger(failure.count)&&failure.count>=0;
      if(previous){
        if(valid&&previous.failure?.generation===failure.generation&&failure.count>previous.failure.count)events.push({agentId:agent.id,type:'tool_failed'});
        if((agentState(agent)==='question'&&previous.state!=='question')||[...questions].some(id=>!previous.questions.has(id)))events.push({agentId:agent.id,type:'question_raised'});
      }
      nextAgents.set(agent.id,{state:agentState(agent),questions,failure:valid?{...failure}:previous?.failure});
      if(!groups.has(agent.sessionId))groups.set(agent.sessionId,[]);
      groups.get(agent.sessionId).push(agent);
    }
    for(const [id,members] of groups){
      const previous=sessions.get(id),signature=members.map(agent=>agent.id).sort().join('\n');
      const busy=members.some(agent=>agentState(agent)==='working'),ready=members.every(agent=>agentState(agent)==='ready');
      if(previous?.hadWork&&ready&&previous.signature===signature)for(const agent of members)events.push({agentId:agent.id,type:'session_ready'});
      nextSessions.set(id,{hadWork:!ready&&(busy||(previous?.signature===signature&&previous.hadWork)),signature});
    }
    agents=nextAgents;sessions=nextSessions;return events;
  }};
}
