#!/bin/bash
# ABox Editor Registry Unit Tests
# Verifies get_editor_info() reads from editors.json (single source of truth)
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$TEST_DIR")"

# Source helpers to get get_editor_info()
source "$PROJECT_ROOT/src/helpers.sh"

FAILED=0
PASSED=0

assert_equals() {
    local description="$1"
    local expected="$2"
    local actual="$3"
    if [[ "$expected" == "$actual" ]]; then
        echo -e "  ${GREEN}PASS${NC}: $description"
        ((PASSED++)) || true
    else
        echo -e "  ${RED}FAIL${NC}: $description"
        echo "    Expected: $expected"
        echo "    Actual:   $actual"
        ((FAILED++)) || true
    fi
}

assert_field() {
    local description="$1"
    local editor="$2"
    local field_index="$3"  # 1-based: 1=image, 2=cmd, 3=config, 4=env, 5=legacy
    local expected="$4"

    local output
    output=$(get_editor_info "$editor")
    local actual
    actual=$(echo "$output" | cut -d'|' -f"$field_index")
    assert_equals "$description" "$expected" "$actual"
}

ALL_EDITORS=("aider" "claude" "codex" "copilot" "gemini" "goose" "vibe" "opencode")

echo "=========================================="
echo "Editor Registry Unit Tests"
echo "=========================================="
echo

# --- Verify all 8 editors produce correct output ---

echo "--- Image Tag (field 1) ---"
assert_field "aider image tag"        "aider"    1 "ghcr.io/r-dson/abox:aider"
assert_field "claude image tag"       "claude"   1 "ghcr.io/r-dson/abox:claude"
assert_field "codex image tag"        "codex"    1 "ghcr.io/r-dson/abox:codex"
assert_field "copilot image tag"      "copilot"  1 "ghcr.io/r-dson/abox:copilot"
assert_field "gemini image tag"       "gemini"   1 "ghcr.io/r-dson/abox:gemini"
assert_field "goose image tag"        "goose"    1 "ghcr.io/r-dson/abox:goose"
assert_field "vibe image tag"         "vibe"     1 "ghcr.io/r-dson/abox:vibe"
assert_field "opencode image tag"     "opencode" 1 "ghcr.io/r-dson/abox:opencode"

echo
echo "--- Command Name (field 2) ---"
assert_field "aider command name"     "aider"    2 "aider"
assert_field "claude command name"    "claude"   2 "claude"
assert_field "codex command name"     "codex"    2 "codex"
assert_field "copilot command name"   "copilot"  2 "copilot"
assert_field "gemini command name"    "gemini"   2 "gemini"
assert_field "goose command name"     "goose"    2 "goose"
assert_field "vibe command name"      "vibe"     2 "vibe"
assert_field "opencode command name"  "opencode" 2 "opencode"

echo
echo "--- Config Path (field 3) ---"
assert_field "aider config path"      "aider"    3 ".aider.conf.yml"
assert_field "claude config path"     "claude"   3 ".claude"
assert_field "codex config path"      "codex"    3 ".codex"
assert_field "copilot config path"    "copilot"  3 ".copilot"
assert_field "gemini config path"     "gemini"   3 ".gemini"
assert_field "goose config path"      "goose"    3 ".config/goose"
assert_field "vibe config path"       "vibe"     3 ".vibe"
assert_field "opencode config path"   "opencode" 3 ".config/opencode"

echo
echo "--- Env Vars (field 4) ---"
assert_field "aider env vars"         "aider"    4 "OPENAI_API_KEY,ANTHROPIC_API_KEY"
assert_field "claude env vars"        "claude"   4 "ANTHROPIC_API_KEY"
assert_field "codex env vars"         "codex"    4 ""
assert_field "copilot env vars"       "copilot"  4 "GITHUB_TOKEN"
assert_field "gemini env vars"        "gemini"   4 "GOOGLE_API_KEY"
assert_field "goose env vars"         "goose"    4 ""
assert_field "vibe env vars"          "vibe"     4 ""
assert_field "opencode env vars"      "opencode" 4 ""

