import os
import requests
import yaml

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
    if not os.path.exists("VERSIONS.yaml"):
        versions = {}
    else:
        with open("VERSIONS.yaml", "r") as f:
            versions = yaml.safe_load(f) or {}

    mappings = {
        "opencode": ("github", "anomalyco/opencode"),
        "claude": ("npm", "@anthropic-ai/claude-code"),
        "aider": ("pypi", "aider-chat"),
        "copilot": ("github", "github/copilot-cli"),
        "vibe": ("pypi", "mistral-vibe"),
        "goose": ("github", "block/goose"),
        "codex": ("npm", "@openai/codex"),
        "gemini": ("npm", "@google/gemini-cli"),
        "cursor": ("npm", "cursor-chat"),
    }

    updated = False
    for editor, (source, identifier) in mappings.items():
        latest = None
        if source == "github":
            latest = get_latest_github(identifier)
        elif source == "npm":
            latest = get_latest_npm(identifier)
        elif source == "pypi":
            latest = get_latest_pypi(identifier)

        if latest:
            print(f"Found {editor}: {latest}")
            if versions.get(editor) != latest:
                print(f"Updating {editor}: {versions.get(editor)} -> {latest}")
                versions[editor] = latest
                updated = True
        else:
            print(f"Could not find latest for {editor}")

    if updated:
        with open("VERSIONS.yaml", "w") as f:
            yaml.dump(versions, f, default_flow_style=False)
        print("Updated VERSIONS.yaml")
    else:
        print("All versions are up to date")

if __name__ == "__main__":
    main()
