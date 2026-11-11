import {agentState} from './state.js';

export function recoveryOrder(data) {
  const byId=new Map(data.map(agent=>[agent.id,agent]));
  const paired=agent=>agentState(agent)==='ready'&&byId.has(agent.partnerId)&&agentState(byId.get(agent.partnerId))==='ready';
  return [...data].sort((a,b)=>Number(paired(b))-Number(paired(a))||a.id.localeCompare(b.id));
}

function beginMoment(a,b,type,time) {
  for(const [actor,partner] of [[a,b],[b,a]]) {
    const dx=partner.position.x-actor.position.x,dz=partner.position.z-actor.position.z;
    actor.partnerMoment={type,partnerId:partner.data.id,startedAt:time,duration:type==='fist-bump'?2.2:2.8,
      hand:dx>0?1:0,target:[dx/1.7,.43,dz/1.7+.28],giver:Boolean(actor.data.spotting)};
  }
}

export function updatePartners(actors,time) {
  for(const actor of actors.values()) {
    actor.company=null;
    const peer=actors.get(actor.data.partnerId);
    if(actor.state!=='ready'||!peer||peer.state!=='ready')actor.pendingBump=false;
    if(actor.state!=='working')actor.carryingPlate=false;
    const moment=actor.partnerMoment;
    if(!moment)continue;
    const partner=actors.get(moment.partnerId);
    const valid=partner&&!actor.event&&!partner.event&&!actor.path.length&&!partner.path.length&&
      (moment.type==='fist-bump'?actor.state==='ready'&&partner.state==='ready':actor.state==='working'&&partner.state==='working'&&
        (moment.giver?actor.data.spotting===partner.data.id:partner.data.spotting===actor.data.id));
    if(!valid||time>=moment.startedAt+moment.duration) {
      if(valid&&moment.type==='plate-handoff'&&!moment.giver){actor.carryingPlate=true;actor.carryHand=moment.hand;}
      actor.partnerMoment=null;
    }
  }
  for(const actor of actors.values()) {
    const partner=actors.get(actor.data.partnerId);
    if(!partner||actor.data.id>partner.data.id||actor.path.length||partner.path.length||actor.event||partner.event)continue;
    const distance=Math.hypot(actor.position.x-partner.position.x,actor.position.z-partner.position.z);
    if(distance>1.7)continue;
    if(actor.state==='ready'&&partner.state==='ready') {
      actor.company={side:Math.sign(partner.position.x-actor.position.x),clock:time+Math.min(actor.identity,partner.identity)%31};
      partner.company={side:-actor.company.side,clock:actor.company.clock};
      if((actor.pendingBump||partner.pendingBump)&&!actor.partnerMoment&&!partner.partnerMoment) {
        actor.pendingBump=partner.pendingBump=false;beginMoment(actor,partner,'fist-bump',time);
      }
      continue;
    }
    const lifter=actor.data.spotting?partner:actor,spotter=lifter===actor?partner:actor;
    if(spotter.data.spotting!==lifter.data.id||lifter.state!=='working'||lifter.station.kind!=='floor'||lifter.partnerMoment||spotter.partnerMoment)continue;
    if(lifter.handoffAt===lifter.arrivedAt||time-lifter.arrivedAt<.4||lifter.carryingPlate)continue;
    lifter.handoffAt=lifter.arrivedAt;beginMoment(lifter,spotter,'plate-handoff',time);
  }
}
