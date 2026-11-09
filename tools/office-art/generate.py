#!/usr/bin/env python3
"""Generate original layered pixel artwork for the browser office (Pillow only).

Export application assets or render demonstration frames; session state is untouched.
"""

import argparse
import json
import math
from pathlib import Path
import random

from PIL import Image, ImageDraw, ImageFilter, ImageFont

SIZE = (864, 448)
ORIGIN = (394, 100)
ROOMS = [
    {"id": "work", "name": "Work room", "bounds": [0, 9, 11, 18]},
    {"id": "meeting", "name": "Meeting room", "bounds": [0, 0, 11, 9]},
    {"id": "break", "name": "Break room", "bounds": [11, 0, 22, 9]},
]
INK = "#342b30"
BG = "#171d27"


def render(frame=0, frames=32, export_dir=None):
    image = Image.new("RGB", SIZE, BG)
    draw = ImageDraw.Draw(image)
    rng = random.Random(19)

    def point(u, v, z=0):
        return (round(ORIGIN[0] + (u - v) * 20), round(ORIGIN[1] + (u + v) * 10 - z))

    def polygon(points, fill, outline=None):
        draw.polygon(points, fill=fill)
        if outline:
            draw.line(points + [points[0]], fill=outline, width=1)

    def floor_poly(u, v, w, d, fill, outline=None, z=0):
        polygon([point(u, v, z), point(u+w, v, z), point(u+w, v+d, z), point(u, v+d, z)], fill, outline)

    def box(u, v, w, d, h, top, left, right, z=0, outline=INK):
        a, b, c, e = [point(x, y, z) for x, y in [(u,v), (u+w,v), (u+w,v+d), (u,v+d)]]
        at, bt, ct, et = [point(x, y, z+h) for x, y in [(u,v), (u+w,v), (u+w,v+d), (u,v+d)]]
        polygon([e,c,ct,et], left, outline)
        polygon([b,c,ct,bt], right, outline)
        polygon([at,bt,ct,et], top, outline)

    def panel(u, v, width, bottom, height, axis, fill, outline=None):
        du, dv = (width, 0) if axis == "u" else (0, width)
        polygon([point(u,v,bottom),point(u+du,v+dv,bottom),point(u+du,v+dv,bottom+height),point(u,v,bottom+height)], fill, outline)

    def shadow(u,v,rx=12,ry=5):
        x,y=point(u,v)
        draw.ellipse((x-rx,y-ry,x+rx,y+ry),fill="#765844")

    # A thick, finished diorama base rather than a floor floating on transparency.
    floor_palettes = [
        ["#cba776", "#dfbc87", "#b99164", "#d5b17c"],
        ["#b6a1a0", "#c8b6ad", "#a78e8c", "#bca7a0"],
        ["#aeb28a", "#bec19b", "#979f77", "#b0b58a"],
    ]
    for room, colors in zip(ROOMS, floor_palettes):
        u0, v0, u1, v1 = room["bounds"]
        box(u0,v0,u1-u0,v1-v0,7,colors[0],"#735442","#4b3d37",z=-7)
        for u in range(u0, u1):
            for v in range(v0, v1):
                color=colors[(u+v)%2 + (2 if (u*7+v*3)%13==0 else 0)]
                floor_poly(u,v,1,1,color,"#99866a")
                if rng.random()<0.35:
                    x,y=point(u+.25,v+.4)
                    draw.line((x,y,x+3,y+1),fill=colors[1])
    # Back walls, dark structural caps, timber wainscot and warm plaster.
    panel(0,0,18,0,66,"v","#cba46f",INK)
    panel(0,0,22,0,66,"u","#ae8056",INK)
    panel(0,0,18,0,14,"v","#84613e",INK)
    panel(0,0,22,0,14,"u","#775037",INK)
    for v in range(18):
        draw.line([point(0,v,1),point(0,v,13)],fill="#a07b4b")
    for u in range(22):
        draw.line([point(u,0,1),point(u,0,13)],fill="#936340")
    for axis,length in [("u",22),("v",18)]:
        panel(0,0,length,64,4,axis,"#514237",INK)
        panel(0,0,length,13,2,axis,"#e0b77a")
        panel(0,0,length,0,3,axis,"#503d31")

    def window(v):
        panel(.03,v,1.75,22,36,"v","#5d422f",INK)
        panel(.04,v+.12,1.51,25,30,"v","#fbe0a0","#b18147")
        for z in range(28,55,4):
            draw.line([point(.05,v+.14,z),point(.05,v+1.62,z)],fill="#e6bc77")
        draw.line([point(.06,v+.88,25),point(.06,v+.88,55)],fill="#a97743",width=2)
        box(.03,v-.04,.20,1.85,2,"#d2a36d","#98653f","#b27f49",z=21)
    for v in [1.05, 5.5, 10.05, 14.5]:
        window(v)

    # Sunlit parallelograms across the floor, composited rather than opaque paint.
    light=Image.new("RGBA",SIZE)
    ld=ImageDraw.Draw(light)
    for v in [1.2,5.65,10.2,14.65]:
        ld.polygon([point(.15,v),point(.15,v+1.4),point(5.0,v+.75),point(5.0,v-.65)],fill=(255,221,145,48))
    image=Image.alpha_composite(image.convert("RGBA"),light).convert("RGB")
    draw=ImageDraw.Draw(image)

    # Framed team board on the right wall.
    panel(16.1,.04,2.2,30,25,"u","#6e4b33",INK)
    panel(16.22,.07,1.96,32,21,"u","#e1d0a5","#b79769")
    for i in range(4):
        u=16.38+i*.4
        panel(u,.10,.24,35,5+i*2,"u",["#728c77","#ae8253","#6d8591","#ad6d54"][i])
    draw.line([point(16.35,.11,49),point(17.9,.11,49)],fill="#9b855f")
    x,y=point(4.35,0,48)
    draw.ellipse((x-6,y-7,x+6,y+7),fill="#5c4333",outline=INK)
    draw.ellipse((x-4,y-5,x+4,y+5),fill="#e5cd9c")
    draw.line((x,y-3,x,y,x+3,y+1),fill="#715b43")

    def shelf(u,v,axis):
        along=1.75
        w,d=(along,.38) if axis=="u" else (.38,along)
        box(u,v,w,d,48,"#b7804d","#805232","#65422e")
        fu,fv=(u,v+d+.015) if axis=="u" else (u+w+.015,v)
        panel(fu,fv,along,5,39,axis,"#443c30",INK)
        palette=["#597c70","#ba704b","#d0ae65","#6c8394","#936361","#c3b58a"]
        for row,z in enumerate([8,21,34]):
            for i in range(8):
                width=.13+rng.random()*.045
                du,dv=((.08+i*.2),0) if axis=="u" else (0,(.08+i*.2))
                height=rng.randint(7,10)
                panel(fu+du,fv+dv,width,z,height,axis,palette[(i+row*3)%len(palette)],"#4a3d30")
                panel(fu+du,fv+dv,width,z+height-2,1,axis,"#dac18b")
            panel(fu,fv,along,z-1,2,axis,"#b37c48",INK)
        panel(fu,fv,along,2,4,axis,"#8e5c35",INK)

    shelf(.14,3.15,"v")
    shelf(.14,12.15,"v")
    shelf(.14,16.0,"v")
    shelf(1.0,.12,"u")
    shelf(8.1,.12,"u")
    shelf(19.6,.12,"u")
    floor_poly(14.1,3.4,5.1,3.6,"#7c8f77","#627363")
    floor_poly(14.3,3.6,4.7,3.2,"#a5ad8a","#c5c49b")
    floor_poly(2.2,2.6,6.7,4.5,"#827884","#6c6370")
    floor_poly(2.4,2.8,6.3,4.1,"#a4979b","#c2ada8")

    def plant(u,v):
        shadow(u,v,8,4)
        box(u-.22,v-.22,.44,.44,9,"#b67c4c","#985d3e","#734734")
        x,y=point(u,v,10)
        draw.ellipse((x-6,y-3,x+6,y+2),fill="#be9059",outline="#5c4931")
        draw.ellipse((x-4,y-2,x+4,y+1),fill="#574530")
        draw.line((x,y,x,y-22),fill="#665a36",width=2)
        for dx,dy,r in [(-5,-15,5),(4,-20,5),(-4,-24,4),(5,-10,4),(0,-28,3)]:
            draw.ellipse((x+dx-r,y+dy-r//2,x+dx+r,y+dy+r),fill="#506749",outline="#364a38")
            draw.line((x,y-8,x+dx,y+dy),fill="#718050")
            draw.point((x+dx-1,y+dy-1),fill="#a1a267")

    def monitor(u,v):
        box(u+.14,v+.07,.38,.32,1,"#c1b48d","#8b856d","#716d5c",z=20)
        box(u+.24,v+.09,.14,.12,4,"#cec19b","#a99c7e","#80765f",z=21)
        box(u,v,.74,.24,10,"#d6c8a4","#a69b7e","#817860",z=24)
        panel(u+.08,v+.251,.58,26,6,"u","#2e4b49","#4c6055")
        for row in range(3):
            draw.line([point(u+.14,v+.26,27+row*1.7),point(u+.34+(row%2)*.2,v+.26,27+row*1.7)],fill="#8cae8d")
        if frame%16<8:
            draw.point(point(u+.52,v+.27,27),fill="#d6d4a3")
        box(u-.05,v+.55,.78,.30,1,"#bdb395","#948971","#7a735f",z=19.5)
        for row in range(3):
            draw.line([point(u+.02,v+.59+row*.07,21),point(u+.65,v+.59+row*.07,21)],fill="#8a826e")

    def lamp(u,v):
        x,y=point(u,v,23)
        glow=Image.new("RGBA",SIZE)
        ImageDraw.Draw(glow).ellipse((x-14,y-8,x+14,y+8),fill=(255,198,90,65))
        glow=glow.filter(ImageFilter.GaussianBlur(6))
        image.paste(glow,(0,0),glow)
        box(u-.1,v-.1,.23,.23,1,"#d4ad66","#ad8149","#8d663c",z=20)
        x,y=point(u,v,21)
        draw.line((x,y,x,y-12),fill="#5d4a32",width=2)
        polygon([(x-6,y-10),(x-3,y-17),(x+3,y-17),(x+6,y-10)],"#e9bc67","#936a3c")
        draw.line((x-4,y-11,x+4,y-11),fill="#ffe6a0")
        draw.point((x-1,y-15),fill="#f8d98a")

    def desk(u,v):
        shadow(u+1.35,v+.8,37,12)
        for du,dv in [(.1,.1),(2.5,.1),(.1,1.0),(2.5,1.0)]:
            box(u+du,v+dv,.16,.18,18,"#b98956","#855337","#64412e")
        box(u+.10,v+.35,.65,.75,15,"#b98956","#9d663c","#784b30")
        for z in [5,10]:
            panel(u+.1,v+1.11,.65,z,.6,"u","#5d412d")
            draw.line([point(u+.30,v+1.12,z+2),point(u+.52,v+1.12,z+2)],fill="#d3ad65")
        box(u,v,2.8,1.25,2,"#c99659","#a96c3e","#895432",z=18)
        draw.line([point(u+.1,v+.06,20),point(u+2.7,v+.06,20)],fill="#e0b476")
        monitor(u+.25,v+.12)
        monitor(u+1.72,v+.12)
        lamp(u+1.37,v+.3)
        # Open notebook, pencil and a ceramic mug.
        floor_poly(u+1.08,v+.71,.5,.35,"#e2d2a8","#8e795b",z=20.1)
        draw.line([point(u+1.31,v+.72,20.5),point(u+1.31,v+1.04,20.5)],fill="#aa9470")
        x,y=point(u+2.52,v+.89,21)
        draw.rectangle((x-2,y-3,x+1,y+1),fill="#648780",outline="#3e5b56")
        draw.arc((x,y-2,x+4,y+1),270,90,fill="#78948a")

    # Original hand-pixelled characters, with palette variations and two foot poses.
    person=[
        "     HHHHH     ",
        "    HhhhhHH    ",
        "   HhhhhhhHH   ",
        "   HhssssshH   ",
        "   HsssssssH   ",
        "    sseesse    ",
        "    ssssss     ",
        "     sSSs      ",
        "    cCssCc     ",
        "   cCCCCCCc    ",
        "  scCCCCCCcs   ",
        "  sccCCCCccs   ",
        "  ssCCCCCCss   ",
        "   sCCCCCCs    ",
        "    CCCCCc     ",
        "    PPPPPP     ",
        "    PPPpPP     ",
        "    PP  PP     ",
        "    Pp  Pp     ",
        "    PP  PP     ",
        "   BBB  BBB    ",
    ]

    def character(u,v,shirt,shade,hair="#544133",seated=False,back=False,walking=False):
        x,y=point(u,v)
        if not seated:
            shadow(u,v,7,3)
        sprite=Image.new("RGBA",(15,23))
        sd=ImageDraw.Draw(sprite)
        palette={"H":"#302a2d","h":hair,"s":"#d9a476","S":"#b87956","e":"#3d3635","C":shirt,"c":shade,"P":"#435568","p":"#344354","B":"#30323b"}
        for row,line in enumerate(person):
            for col,ch in enumerate(line):
                if ch not in palette:
                    continue
                color=palette[ch]
                if back and row<7 and ch in "sSe":
                    color=hair if row<6 else "#bd865f"
                dy=1 if walking and row>16 and ((col<7)==(frame//3%2==0)) else 0
                sd.point((col,row+dy),fill=color)
        if seated:
            sprite=sprite.crop((0,0,15,18))
            y-=7
        bob=1 if walking and frame//3%2 else 0
        image.paste(sprite,(x-7,y-sprite.height-bob),sprite)

    def chair(u,v,shirt,shade,hair="#544133",back=True,with_person=True):
        shadow(u,v,9,4)
        x,y=point(u,v)
        for dx,dy in [(-7,0),(7,0),(-3,4),(4,3)]:
            draw.line((x,y-3,x+dx,y+dy),fill="#394143",width=1)
            draw.rectangle((x+dx-1,y+dy,x+dx+1,y+dy+1),fill="#2e353b")
        box(u-.25,v-.2,.55,.55,3,"#567477","#3d565d","#30444e",z=6)
        if with_person:
            character(u,v,shirt,shade,hair,seated=True,back=back)
        if back:
            box(u-.27,v+.24,.58,.12,9,"#668484","#435d63","#34484f",z=8)
            draw.line([point(u-.18,v+.37,14),point(u+.22,v+.37,14)],fill="#75918d")

    def sofa(u,v):
        shadow(u+1,v+.3,25,8)
        box(u,v,1.9,.7,9,"#718c7d","#4f6d65","#3c5553")
        box(u,v,1.9,.16,11,"#8fa58b","#5e7c70","#47635c",z=7)
        for du in [.2,1.0]:
            box(u+du,v+.18,.70,.5,2,"#8b9e83","#688072","#50685e",z=9)
        for du in [0,1.7]:
            box(u+du,v,.2,.75,4,"#a3af91","#728c79","#536f62",z=9)

    def coffee(u,v):
        box(u,v,1.45,.7,19,"#c39763","#986b44","#745034")
        box(u+.2,v+.12,.65,.4,11,"#606d67","#424d4c","#303c40",z=19)
        panel(u+.28,v+.53,.45,24,4,"u","#242f34")
        draw.point(point(u+.58,v+.54,28),fill="#e9b361")
        x,y=point(u+1.08,v+.4,20)
        draw.rectangle((x-2,y-4,x+2,y),fill="#d8c9a2",outline="#8e836d")

    background=image.copy()
    objects=[]
    obstacles = [
        {"u":.14,"v":v,"w":.38,"d":1.75} for v in [3.15,12.15,16.0]
    ] + [{"u":u,"v":.12,"w":1.75,"d":.38} for u in [1.0,8.1,19.6]]
    seats=[]
    def add(name,depth,fn,obstacle=None):
        objects.append((depth,name,fn))
        if obstacle:
            obstacles.append(dict(zip(["u","v","w","d"],obstacle)))
    add("coffee",15.4,lambda: coffee(14.0,.35),(14.0,.35,1.45,.7))
    add("plant-back",11.2,lambda: plant(10.1,.9),(9.8,.6,.6,.6))
    add("plant-left",18.0,lambda: plant(.75,17.1),(.45,16.8,.6,.6))
    add("plant-front",25.4,lambda: plant(10.0,15.3),(9.7,15.0,.6,.6))
    add("plant-meeting",28.4,lambda: plant(21.0,7.3),(20.7,7.0,.6,.6))
    add("sofa",18.6,lambda: sofa(14.4,3.2),(14.4,3.2,1.9,.75))
    add("sofa-two",21.7,lambda: sofa(17.6,3.2),(17.6,3.2,1.9,.75))
    add("tea-table",22.0,lambda: box(16.0,5.0,1.8,.9,9,"#be925e","#936842","#6a4937"),(16.0,5.0,1.8,.9))
    # Cutaway partitions leave two full-width doorways in the walkable floor.
    partitions = [(0,8.9,4.6,.2), (6.4,8.9,4.6,.2), (10.9,0,.2,4.4), (10.9,6.2,.2,2.7)]
    for index,(u,v,w,d) in enumerate(partitions):
        length = max(w,d)
        segments = math.ceil(length / .5)
        obstacles.append({"u":u,"v":v,"w":w,"d":d})
        for segment in range(segments):
            su = u + (w * segment / segments if w > d else 0)
            sv = v + (d * segment / segments if d > w else 0)
            sw = w / segments if w > d else w
            sd = d / segments if d > w else d
            add(f"partition-{index}-{segment}",su+sv+(sw+sd)/2,
                lambda u=su,v=sv,w=sw,d=sd: box(u,v,w,d,14,"#d7b681","#a67c57","#7f604b"))
    teams=[(1.8,11.3,[("#658f89","#416c6b","#544133"),("#b17c55","#885941","#593f30")]),(6.0,12.0,[("#8c79a1","#655675","#5b3c30"),("#6986ad","#4a6285","#b48b51")]),(4.8,15.2,[("#b28e51","#876b3f","#4b3730"),("#8e9a67","#647248","#413a37")])]
    for index,(u,v,palettes) in enumerate(teams):
        add(f"desk-{index}",u+v+1.3,lambda u=u,v=v:desk(u,v),(u,v,2.8,1.25))
        for chair_index,(offset,palette) in enumerate(zip([.65,2.13],palettes)):
            seats.append({"u":u+offset,"v":v+1.65})
            add(f"chair-{index}-{chair_index}",u+offset+v+1.7,lambda u=u,v=v,offset=offset,palette=palette:chair(u+offset,v+1.65,*palette,with_person=not bool(export_dir)))
    meeting_seats = []
    obstacles.append({"u":3.0,"v":3.4,"w":4.5,"d":1.65})
    for index,(u,v) in enumerate([(3.15,3.55),(7.15,3.55),(3.15,4.8),(7.15,4.8)]):
        add(f"meeting-leg-{index}",u+v,lambda u=u,v=v: box(u,v,.18,.18,17,"#be925e","#936842","#6a4937"))
    # Narrow tabletop strips sort correctly against chairs on both long sides.
    for index in range(6):
        u = 3.0 + index * .75
        add(f"meeting-top-{index}",u+5.05,lambda u=u: box(u,3.4,.75,1.65,2,"#bf9669","#936648","#70503d",z=17))
    for index,u in enumerate([3.7,5.25,6.8]):
        for back,v in [(False,2.85),(True,5.55)]:
            meeting_seats.append({"u":u,"v":v,"facing":"back" if back else "front","seated":True})
            add(f"meeting-chair-{index}-{back}",u+v+.1,
                lambda u=u,v=v,back=back: chair(u,v,"#8c79a1","#655675",back=back,with_person=False))
    if export_dir:
        export_dir.mkdir(parents=True,exist_ok=True)
        background.save(export_dir/"room.png")
        walls = [[0, 0, 0, 18], [0, 0, 22, 0]] + [
            [u, v + d/2, u + w, v + d/2] if w > d else [u + w/2, v, u + w/2, v + d]
            for u, v, w, d in partitions
        ]
        manifest={"width":SIZE[0],"height":SIZE[1],"origin":list(ORIGIN),"tile":[20,10],"bounds":[.5,.5,21.5,17.5],"background":"room.png","furniture":[],"obstacles":obstacles,"walls":walls,"rooms":[],"sprites":[]}
        spots = {
            "work": [{**seat,"seated":True,"facing":"back"} for seat in seats],
            "break": [{"u":u+11,"v":v} for u,v in [(4,4.6),(7.5,4.6),(3.2,6.5),(7.5,6.5),(2.2,2.1),(6.4,1.8),(8.7,7.3),(1.5,7.0)]],
            "meeting": meeting_seats,
        }
        for room in ROOMS:
            u0,v0,u1,v1 = room["bounds"]
            left = point(u0,v1)[0]-16
            top = point(u0,v0)[1]-76
            right = point(u1,v0)[0]+16
            bottom = point(u1,v1)[1]+18
            manifest["rooms"].append({**room,"spots":spots[room["id"]],"view":[left,top,right-left,bottom-top]})
        for depth,name,fn in objects:
            image=Image.new("RGBA",SIZE)
            draw=ImageDraw.Draw(image)
            fn()
            bounds=image.getbbox()
            if bounds:
                image.crop(bounds).save(export_dir/f"{name}.png")
                manifest["furniture"].append({"src":f"{name}.png","x":bounds[0],"y":bounds[1],"depth":depth})
        for index,palette in enumerate(palette for _,_,palettes in teams for palette in palettes):
            variants={}
            for back in [False,True]:
                facing="back" if back else "front"
                for pose,walking,seated,step in [("idle",False,False,0),("walk0",True,False,0),("walk1",True,False,3),("seated",False,True,0)]:
                    frame=step
                    image=Image.new("RGBA",SIZE)
                    draw=ImageDraw.Draw(image)
                    character(0,0,*palette,back=back,walking=walking,seated=seated)
                    bounds=image.getbbox()
                    name=f"person-{index}-{facing}-{pose}.png"
                    image.crop(bounds).save(export_dir/name)
                    variants[f"{facing}-{pose}"]={"src":name,"anchorX":ORIGIN[0]-bounds[0],"anchorY":ORIGIN[1]-bounds[1]}
            manifest["sprites"].append(variants)
        (export_dir/"scene.json").write_text(json.dumps(manifest,indent=2)+"\n")
        return background
    phase=frame/max(1,frames)*math.tau
    walker_u=3.0+1.0*(1-math.cos(phase))
    walker_v=5.4+.12*math.sin(phase)
    add("walker",walker_u+walker_v,lambda:character(walker_u,walker_v,"#bb7960","#925542",hair="#735340",walking=True))
    for _,_,fn in sorted(objects,key=lambda entry:entry[0]):
        fn()

    # Tiny highlights keep the diorama lively without visual clutter.
    for x,y in [(118,108),(151,83),(177,121),(93,161),(126,145)]:
        if (x+y+frame//8)%3:
            draw.point((x,y),fill="#eed6a0")

    font=ImageFont.load_default(size=9)
    small=ImageFont.load_default(size=8)
    def bubble(u,v,text,z=40):
        x,y=point(u,v,z)
        bounds=draw.textbbox((0,0),text,font=font)
        width=bounds[2]-bounds[0]+12
        height=17
        x=max(4,min(SIZE[0]-width-4,x-width//2))
        draw.rectangle((x+2,y+2,x+width+2,y+height+2),fill="#584c40")
        draw.rectangle((x,y,x+width,y+height),fill="#f1e3c6",outline="#5b5146")
        polygon([(x+width//2-3,y+height),(x+width//2,y+height+4),(x+width//2+3,y+height)],"#f1e3c6")
        draw.text((x+6,y+3),text,font=font,fill="#433e3a")
    bubble(walker_u,walker_v,"Drawing the office",z=51)
    bubble(8.1,4.65,"Checking graphics",z=48)
    # Clearly a visual study, not fabricated live telemetry.
    draw.text((16,10),"OAK TREE",font=font,fill="#e5c898")
    draw.text((76,11),"/  THE STUDIO",font=small,fill="#8f9b9e")
    draw.text((374,11),"PIXEL-ART STUDY",font=small,fill="#8f9b9e")
    draw.line((16,25,496,25),fill="#343e48")
    return image.resize((SIZE[0]*2,SIZE[1]*2),Image.Resampling.NEAREST)


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    destination=parser.add_mutually_exclusive_group(required=True)
    destination.add_argument("--out",type=Path,help="Render complete prototype frames")
    destination.add_argument("--export",type=Path,help="Export layered browser assets and character sprites")
    parser.add_argument("--frames",type=int,default=32)
    args=parser.parse_args()
    if args.export:
        render(export_dir=args.export)
        print(args.export/"scene.json")
        return
    if args.frames<1 or args.frames>120:
        parser.error("frames must be between 1 and 120")
    args.out.mkdir(parents=True,exist_ok=True)
    for frame in range(args.frames):
        render(frame,args.frames).save(args.out/f"office-{frame:03d}.png")
    print(args.out/f"office-{0:03d}.png")


if __name__=="__main__":
    main()