echo
echo "--- Legacy Path (field 5) ---"
assert_field "opencode legacy path"   "opencode" 5 ".opencode"

echo
echo "--- Unknown editor falls back to opencode ---"
assert_field "unknown editor image"   "nonexistent" 1 "ghcr.io/r-dson/abox:opencode"
assert_field "unknown editor cmd"     "nonexistent" 2 "opencode"

echo
echo "--- JSON is the source of truth ---"
# Verify editors.json has the required runtime fields
EDITORS_JSON="$PROJECT_ROOT/config/editors.json"
if [[ ! -f "$EDITORS_JSON" ]]; then
    echo -e "  ${RED}FAIL${NC}: $EDITORS_JSON not found"
    ((FAILED++)) || true
else
    for editor in "${ALL_EDITORS[@]}"; do
        for field in image_tag config_path; do
            val=$(jq -r ".editors.\"$editor\".$field // \"\"" "$EDITORS_JSON")
            if [[ -z "$val" ]]; then
                echo -e "  ${RED}FAIL${NC}: editors.json missing $field for $editor"
                ((FAILED++)) || true
            else
                echo -e "  ${GREEN}PASS${NC}: editors.json has $field for $editor"
                ((PASSED++)) || true
            fi
        done
    done
fi

# Verify get_editor_info() output matches editors.json data
echo
echo "--- get_editor_info() reads from editors.json ---"
if [[ -f "$EDITORS_JSON" ]]; then
    for editor in "${ALL_EDITORS[@]}"; do
        expected_image=$(jq -r ".editors.\"$editor\".image_tag // \"\"" "$EDITORS_JSON")
        actual_image=$(get_editor_info "$editor" | cut -d'|' -f1)
        if [[ "$expected_image" != "$actual_image" ]]; then
            echo -e "  ${RED}FAIL${NC}: $editor image_tag mismatch (json=$expected_image, func=$actual_image)"
            ((FAILED++)) || true
        else
            echo -e "  ${GREEN}PASS${NC}: $editor image matches editors.json"
            ((PASSED++)) || true
        fi
    done
fi

echo "--- Container capabilities ---"
DAC_OUTPUT=$(ABOX_RUNTIME=echo bin/abx --editor claude . 2>&1)
if echo "$DAC_OUTPUT" | grep -q -- '--cap-add=DAC_OVERRIDE'; then
    MAIN_RUN=$(echo "$DAC_OUTPUT" | grep 'no-new-privileges' || true)
    if echo "$MAIN_RUN" | grep -q -- 'DAC_OVERRIDE'; then
        echo -e "  ${RED}FAIL${NC}: DAC_OVERRIDE present in editor container run command"
        ((FAILED++)) || true
    else
        echo -e "  ${GREEN}PASS${NC}: DAC_OVERRIDE not in editor container run"
        ((PASSED++)) || true
    fi
else
    echo -e "  ${GREEN}PASS${NC}: DAC_OVERRIDE not in editor container run"
    ((PASSED++)) || true
fi

echo "--- SSH mount security ---"
FAKE_SOCK="/tmp/abx_test_ssh_sock_$$"
python3 -c "import socket; s=socket.socket(socket.AF_UNIX); s.bind('$FAKE_SOCK')"
SSH_OUTPUT=$(SSH_AUTH_SOCK="$FAKE_SOCK" ABOX_RUNTIME=echo bin/abx --editor claude . 2>&1)
rm -f "$FAKE_SOCK"
MAIN_RUN=$(echo "$SSH_OUTPUT" | grep 'no-new-privileges' || true)
if echo "$MAIN_RUN" | grep -q '/tmp/ssh-agent.sock'; then
    echo -e "  ${GREEN}PASS${NC}: SSH agent socket forwarded when SSH_AUTH_SOCK is set"
    ((PASSED++)) || true
else
    echo -e "  ${RED}FAIL${NC}: Expected SSH agent socket forwarding, got directory mount"
    ((FAILED++)) || true
fi

