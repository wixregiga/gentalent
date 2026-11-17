import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from gentalent.generative import (
    STRIPPED_API_KEY_VARS,
    SubscriptionAgentRunner,
    subscription_env,
)
from support import make_config


def _create_test_image(path: Path, color: str):
    convert_bin = shutil.which("convert") or "/usr/bin/convert"
    subprocess.run(
        [convert_bin, "-size", "1200x900", f"xc:{color}", str(path)],
        check=True,
        capture_output=True,
    )


class GenerativeMediaTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.config = make_config(self.temp.name)

    def tearDown(self):
        self.temp.cleanup()

    def test_subscription_env_strips_all_api_keys(self):
        fake_env = dict(os.environ)
        for var in STRIPPED_API_KEY_VARS:
            fake_env[var] = "secret-paid-key"
        cleaned = subscription_env(fake_env)
        for var in STRIPPED_API_KEY_VARS:
            self.assertNotIn(var, cleaned)

    def test_multi_model_failover_and_real_imagemagick_strip(self):
        calls = []

        def fake_which(name):
            return f"/usr/bin/{name}"

        def fake_run(cmd, input=None, cwd=None, env=None, capture_output=True, text=True, timeout=180):
            exe_name = Path(cmd[0]).name
            calls.append((exe_name, cmd, env))
            for var in STRIPPED_API_KEY_VARS:
                self.assertNotIn(var, env or {})

            if exe_name == "codex":
                return subprocess.CompletedProcess(cmd, 1, stdout="", stderr="codex rate limit")

            if exe_name == "claude":
                raw_dir = Path(cwd)
                for idx, color in enumerate(("navy", "crimson", "gold"), start=1):
                    _create_test_image(raw_dir / f"0{idx}.jpg", color)
                manifest = {
                    "canonical_name": "Welvin Harris",
                    "summary": "Viral Instagram comedian known for Deez Nuts.",
                    "photos": [
                        {
                            "raw_filename": f"0{idx}.jpg",
                            "source_page_url": f"https://example.org/page/{idx}",
                            "image_url": f"https://example.org/img/0{idx}.jpg",
                            "caption": f"Photo {idx}",
                        }
                        for idx in (1, 2, 3)
                    ],
                }
                return subprocess.CompletedProcess(
                    cmd,
                    0,
                    stdout=json.dumps({"structured_output": manifest}),
                    stderr="",
                )

            raise AssertionError(f"Unexpected invocation: {exe_name}")

        runner = SubscriptionAgentRunner(
            self.config,
            providers=[
                {"cli": "codex", "model": "gpt-6.1-sol"},
                {"cli": "claude", "model": "sonnet"},
                {"cli": "agy", "model": "gemini-4-carbon-medium"},
            ],
            run_fn=fake_run,
            which_fn=fake_which,
        )

        # Ensure env keys are stripped even if set in parent environment
        old_openai = os.environ.get("OPENAI_API_KEY")
        os.environ["OPENAI_API_KEY"] = "should-not-leak"
        try:
            bundle = runner.generate("Welvin Harris", "aka WelvinDaGreat")
        finally:
            if old_openai is None:
                os.environ.pop("OPENAI_API_KEY", None)
            else:
                os.environ["OPENAI_API_KEY"] = old_openai

        self.assertEqual([c[0] for c in calls], ["codex", "claude"])
        self.assertEqual(bundle.provider_used, "claude:sonnet")
        self.assertEqual(len(bundle.photo_paths), 3)
        for p in bundle.photo_paths:
            self.assertTrue(p.is_file())
        self.assertTrue(bundle.strip_path.is_file())
        self.assertTrue(bundle.sources_path.is_file())

        # Verify dimensions of generated 3-photo strip using real ImageMagick identify
        identify_bin = shutil.which("identify") or "/usr/bin/identify"
        ident = subprocess.run(
            [identify_bin, "-format", "%w %h", str(bundle.strip_path)],
            check=True,
            capture_output=True,
            text=True,
        )
        width, height = map(int, ident.stdout.strip().split())
        self.assertGreater(width, 1000)
        self.assertEqual(height, 492)  # 480px tall + 6px top/bottom border

        # Verify claude never gets --bare (which breaks subscription OAuth)
        claude_cmd = calls[1][1]
        self.assertNotIn("--bare", claude_cmd)
        self.assertIn("--json-schema", claude_cmd)

    def test_agy_fallback_runs_when_codex_and_claude_fail(self):
        def fake_which(name):
            return f"/usr/bin/{name}"

        def fake_run(cmd, input=None, cwd=None, env=None, capture_output=True, text=True, timeout=180):
            exe_name = Path(cmd[0]).name
            if exe_name in ("codex", "claude"):
                return subprocess.CompletedProcess(cmd, 1, stdout="", stderr=f"{exe_name} unavailable")
            raw_dir = Path(cwd)
            for idx, color in enumerate(("teal", "purple", "orange"), start=1):
                _create_test_image(raw_dir / f"0{idx}.jpg", color)
            manifest = {
                "canonical_name": "Aster Example",
                "summary": "Representative photos.",
                "photos": [
                    {
                        "raw_filename": f"0{idx}.jpg",
                        "source_page_url": f"https://example.org/p/{idx}",
                        "image_url": f"https://example.org/i/{idx}.jpg",
                        "caption": f"Shot {idx}",
                    }
                    for idx in (1, 2, 3)
                ],
            }
            return subprocess.CompletedProcess(cmd, 0, stdout=json.dumps(manifest), stderr="")

        runner = SubscriptionAgentRunner(self.config, run_fn=fake_run, which_fn=fake_which)
        bundle = runner.generate("Aster Example")
        self.assertEqual(bundle.provider_used, "agy:gemini-4-carbon-medium")
        self.assertTrue(bundle.strip_path.is_file())
