"""Source inventories keep authoritative links across regeneration."""
import importlib.util
import json
from pathlib import Path
import shutil
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("tool_inventory", ROOT / "scripts/tool-inventory.py")
INVENTORY = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(INVENTORY)


class InventoryLinksTests(unittest.TestCase):
    def test_toolbox_links_and_regeneration(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for relative in ("bundles/toolbox", "internal/toolbundle/python"):
                shutil.copytree(ROOT / relative, root / relative)
            (root / "docs").mkdir()
            shutil.copyfile(ROOT / "docs/toolbox.md", root / "docs/toolbox.md")
            doc, _, generated = INVENTORY.generated(root)
            self.assertIn("| Tool | Pinned version | Homepage | Source |", generated)
            table = generated.split(INVENTORY.BEGIN)[1].split(INVENTORY.END)[0]
            catalog = json.loads((root / "bundles/toolbox/packages.catalog.json").read_text())
            self.assertEqual(len([row for row in table.splitlines() if row.startswith("| ")]) - 2, len(catalog["packages"]) + 2)
            for package in catalog["packages"].values():
                if package["provider"] == "source-build":
                    # The publisher's download page is the source of a source build.
                    self.assertIn("[Source](" + package["version_source"]["url"] + ")", table)
                    continue
                self.assertIn("[Source](https://github.com/" + package["repository"] + ")", table)
            self.assertIn("[Source](https://github.com/tldr-pages/tldr-python-client)", table)
            self.assertIn("[Source](https://github.com/github/spec-kit)", table)
            doc.write_text(generated)
            _, actual, regenerated = INVENTORY.generated(root)
            self.assertEqual(actual, regenerated)
            self.assertEqual(actual.split(INVENTORY.BEGIN)[0], (ROOT / "docs/toolbox.md").read_text().split(INVENTORY.BEGIN)[0])
            # Changing authoritative catalog metadata must change the generated source link.
            catalog["packages"]["jq"]["repository"] = "example/reviewed-upstream"
            (root / "bundles/toolbox/packages.catalog.json").write_text(json.dumps(catalog))
            _, _, changed = INVENTORY.generated(root)
            self.assertIn("[Source](https://github.com/example/reviewed-upstream)", changed)
            self.assertNotEqual(changed, regenerated)

    def test_unsafe_catalog_repository_is_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for relative in ("bundles/toolbox", "internal/toolbundle/python"):
                shutil.copytree(ROOT / relative, root / relative)
            (root / "docs").mkdir()
            shutil.copyfile(ROOT / "docs/toolbox.md", root / "docs/toolbox.md")
            catalog_path = root / "bundles/toolbox/packages.catalog.json"
            catalog = json.loads(catalog_path.read_text())
            catalog["packages"]["jq"]["repository"] = "jqlang/jq) | injected"
            catalog_path.write_text(json.dumps(catalog))
            with self.assertRaises(ValueError):
                INVENTORY.generated(root)


class SourceBuildLinkTests(unittest.TestCase):
    def test_unsafe_source_build_url_is_rejected(self):
        package = {"provider": "source-build", "allowed_hosts": ["www.sqlite.org"],
                   "version_source": {"url": "https://www.sqlite.org/download.html"}}
        self.assertEqual(INVENTORY.source_url(package), "https://www.sqlite.org/download.html")
        for url in ("http://www.sqlite.org/download.html", "https://evil.example/download.html",
                    "https://www.sqlite.org/download.html) | injected", "https://www.sqlite.org/a?b=1",
                    "https://www.sqlite.org/a#frag", ""):
            with self.subTest(url=url):
                with self.assertRaises(ValueError):
                    INVENTORY.source_url(dict(package, version_source={"url": url}))
