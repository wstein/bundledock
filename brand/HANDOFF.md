# BundleDock brand asset handoff

Scope: seven self-contained assets in `assets/`, authored exclusively in this temporary lane. No Git repository, shared checkout edits, commits, history rewrites, external publication, or image generation were performed. Import these assets into the separately extracted BundleDock repository while preserving its original history.

Design: three stacked shipping packages over a dock platform. Navy #0b2545 rounded tile, decorative #2a4a7a outline, turquoise #5eead4 strokes at width 4. Banner/social use the family #0b2545–#13315c diagonal gradient; Bundle is white, Dock turquoise. Subtitle: “Build and ship OCI bundles.”

Dimensions: logo.svg 256×256 (64×64 viewBox); logo.png 256×256; favicon.svg same scalable tile; banner.svg/png 1000×150; social-preview.svg/png 1280×640. Banner and social preview match the actual reference SVG dimensions. Banner corner radius 18.

Typography: Bricolage Grotesque Bold (wght 700, opsz 72 banner / 96 social) and IBM Plex Sans Regular. Local font sources read from `/workspaces/Anchorage/workharbor/assets/fonts/bricolage-grotesque/BricolageGrotesque-Variable.ttf` and `/workspaces/Anchorage/workharbor/docs/static/fonts/ibm-plex-sans/IBMPlexSans-Regular.woff2`; both are SIL OFL 1.1 per their adjacent OFL.txt files. All delivered typography is vector outlines; no font files, text elements, network resources or external font references ship in the assets.

Production: `generate.py` is retained in this lane as provenance. Isolated temporary Python 3.13.5 environment uses fonttools 4.60.1, brotli 1.1.0, resvg-py 0.2.6, pillow 12.0.0. It reads local source fonts, outlines glyphs, emits SVG, renders PNG with resvg, and generates SHA256SUMS. The environment and generator are not required at asset runtime. No AI raster tool was used. Actual model/reasoning configuration is unverified in this leaf.

Verification: all four SVGs parse as XML, provide accessible title/description, contain no text elements or href resource references. All three PNG dimensions were measured with Pillow. Banner, social preview and final 256px logo were opened with view_image and visually inspected: balanced family lockup, readable typography, no clipping, complete mark. Favicon was rendered at 16/24/32px and viewed in `favicon-small-review.png`: stacked package silhouette and dock remain recognizable at 16px; tape detail is clearest from 24px. This visual inspection is author evidence, not independent review or clearance.

Measured WCAG luminance contrast: white/navy 15.39:1; white/gradient endpoint 12.95:1; turquoise/navy 10.40:1; turquoise/gradient endpoint 8.75:1. Decorative tile outline/navy is 1.73:1, matching the family reference; the outline conveys no content or control boundary. Keyboard, interactive states and touch targets are not applicable to these static image assets. Color is accompanied by the full wordmark and package/dock geometry. No behavioral changes were made; TDD is not applicable.

`SHA256SUMS` records the exact seven delivered files. `check.py` retains the nonbehavioral validation and small-size review-sheet generation. Import only the requested `assets/` content, with optional project-native brand documentation written by the receiving author.
