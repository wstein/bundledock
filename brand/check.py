from pathlib import Path
import xml.etree.ElementTree as ET
import resvg_py, io
from PIL import Image
root=Path(__file__).resolve().parent.parent
for p in sorted((root/'assets').glob('*.svg')):
    node=ET.fromstring(p.read_text())
    assert not node.findall('.//{http://www.w3.org/2000/svg}text')
    assert node.find('{http://www.w3.org/2000/svg}title') is not None
    assert node.find('{http://www.w3.org/2000/svg}desc') is not None
    assert 'href=' not in p.read_text()
    print('XML, accessible name, outlined text, external-resource check:',p.name,'PASS')
sheet=Image.new('RGB',(320,112),'white')
for i,size in enumerate((16,24,32)):
    data=resvg_py.svg_to_bytes(svg_string=(root/'assets/favicon.svg').read_text(),width=size,height=size)
    im=Image.open(io.BytesIO(data)).convert('RGBA')
    sheet.paste(im.resize((96,96),Image.Resampling.NEAREST),(i*112,8),im.resize((96,96),Image.Resampling.NEAREST))
sheet.save(root/'brand/favicon-small-review.png')
def lum(hexcolor):
    vals=[int(hexcolor[i:i+2],16)/255 for i in (1,3,5)]
    return sum(w*(v/12.92 if v<=.04045 else ((v+.055)/1.055)**2.4) for v,w in zip(vals,(.2126,.7152,.0722)))
for fg in ('#ffffff','#5eead4','#2a4a7a'):
    for bg in ('#0b2545','#13315c'):
        a,b=sorted((lum(fg),lum(bg)))
        print('Contrast',fg,bg,round((b+.05)/(a+.05),2))
