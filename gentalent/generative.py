from dataclasses import dataclass
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
from typing import Any, Callable, Optional

STRIPPED_API_KEY_VARS = (
    "OPENAI_API_KEY",
    "ANTHROPIC_API_KEY",
    "CLAUDE_API_KEY",
    "GEMINI_API_KEY",
    "GOOGLE_API_KEY",
    "ELECTRON_RUN_AS_NODE",
)

MEDIA_MANIFEST_SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "canonical_name": {"type": "string"},
        "summary": {"type": "string"},
        "photos": {
            "type": "array",
            "description": "Exactly 3 verified representative photos saved in raw_dir.",
            "items": {
                "type": "object",
                "properties": {
                    "raw_filename": {"type": "string"},
                    "source_page_url": {"type": "string"},
                    "image_url": {"type": "string"},
                    "caption": {"type": "string"},
                },
                "required": ["raw_filename", "source_page_url", "image_url", "caption"],
                "additionalProperties": False,
            },
        },
    },
    "required": ["canonical_name", "summary", "photos"],
    "additionalProperties": False,
}


@dataclass(frozen=True)
class PhotoSource:
    raw_filename: str
    source_page_url: str
    image_url: str
    caption: str


@dataclass(frozen=True)
class AgentMediaManifest:
    canonical_name: str
    summary: str
    photos: list[PhotoSource]


@dataclass(frozen=True)
class GeneratedMediaBundle:
    nominee_name: str
    summary: str
    artifact_dir: Path
    photo_paths: list[Path]
    strip_path: Path
    sources_path: Path
    provider_used: str


def slugify(name: str) -> str:
    slug = re.sub(r"[^a-z0-9]+", "_", name.casefold()).strip("_")
    return slug[:64] or "nominee"


def subscription_env(base_env: Optional[dict[str, str]] = None) -> dict[str, str]:
    """Return an environment stripped of API keys so CLIs use subscription logins only."""
    env = dict(os.environ if base_env is None else base_env)
    for key in STRIPPED_API_KEY_VARS:
        env.pop(key, None)
    return env


def _text(data: dict, key: str, limit: int) -> str:
    value = str(data.get(key, "")).strip()
    if not 1 <= len(value) <= limit:
        raise ValueError(f"{key} must be 1 to {limit} characters.")
    return value


def validate_manifest(data: Any) -> AgentMediaManifest:
    if not isinstance(data, dict):
        raise ValueError("Agent output must be a JSON object.")
    photos_raw = data.get("photos")
    if not isinstance(photos_raw, list) or len(photos_raw) != 3:
        raise ValueError("Manifest requires exactly 3 photos.")
    photos: list[PhotoSource] = []
    for item in photos_raw:
        if not isinstance(item, dict):
            raise ValueError("Each photo entry must be a JSON object.")
        fn = _text(item, "raw_filename", 120)
        if "/" in fn or "\\" in fn or fn in (".", ".."):
            raise ValueError("raw_filename must be a relative filename inside raw_dir.")
        photos.append(PhotoSource(
            raw_filename=fn,
            source_page_url=_text(item, "source_page_url", 2000),
            image_url=_text(item, "image_url", 2000),
            caption=_text(item, "caption", 300),
        ))
    return AgentMediaManifest(
        canonical_name=_text(data, "canonical_name", 120),
        summary=_text(data, "summary", 1000),
        photos=photos,
    )


def _extract_json_blob(text: str) -> Any:
    raw = text.strip()
    if not raw:
        raise ValueError("Empty agent output.")
    try:
        return json.loads(raw)
    except json.JSONDecodeError:
        pass
    fenced = re.search(r"```(?:json)?\s*(\{.*\})\s*```", raw, flags=re.DOTALL)
    if fenced:
        return json.loads(fenced.group(1))
    start = raw.find("{")
    end = raw.rfind("}")
    if start != -1 and end > start:
        return json.loads(raw[start:end + 1])
    raise ValueError("Could not parse JSON object from agent output.")


