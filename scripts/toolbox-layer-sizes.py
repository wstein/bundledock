#!/usr/bin/env python3
"""Download size of each toolbox tool and per layer group, without pulling an image.

Sizes come from the lock URLs: one ranged HTTP request per archive (size from
Content-Range), no payload download. A tar.gz or tar.xz archive is the
compressed size; a raw binary is its full size. These are download sizes, not
the installed or layer sizes (UNKNOWN until an image is built and inspected).
"""
import argparse
import json
from pathlib import Path
import sys
import urllib.request
from concurrent.futures import ThreadPoolExecutor

ROOT = Path(__file__).resolve().parent.parent
TOOLBOX = ROOT / "bundles" / "toolbox"


def group_totals(sizes, plan):
    """Return [(layer, tool count, bytes)] in plan order; unknown sizes raise."""
    tools = {t for layer in plan["layers"] for t in layer["tools"]}
    missing = sorted(set(sizes) ^ tools)
    if missing:
        raise ValueError("lock and plan differ: " + ", ".join(missing))
    return [(layer["name"], len(layer["tools"]), sum(sizes[t] for t in layer["tools"])) for layer in plan["layers"]]


def remote_size(url):
    request = urllib.request.Request(url, headers={"Range": "bytes=0-0", "User-Agent": "bundledock-layer-sizes"})
    with urllib.request.urlopen(request, timeout=60) as response:
        content_range = response.headers.get("Content-Range", "")
        total = content_range.rpartition("/")[2]
        return int(total) if total.isdigit() else int(response.headers["Content-Length"])


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--arch", default="amd64", choices=["amd64", "arm64"])
    parser.add_argument("--sizes", help="read '<tool> <bytes>' lines instead of querying the network")
    args = parser.parse_args(argv)
    plan = json.loads((TOOLBOX / "layers.json").read_text())
    pins = json.loads((TOOLBOX / "packages.lock.json").read_text())["platforms"][args.arch]
    if args.sizes:
        sizes = {l.split()[0]: int(l.split()[1]) for l in Path(args.sizes).read_text().splitlines() if l.strip()}
    else:
        with ThreadPoolExecutor(8) as pool:
            sizes = dict(zip(pins, pool.map(lambda p: remote_size(p["url"]), pins.values())))
    for name, count, total in group_totals(sizes, plan):
        print(f"{name}\t{count}\t{total / 1e6:.1f} MB")
    print(f"total\t{len(sizes)}\t{sum(sizes.values()) / 1e6:.1f} MB")
    return 0


if __name__ == "__main__":
    sys.exit(main())