echo "--- Seccomp profile applied to editor container ---"
SECCOMP_PATH="${ABOX_SECCOMP:-$PROJECT_ROOT/config/seccomp.json}"
MAIN_RUN=$(ABOX_SECCOMP="$SECCOMP_PATH" ABOX_RUNTIME=echo bin/abx --editor claude . 2>&1 | grep 'no-new-privileges' || true)
if echo "$MAIN_RUN" | grep -q 'seccomp'; then
    echo -e "  ${GREEN}PASS${NC}: Seccomp profile applied to editor container"
    ((PASSED++)) || true
else
    echo -e "  ${RED}FAIL${NC}: No seccomp profile in editor container run command"
    ((FAILED++)) || true
fi

echo "--- Strict network uses Docker network isolation ---"
STRICT_OUTPUT=$(ABOX_RUNTIME=echo bin/abx --strict-network --editor claude . 2>&1)
MAIN_RUN=$(echo "$STRICT_OUTPUT" | grep 'ghcr.io' || true)
if echo "$MAIN_RUN" | grep -q -- '--network'; then
    if echo "$MAIN_RUN" | grep -q 'add-host'; then
        echo -e "  ${RED}FAIL${NC}: --strict-network still uses insecure --add-host pattern"
        ((FAILED++)) || true
    else
        echo -e "  ${GREEN}PASS${NC}: --strict-network uses Docker network isolation"
        ((PASSED++)) || true
    fi
else
    echo -e "  ${RED}FAIL${NC}: --strict-network missing --network flag (uses --add-host)"
    ((FAILED++)) || true
fi

echo "--- JSON config support ---"
TEST_HOME_JSON="/tmp/abx-json-home-$$"
mkdir -p "$TEST_HOME_JSON/.config/abx"
echo '{"editor":"gemini","exclude_url":""}' > "$TEST_HOME_JSON/.config/abx/config.json"
JSON_OUTPUT=$(HOME="$TEST_HOME_JSON" ABOX_RUNTIME=echo bin/abx . 2>&1)
rm -rf "$TEST_HOME_JSON"
if echo "$JSON_OUTPUT" | grep -q 'abox:gemini gemini'; then
    echo -e "  ${GREEN}PASS${NC}: JSON config file read correctly (editor=gemini)"
    ((PASSED++)) || true
else
    echo -e "  ${RED}FAIL${NC}: JSON config file not read correctly"
    ((FAILED++)) || true
fi

echo "--- Custom env pass-through with --env ---"
ENV_OUTPUT=$(ABOX_RUNTIME=echo bin/abx --editor claude --env MY_CUSTOM_VAR . 2>&1)
MAIN_RUN=$(echo "$ENV_OUTPUT" | grep 'ghcr.io' || true)
if echo "$MAIN_RUN" | grep -q -- '-e MY_CUSTOM_VAR'; then
    echo -e "  ${GREEN}PASS${NC}: --env MY_CUSTOM_VAR passed to container"
    ((PASSED++)) || true
else
    echo -e "  ${RED}FAIL${NC}: --env flag not passed to container"
    ((FAILED++)) || true
fi

echo "--- Verbose flag enables log file ---"
LOG_DIR="/tmp/abx-verbose-test-$$"
mkdir -p "$LOG_DIR"
VERBOSE_OUTPUT=$(ABOX_VERBOSE=true ABX_LOG_DIR="$LOG_DIR" ABOX_RUNTIME=echo bin/abx --verbose --editor claude . 2>&1)
if [[ -f "$LOG_DIR/abx.log" ]]; then
    echo -e "  ${GREEN}PASS${NC}: Verbose mode creates log file"
    ((PASSED++)) || true
else
    echo -e "  ${RED}FAIL${NC}: Verbose mode does not create log file"
    ((FAILED++)) || true
fi
rm -rf "$LOG_DIR"

echo
echo "=========================================="
if [[ $FAILED -eq 0 ]]; then
    echo -e "${GREEN}All $PASSED tests passed!${NC}"
else
    echo -e "${RED}$PASSED passed, $FAILED failed${NC}"
fi
echo "=========================================="
exit $FAILED
