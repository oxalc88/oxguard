"""Exercise release documentation generation and pre-publication rejection."""
import importlib.util
from pathlib import Path
import tempfile
import unittest


def load(name):
    spec = importlib.util.spec_from_file_location(name.replace("-", "_"), Path(__file__).with_name(name + ".py"))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


package = load("package-release-docs")
smoke = load("release-smoke")


class ReleaseDocumentationTest(unittest.TestCase):
    def test_package_is_versioned_without_changing_source(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            source = root / "source"
            guide = source / "docs/guides/example.md"
            guide.parent.mkdir(parents=True)
            original = "---\nsummary: Example\n---\n\n# Historical v0.8 plan\nhttps://github.com/oxalc88/oxguard/blob/main/docs/example.md\n"
            guide.write_text(original)
            (source / "README.md").write_text("# Read me\n")
            output = root / "payload"
            package.package_docs("v0.9.0", output, source)
            smoke.check_documentation(output, "v0.9.0", "guides/example.md")
            stamped = (output / "docs/guides/example.md").read_text()
            self.assertTrue(stamped.startswith("---\nsummary: Example\n---\n"))
            self.assertIn("Historical v0.8 plan", stamped)
            self.assertIn("/blob/v0.9.0/docs/", stamped)
            self.assertEqual(guide.read_text(), original)
            with self.assertRaises(ValueError):
                smoke.check_documentation(output, "v0.9.1", "guides/example.md")
            (output / "VERSION").write_text("v0.9.1\n")
            with self.assertRaises(ValueError):
                smoke.check_documentation(output, "v0.9.1", "guides/example.md")
            package.package_docs("v0.9.1", output, source)
            smoke.check_documentation(output, "v0.9.1", "guides/example.md")

    def test_invalid_tag_cannot_create_payload(self):
        with tempfile.TemporaryDirectory() as temp:
            output = Path(temp) / "payload"
            with self.assertRaises(ValueError):
                package.package_docs("../../main", output)
            self.assertFalse(output.exists())

    def test_missing_guide_or_stale_link_blocks_smoke(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "VERSION").write_text("v0.9.0\n")
            (root / "README.md").write_text("OxGuard version: v0.9.0\n")
            with self.assertRaises(ValueError):
                smoke.check_documentation(root, "v0.9.0", "missing.md")
            (root / "README.md").write_text("OxGuard version: v0.9.01\n")
            with self.assertRaises(ValueError):
                smoke.check_documentation(root, "v0.9.0")
            (root / "README.md").write_text("OxGuard version: v0.9.0\nhttps://github.com/oxalc88/oxguard/blob/main/docs/example.md\n")
            with self.assertRaises(ValueError):
                smoke.check_documentation(root, "v0.9.0")


if __name__ == "__main__":
    unittest.main()
