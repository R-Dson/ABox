# /// script
# dependencies = ["requests"]
# ///
import json
import os
import requests


def get_latest_github(repo):
    url = f"https://api.github.com/repos/{repo}/releases/latest"
    response = requests.get(url)
    if response.status_code == 200:
        return response.json()["tag_name"]
    return None


def get_latest_npm(package):
    url = f"https://registry.npmjs.org/{package}/latest"
    response = requests.get(url)
    if response.status_code == 200:
        return response.json()["version"]
    return None


def get_latest_pypi(package):
    url = f"https://pypi.org/pypi/{package}/json"
    response = requests.get(url)
    if response.status_code == 200:
        return response.json()["info"]["version"]
    return None


def main():
    config_path = "config/editors.json"
    if not os.path.exists(config_path):
        print(f"Error: {config_path} not found")
        return

    with open(config_path, "r") as f:
        data = json.load(f)

    mappings = {
        "opencode": ("github", "anomalyco/opencode"),
        "claude": ("npm", "@anthropic-ai/claude-code"),
        "aider": ("pypi", "aider-chat"),
        "copilot": ("npm", "@github/copilot"),
        "vibe": ("pypi", "mistral-vibe"),
        "goose": ("github", "block/goose"),
        "codex": ("npm", "@openai/codex"),
        "gemini": ("npm", "@google/gemini-cli"),
    }

    updated = False
    for editor, (source, identifier) in mappings.items():
        if editor not in data["editors"]:
            continue

        latest = None
        if source == "github":
            latest = get_latest_github(identifier)
        elif source == "npm":
            latest = get_latest_npm(identifier)
        elif source == "pypi":
            latest = get_latest_pypi(identifier)

        if latest:
            clean_version = latest.lstrip("v")
            current_version = data["editors"][editor].get("version")
            if current_version != clean_version:
                print(f"Updating {editor}: {current_version} -> {clean_version}")
                data["editors"][editor]["version"] = clean_version
                updated = True
            else:
                print(f"{editor} is up to date ({clean_version})")
        else:
            print(f"Could not find latest for {editor}")

    if updated:
        with open(config_path, "w") as f:
            json.dump(data, f, indent=2)
        print(f"Updated {config_path}")
    else:
        print("All versions are up to date")


if __name__ == "__main__":
    main()
