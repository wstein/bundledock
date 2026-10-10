from pathlib import Path
from fontTools.ttLib import TTFont
from fontTools.varLib.instancer import instantiateVariableFont
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
import resvg_py
from PIL import Image
import hashlib, json
ROOT=Path(__file__).resolve().parent.parent
OUT=ROOT/'assets'
REF=ROOT/'brand/fonts'
word=instantiateVariableFont(TTFont(REF/'bricolage-grotesque/BricolageGrotesque-Variable.ttf'),{'wght':700,'opsz':72},inplace=True)
tag=TTFont(REF/'ibm-plex-sans/IBMPlexSans-Regular.woff2')
NAVY='#0b2545'; TEAL='#5eead4'; WHITE='#ffffff'

def textpath(text,font,size,x,y,fill):
    glyphs=font.getGlyphSet(); cmap=font.getBestCmap(); scale=size/font['head'].unitsPerEm
    pen=SVGPathPen(glyphs)
    for char in text:
        name=cmap[ord(char)]
        glyphs[name].draw(TransformPen(pen,(scale,0,0,-scale,x,y)))
        x+=font['hmtx'][name][0]*scale
    return f'<path fill="{fill}" d="{pen.getCommands()}"/>', x

def textwidth(text,font,size):
    return sum(font['hmtx'][font.getBestCmap()[ord(c)]][0] for c in text)*size/font['head'].unitsPerEm

def icon():
    return '''<rect x="0.8" y="0.8" width="62.4" height="62.4" rx="13.28" fill="#0b2545" stroke="#2a4a7a" stroke-width="1.6"/>
<g fill="none" stroke="#5eead4" stroke-width="4" stroke-linecap="round" stroke-linejoin="round">
<path d="M24 11H40V25H24Z M32 11V17"/>
<path d="M16 25H48V40H16Z M32 25V40 M24 25V31 M40 25V31"/>
<path d="M11 47H53 M16 47V54 M48 47V54"/>
</g>'''

def svg(width,height,body,desc,view=None):
    return f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {view or str(width)+' '+str(height)}" width="{width}" height="{height}" role="img" aria-labelledby="title description">
<title id="title">BundleDock</title>
<desc id="description">{desc}</desc>
{body}
</svg>\n'''

def background(w,h,rounding=0):
    return f'''<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#0b2545"/><stop offset="1" stop-color="#13315c"/></linearGradient></defs>
<rect width="{w}" height="{h}" rx="{rounding}" fill="url(#g)"/>'''

OUT.joinpath('logo.svg').write_text(svg(256,256,icon(),'Three stacked shipping packages above a dock platform, in turquoise on a rounded navy tile.','64 64'))
OUT.joinpath('favicon.svg').write_text(OUT.joinpath('logo.svg').read_text())
for name,w,h,size,baseline,tag_size,tag_baseline,tile_x,tile_y,tile_scale,text_x in [
    ('banner',1000,150,68,80,30,118,56,27,1.5,184),
    ('social-preview',1280,640,100,318,36,388,0,184,4.5,0)]:
    if name=='social-preview':
        word=instantiateVariableFont(TTFont(REF/'bricolage-grotesque/BricolageGrotesque-Variable.ttf'),{'wght':700,'opsz':96},inplace=True)
        blockwidth=288+54+max(textwidth('BundleDock',word,size),textwidth('Build and ship OCI bundles.',tag,tag_size))
        tile_x=(w-blockwidth)/2
        text_x=tile_x+288+54
    p1,after=textpath('Bundle',word,size,text_x,baseline,WHITE)
    p2,_=textpath('Dock',word,size,after,baseline,TEAL)
    p3,_=textpath('Build and ship OCI bundles.',tag,tag_size,text_x+3.5,tag_baseline,TEAL)
    body=background(w,h,18 if name=='banner' else 0)+f'\n<g transform="translate({tile_x} {tile_y}) scale({tile_scale})">{icon()}</g>\n'+p1+'\n'+p2+'\n'+p3
    OUT.joinpath(name+'.svg').write_text(svg(w,h,body,'BundleDock. Build and ship OCI bundles. Stacked shipping packages above a dock platform.'))
for name,size in [('logo',256),('banner',None),('social-preview',None)]:
    opts={'svg_string':OUT.joinpath(name+'.svg').read_text()}
    if size: opts.update(width=size,height=size)
    OUT.joinpath(name+'.png').write_bytes(resvg_py.svg_to_bytes(**opts))
for p in sorted(OUT.iterdir()):
    if p.suffix=='.png':
        with Image.open(p) as im: print(p.name,im.size,im.mode)
    else: print(p.name,p.stat().st_size)
manifest={str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(OUT.iterdir())}
ROOT.joinpath('brand/SHA256SUMS').write_text(''.join(f'{v}  {k}\n' for k,v in manifest.items()))