def _unwrap_cli_payload(payload: Any) -> Any:
    if isinstance(payload, str):
        return _extract_json_blob(payload)
    if not isinstance(payload, dict):
        raise ValueError("CLI response is not a JSON object.")
    if "structured_output" in payload and isinstance(payload["structured_output"], dict):
        return payload["structured_output"]
    if "photos" in payload and "canonical_name" in payload:
        return payload
    if "result" in payload:
        res = payload["result"]
        if isinstance(res, dict):
            return res
        if isinstance(res, str):
            return _extract_json_blob(res)
    return payload


def _resolve_tool(name: str, fallback_path: str) -> str:
    found = shutil.which(name)
    if found:
        return found
    if Path(fallback_path).exists():
        return fallback_path
    raise RuntimeError(f"Required image tool not found: {name}")


def build_media_bundle(
    manifest: AgentMediaManifest,
    nominee_name: str,
    raw_dir: Path,
    out_dir: Path,
    provider_used: str,
) -> GeneratedMediaBundle:
    """Verify raw downloads with identify and render optimized JPEGs + 3-photo strip."""
    identify_bin = _resolve_tool("identify", "/usr/bin/identify")
    convert_bin = _resolve_tool("convert", "/usr/bin/convert")

    raw_dir_resolved = raw_dir.resolve()
    raw_files: list[Path] = []
    for photo in manifest.photos:
        candidate = (raw_dir_resolved / photo.raw_filename).resolve()
        if candidate.parent != raw_dir_resolved or not candidate.is_file() or candidate.stat().st_size == 0:
            raise ValueError(f"Missing or invalid raw image file: {photo.raw_filename}")
        check = subprocess.run(
            [identify_bin, str(candidate)],
            capture_output=True,
            text=True,
            timeout=15,
        )
        if check.returncode != 0:
            raise ValueError(f"File is not a valid image: {photo.raw_filename}")
        raw_files.append(candidate)

    photos_dir = out_dir / "photos"
    photos_dir.mkdir(parents=True, exist_ok=True)
    slug = slugify(nominee_name or manifest.canonical_name)

    photo_paths: list[Path] = []
    for idx, src_path in enumerate(raw_files, start=1):
        dst_path = photos_dir / f"0{idx}_{slug}.jpg"
        res = subprocess.run(
            [
                convert_bin,
                str(src_path),
                "-auto-orient",
                "-resize",
                "960x960>",
                "-colorspace",
                "sRGB",
                "-strip",
                "-sampling-factor",
                "4:2:0",
                "-quality",
                "78",
                str(dst_path),
            ],
            capture_output=True,
            text=True,
            timeout=30,
        )
        if res.returncode != 0 or not dst_path.is_file():
            raise RuntimeError(f"Failed optimizing photo {idx}: {res.stderr.strip()}")
        photo_paths.append(dst_path)

    strip_path = out_dir / "strip.jpg"
    with tempfile.TemporaryDirectory(prefix="gentalent-strip-") as tmpdir:
        tiles: list[Path] = []
        for idx, p_path in enumerate(photo_paths, start=1):
            tile = Path(tmpdir) / f"tile_{idx}.jpg"
            res = subprocess.run(
                [
                    convert_bin,
                    str(p_path),
                    "-resize",
                    "x480",
                    "-bordercolor",
                    "#151e2b",
                    "-border",
                    "6x6",
                    str(tile),
                ],
                capture_output=True,
                text=True,
                timeout=30,
            )
            if res.returncode != 0 or not tile.is_file():
                raise RuntimeError(f"Failed preparing strip tile {idx}: {res.stderr.strip()}")
            tiles.append(tile)

        strip_res = subprocess.run(
            [
                convert_bin,
                *[str(t) for t in tiles],
                "+append",
                "-colorspace",
                "sRGB",
                "-strip",
                "-sampling-factor",
                "4:2:0",
                "-quality",
                "80",
                str(strip_path),
            ],
            capture_output=True,
            text=True,
            timeout=30,
        )
        if strip_res.returncode != 0 or not strip_path.is_file():
            raise RuntimeError(f"Failed creating 3-photo strip: {strip_res.stderr.strip()}")

    sources_lines = [
        f"{manifest.canonical_name}: Google Messages attachments",
        f"Provider: {provider_used}",
        f"Summary: {manifest.summary}",
        "",
    ]
    for p_path, meta in zip(photo_paths, manifest.photos):
        sources_lines.extend([
            p_path.name,
            f"Caption: {meta.caption}",
            f"Source page: {meta.source_page_url}",
            f"Image URL: {meta.image_url}",
            "",
        ])
    sources_path = out_dir / "SOURCES.txt"
    sources_path.write_text("\n".join(sources_lines), encoding="utf-8")

    manifest_json = out_dir / "manifest.json"
    manifest_json.write_text(
        json.dumps(
            {
                "canonical_name": manifest.canonical_name,
                "summary": manifest.summary,
                "provider_used": provider_used,
                "strip_path": str(strip_path),
                "photos": [
                    {
                        "file": p_path.name,
                        "source_page_url": meta.source_page_url,
                        "image_url": meta.image_url,
                        "caption": meta.caption,
                    }
                    for p_path, meta in zip(photo_paths, manifest.photos)
                ],
            },
            indent=2,
            ensure_ascii=False,
        ),
        encoding="utf-8",
    )

    return GeneratedMediaBundle(
        nominee_name=manifest.canonical_name,
        summary=manifest.summary,
        artifact_dir=out_dir,
        photo_paths=photo_paths,
        strip_path=strip_path,
        sources_path=sources_path,
        provider_used=provider_used,
    )


