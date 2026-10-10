# BundleDock brand sources

The seven approved SVG/PNG assets in `../assets/` use a navy shipping-package
and dock mark, turquoise accents, and outlined Bricolage Grotesque / IBM Plex
Sans lettering. SVGs are self-contained and include accessible names. The
fonts below are generation inputs; SVG/PNG runtime use requires no fonts.
Both fonts are redistributed under their adjacent SIL Open Font License 1.1.

`HANDOFF.md` preserves the original author evidence and design measurements.
`SHA256SUMS` preserves the seven approved asset hashes exactly; from the
repository root, run `sha256sum -c brand/SHA256SUMS`. No new visual design
iteration was performed during extraction.

The original `generate.py` and `check.py` were imported with only portable
repository-relative paths substituted. `generate.py` outlines the locally
stored fonts and renders PNG with resvg. `check.py` verifies SVG names and
resource independence, measures contrast, and writes an ignored small favicon
review sheet. Python 3.13.5 was used for the original generation; dependencies
are pinned in `requirements.txt`. To regenerate from this repository:

```sh
python3 -m venv /tmp/bundledock-brand-venv
/tmp/bundledock-brand-venv/bin/pip install -r brand/requirements.txt
/tmp/bundledock-brand-venv/bin/python brand/generate.py
/tmp/bundledock-brand-venv/bin/python brand/check.py
sha256sum -c brand/SHA256SUMS
```

Keep environments and caches outside the repository and read-only tool bundle.

Font sources were copied from `https://github.com/wstein/workharbor` at
commit `eb94ac273eb0b28549b5bd492bdb571efd8dc473`. SHA256 values for those source bytes:

- `assets/fonts/bricolage-grotesque/BricolageGrotesque-Variable.ttf`: `413e7357809ddd12fd80a96a8a396de0e401638d4acd3cb3e37532f0472ac682`
- `assets/fonts/bricolage-grotesque/OFL.txt`: `46ba5f18ee20ea529f21d96c0ef8637a8314c1a5cfb2aa84018bc8157cbeff41`
- `docs/static/fonts/ibm-plex-sans/IBMPlexSans-Regular.woff2`: `ba711a3085ff9f27440b6b9c4550cfc47c97bf36591d5da958b975bb3add8c1a`
- `docs/static/fonts/ibm-plex-sans/OFL.txt`: `d741e57d5f865e294df801f96b7b5161a88b211df65887e4358d271c9fc5fb4f`
