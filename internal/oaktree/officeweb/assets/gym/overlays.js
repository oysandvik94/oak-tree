const clamp=(value,min,max)=>Math.max(min,Math.min(max,value));

export function placeOverlays(items,{width,height,occupied=[],direction=1}) {
  const placed=[];
  for(const item of items) {
    const w=Math.min(item.width,Math.max(1,width-16)),h=item.height;
    const x=clamp(item.x,w/2+8,width-w/2-8),start=clamp(item.y,8,height-h-8);
    const obstacles=[...occupied,...placed];
    let best,score=Infinity;
    for(let row=0;row<=12;row++) {
      for(const sign of row?[direction,-direction]:[1]) {
        const y=clamp(start+row*(h+4)*sign,8,height-h-8);
        let overlap=0;
        for(const other of obstacles) {
          const horizontal=Math.max(0,(w+other.width)/2+4-Math.abs(x-other.x));
          const vertical=Math.max(0,Math.min(y+h,other.y+other.height)+4-Math.max(y,other.y));
          overlap+=horizontal*vertical;
        }
        if(overlap<score){score=overlap;best={...item,x,y,width:w,height:h};}
        if(!overlap)break;
      }
      if(!score)break;
    }
    placed.push(best);
  }
  return placed;
}

export function positionOverlay(node,box,anchor) {
  node.style.left=`${box.x}px`;node.style.top=`${box.y}px`;
  const stem=node.lastElementChild;
  const origin=anchor.y>box.y+box.height?box.height:0;
  const dx=anchor.x-box.x,dy=anchor.y-box.y-origin;
  const inside=Math.abs(dx)<=box.width/2&&anchor.y>=box.y&&anchor.y<=box.y+box.height;
  stem.style.top=`${origin}px`;
  stem.style.height=`${inside?0:Math.hypot(dx,dy)}px`;
  stem.style.transform=`translateX(-50%) rotate(${Math.atan2(-dx,dy)}rad)`;
}
