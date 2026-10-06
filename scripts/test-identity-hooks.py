"""Exercise identity hooks through real Git commits and local pushes."""

import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

SOURCE = Path(__file__).resolve().parent.parent
NAME = "Personal User"
EMAIL = "personal@example.com"


class IdentityHooksTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="oxguard-identity-test-")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name) / "project"
        self.root.mkdir()
        self.env = {
            k: v for k, v in os.environ.items()
            if not k.startswith(("GIT_AUTHOR_", "GIT_COMMITTER_"))
        }
        self.git("init", "-q", "-b", "main")
        shutil.copytree(SOURCE / ".githooks", self.root / ".githooks")
        self.install()
        (self.root / "example.txt").write_text("example\n")
        self.git("add", "example.txt")

    def git(self, *args, ok=True, env=None):
        result = subprocess.run(
            ["git", *args], cwd=self.root, env={**self.env, **(env or {})},
            capture_output=True, text=True,
        )
        if ok:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        return result

    def install(self, ok=True):
        result = subprocess.run(
            ["sh", str(SOURCE / "scripts/install-identity-hooks.sh"), NAME, EMAIL],
            cwd=self.root, env=self.env, capture_output=True, text=True,
        )
        self.assertEqual(result.returncode == 0, ok, result.stdout + result.stderr)
        return result

    def test_personal_commit_succeeds(self):
        self.git("commit", "-qm", "personal commit")
        self.assertEqual(self.git("log", "-1", "--format=%ae|%ce").stdout.strip(), f"{EMAIL}|{EMAIL}")

    def test_wrong_config_blocks_even_no_verify(self):
        self.git("config", "--local", "user.email", "work@example.com")
        result = self.git("commit", "--no-verify", "-qm", "wrong identity", ok=False)
        self.assertIn("Commit blocked", result.stderr)

    def test_explicit_author_override_is_blocked(self):
        result = self.git("commit", "--author=Work User <work@example.com>", "-qm", "wrong author", ok=False)
        self.assertIn("author must use", result.stderr)

    def test_committer_environment_override_is_blocked(self):
        result = self.git("commit", "-qm", "wrong committer", ok=False, env={"GIT_COMMITTER_EMAIL": "work@example.com"})
        self.assertIn("committer must use", result.stderr)

    def test_tag_with_wrong_tagger_cannot_be_pushed(self):
        self.git("commit", "-qm", "personal commit")
        remote = Path(self.temporary.name) / "remote.git"
        self.git("init", "--bare", "-q", str(remote))
        self.git("remote", "add", "origin", str(remote))
        self.git("push", "-q", "origin", "main")
        self.git("tag", "-a", "v1.0.0", "-m", "wrong tagger", env={"GIT_COMMITTER_EMAIL": "work@example.com"})
        result = self.git("push", "origin", "v1.0.0", ok=False)
        self.assertIn("tagger must use", result.stderr)
        self.assertEqual(self.git("ls-remote", "--tags", "origin").stdout, "")
        self.git("tag", "-a", "v1.0.1", "-m", "personal tagger")
        self.git("push", "-q", "origin", "v1.0.1")

    def test_installer_preserves_existing_hooks(self):
        self.git("config", "--unset", "core.hooksPath")
        hook = self.root / ".git/hooks/pre-commit"
        hook.write_text("#!/bin/sh\nexit 0\n")
        result = self.install(ok=False)
        self.assertIn("refusing to disable", result.stderr)
        self.assertEqual(hook.read_text(), "#!/bin/sh\nexit 0\n")


if __name__ == "__main__":
    unittest.main()