class SubscriptionAgentRunner:
    """Multi-model subscription CLI orchestrator for !!gt generative media."""

    def __init__(
        self,
        config,
        providers: Optional[list[dict[str, str]]] = None,
        timeout: int = 180,
        run_fn: Callable[..., subprocess.CompletedProcess] = subprocess.run,
        which_fn: Callable[[str], Optional[str]] = shutil.which,
    ):
        self.config = config
        self.providers = providers or list(config.data["ai_providers"])
        self.timeout = int(timeout)
        self.run_fn = run_fn
        self.which_fn = which_fn

    def media_root(self) -> Path:
        return self.config.path("media_dir")

    def build_prompt(self, nominee_name: str, pitch: str, raw_dir: Path) -> str:
        payload = {
            "user_message": (
                "Find 3 verified representative photos of the specified person, verify all 3 show the right "
                "person (replace any wrong-person image), download them into target_raw_dir as 01.jpg, 02.jpg, "
                "and 03.jpg (or .png/.webp if needed), and return ONLY JSON matching the output schema. "
                "Do not inspect parent directories outside target_raw_dir."
            ),
            "exhibit": {
                "name": nominee_name,
                "message": pitch or "",
            },
            "target_raw_dir": str(raw_dir),
            "expected_files": ["01.jpg", "02.jpg", "03.jpg"],
        }
        return json.dumps(payload, ensure_ascii=False)

    def _invoke_provider(
        self,
        cli_name: str,
        model: str,
        prompt: str,
        raw_dir: Path,
        schema_path: Path,
        out_path: Path,
    ) -> AgentMediaManifest:
        exe = self.which_fn(cli_name)
        if not exe:
            raise FileNotFoundError(f"{cli_name} CLI is not installed or on PATH.")

        env = subscription_env()
        if cli_name == "codex":
            cmd = [
                exe,
                "exec",
                "-m",
                model,
                "-c",
                'model_reasoning_effort="low"',
                "--ephemeral",
                "--skip-git-repo-check",
                "--sandbox",
                "workspace-write",
                "-c",
                "sandbox_workspace_write.network_access=true",
                "-C",
                str(raw_dir),
                "--output-schema",
                str(schema_path),
                "-o",
                str(out_path),
                "-",
            ]
            proc = self.run_fn(
                cmd,
                input=prompt,
                cwd=str(raw_dir),
                env=env,
                capture_output=True,
                text=True,
                timeout=self.timeout,
            )
            if proc.returncode != 0:
                raise RuntimeError(f"codex exited {proc.returncode}: {(proc.stderr or proc.stdout).strip()[:400]}")
            raw_text = out_path.read_text(encoding="utf-8") if out_path.exists() else proc.stdout
            return validate_manifest(_unwrap_cli_payload(_extract_json_blob(raw_text)))

        if cli_name == "claude":
            # Never pass --bare: subscription OAuth requires normal Claude settings/keychain access.
            cmd = [
                exe,
                "-p",
                "--model",
                model,
                "--effort",
                "low",
                "--no-session-persistence",
                "--disable-slash-commands",
                "--permission-mode",
                "bypassPermissions",
                "--add-dir",
                str(raw_dir),
                "--output-format",
                "json",
                "--json-schema",
                json.dumps(MEDIA_MANIFEST_SCHEMA),
                prompt,
            ]
            proc = self.run_fn(
                cmd,
                cwd=str(raw_dir),
                env=env,
                capture_output=True,
                text=True,
                timeout=self.timeout,
            )
            if proc.returncode != 0:
                raise RuntimeError(f"claude exited {proc.returncode}: {(proc.stderr or proc.stdout).strip()[:400]}")
            return validate_manifest(_unwrap_cli_payload(_extract_json_blob(proc.stdout)))

        if cli_name == "agy":
            cmd = [
                exe,
                "-p",
                "--model",
                model,
                "--effort",
                "low",
                "--dangerously-skip-permissions",
                "--disable-slash-commands",
                "--add-dir",
                str(raw_dir),
                "--output-format",
                "json",
                "--json-schema",
                str(schema_path),
                prompt,
            ]
            proc = self.run_fn(
                cmd,
                cwd=str(raw_dir),
                env=env,
                capture_output=True,
                text=True,
                timeout=self.timeout,
            )
            if proc.returncode != 0:
                raise RuntimeError(f"agy exited {proc.returncode}: {(proc.stderr or proc.stdout).strip()[:400]}")
            return validate_manifest(_unwrap_cli_payload(_extract_json_blob(proc.stdout)))

        raise ValueError(f"Unsupported CLI provider: {cli_name}")

    def generate(
        self,
        nominee_name: str,
        pitch: str = "",
        cli_override: Optional[str] = None,
    ) -> GeneratedMediaBundle:
        slug = slugify(nominee_name)
        base_dir = self.media_root() / slug
        raw_dir = base_dir / "raw"
        out_dir = base_dir / "google_messages"
        raw_dir.mkdir(parents=True, exist_ok=True)
        out_dir.mkdir(parents=True, exist_ok=True)

        schema_path = base_dir / "manifest.schema.json"
        schema_path.write_text(json.dumps(MEDIA_MANIFEST_SCHEMA, indent=2), encoding="utf-8")
        out_path = base_dir / "agent_output.json"
        prompt = self.build_prompt(nominee_name, pitch, raw_dir)

        providers = self.providers
        if cli_override:
            providers = [p for p in self.providers if p.get("cli") == cli_override]
            if not providers:
                raise ValueError(f"{cli_override} is not listed in ai_providers in config.json.")

        errors: list[str] = []
        for entry in providers:
            cli_name = entry["cli"]
            model = entry.get("model", "")
            for leftover in raw_dir.iterdir():
                if leftover.is_file():
                    leftover.unlink()
            out_path.unlink(missing_ok=True)
            try:
                manifest = self._invoke_provider(cli_name, model, prompt, raw_dir, schema_path, out_path)
                return build_media_bundle(
                    manifest=manifest,
                    nominee_name=nominee_name,
                    raw_dir=raw_dir,
                    out_dir=out_dir,
                    provider_used=f"{cli_name}:{model}" if model else cli_name,
                )
            except Exception as exc:
                errors.append(f"{cli_name} ({model}): {exc}")

        raise RuntimeError("All configured subscription AI providers failed: " + " | ".join(errors))
