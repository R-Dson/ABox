# /// script
# dependencies = ["requests"]
# ///
import json
import os
import sys
import tempfile

import requests

REQUEST_TIMEOUT = 30  # seconds


def get_latest_github(repo: str) -> str | None:
    url = f"https://api.github.com/repos/{repo}/releases/latest"
    response = requests.get(url, timeout=REQUEST_TIMEOUT)
    if response.status_code == 200:
        return response.json()["tag_name"]
    return None


def get_latest_npm(package: str) -> str | None:
    url = f"https://registry.npmjs.org/{package}/latest"
    response = requests.get(url, timeout=REQUEST_TIMEOUT)
    if response.status_code == 200:
        return response.json()["version"]
    return None


def get_latest_pypi(package: str) -> str | None:
    url = f"https://pypi.org/pypi/{package}/json"
    response = requests.get(url, timeout=REQUEST_TIMEOUT)
    if response.status_code == 200:
        return response.json()["info"]["version"]
    return None


# Maps editor name to (source_type, identifier).
# Editors not listed here are skipped (e.g., "pi" has no remote release API).
EDITORS: dict[str, tuple[str, str]] = {
    "opencode": ("github", "anomalyco/opencode"),
    "claude": ("npm", "@anthropic-ai/claude-code"),
    "aider": ("pypi", "aider-chat"),
    "copilot": ("npm", "@github/copilot"),
    "vibe": ("pypi", "mistral-vibe"),
    "goose": ("github", "block/goose"),
    "codex": ("npm", "@openai/codex"),
    "gemini": ("npm", "@google/gemini-cli"),
    # "pi" is intentionally skipped — installed via Go binary, no remote package.
}


def main() -> None:
    config_path = "config/editors.json"
    if not os.path.exists(config_path):
        print(f"Error: {config_path} not found", file=sys.stderr)
        sys.exit(1)

    with open(config_path, encoding="utf-8") as f:
        data = json.load(f)

    if "editors" not in data or not isinstance(data["editors"], dict):
        print("Error: editors.json missing 'editors' object", file=sys.stderr)
        sys.exit(1)

    updated = False
    for editor, (source, identifier) in EDITORS.items():
        if editor not in data["editors"]:
            print(f"Warning: editor '{editor}' not found in config, skipping")
            continue

        latest: str | None = None
        if source == "github":
            latest = get_latest_github(identifier)
        elif source == "npm":
            latest = get_latest_npm(identifier)
        elif source == "pypi":
            latest = get_latest_pypi(identifier)

        if latest is None:
            print(
                f"Warning: could not fetch latest version for {editor}", file=sys.stderr
            )
            continue

        clean_version = latest.lstrip("v")
        current_version = data["editors"][editor].get("version")
        if current_version != clean_version:
            print(f"Updating {editor}: {current_version} -> {clean_version}")
            data["editors"][editor]["version"] = clean_version
            updated = True
        else:
            print(f"{editor} is up to date ({clean_version})")

    if updated:
        # Atomic write: write to temp file then os.replace
        dir_name = os.path.dirname(config_path) or "."
        fd, tmp_path = tempfile.mkstemp(dir=dir_name, suffix=".json")
        try:
            with os.fdopen(fd, "w", encoding="utf-8") as f:
                json.dump(data, f, indent=2)
                f.write("\n")
            os.replace(tmp_path, config_path)
        except BaseException:
            # Clean up temp file on any error
            import contextlib

            with contextlib.suppress(OSError):
                os.unlink(tmp_path)
            raise
        print(f"Updated {config_path}")
    else:
        print("All versions are up to date")


if __name__ == "__main__":
    main()
