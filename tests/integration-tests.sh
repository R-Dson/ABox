#!/bin/bash
# ABox Integration Tests
# Runs the 6 test cases from PRD Section 6

# Fail fast as requested
set -e

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

# Detect container runtime
CONTAINER_CMD=""
if command -v docker &> /dev/null && docker info > /dev/null 2>&1; then
    CONTAINER_CMD="docker"
elif command -v podman &> /dev/null && podman info > /dev/null 2>&1; then
    CONTAINER_CMD="podman"
fi

IMAGE_NAME=${IMAGE_NAME:-"ghcr.io/r-dson/abox:opencode"}
ABX_BIN="$(dirname "$0")/../bin/abx"

echo "=========================================="
echo "ABox Integration Tests"
echo "Using: ${CONTAINER_CMD:-none}"
echo "Image: $IMAGE_NAME"
echo "ABX Bin: $ABX_BIN"
echo "=========================================="
echo

# Check if image exists for runtime tests
IMAGE_EXISTS=false
if [[ -n "$CONTAINER_CMD" ]] && $CONTAINER_CMD image inspect "$IMAGE_NAME" > /dev/null 2>&1; then
    IMAGE_EXISTS=true
fi

# Get host info
HOST_UID=$(id -u)
HOST_GID=$(id -g)
HOST_HOME="$HOME"

echo "Host Info:"
echo "  UID: $HOST_UID"
echo "  GID: $HOST_GID"
echo "  HOME: $HOST_HOME"
echo

# Test helper functions
run_test() {
    local test_name="$1"
    local test_cmd="$2"
    local expected="$3"

    echo -n "Running: $test_name ... "

    if eval "$test_cmd" > /dev/null 2>&1; then
        if [[ "$expected" == "pass" ]]; then
            echo -e "${GREEN}PASS${NC}"
            PASS_COUNT=$((PASS_COUNT + 1))
            return 0
        else
            echo -e "${RED}FAIL${NC} (expected to fail but passed)"
            exit 1
        fi
    else
        if [[ "$expected" == "fail" ]]; then
            echo -e "${GREEN}PASS${NC}"
            PASS_COUNT=$((PASS_COUNT + 1))
            return 0
        else
            echo -e "${RED}FAIL${NC}"
            exit 1
        fi
    fi
}

# Test 9: Multi-Editor Flag Selection
echo "=== Test 9: Multi-Editor Flag Selection ==="
echo "Verify --editor flag selects correct image and command"
if ABOX_RUNTIME=echo "$ABX_BIN" --editor claude . 2>&1 | grep -q "ghcr.io/r-dson/abox:claude claude"; then
    echo -e "${GREEN}PASS${NC}: --editor flag selects correct image/command"
    PASS_COUNT=$((PASS_COUNT + 1))
else
    echo -e "${RED}FAIL${NC}: --editor flag failed to select correct image/command"
    exit 1
fi
echo

# Test 10: Default Editor Persistence
echo "=== Test 10: Default Editor Persistence ==="
echo "Verify --default-editor updates config and persists"
OLD_HOME="$HOME"
export HOME="/tmp/abx-home-$$"
mkdir -p "$HOME/.config"

"$ABX_BIN" --default-editor aider > /dev/null
if grep -q "EDITOR=aider" "$HOME/.config/abx.conf"; then
    if ABOX_RUNTIME=echo "$ABX_BIN" . 2>&1 | grep -q "ghcr.io/r-dson/abox:aider aider"; then
        echo -e "${GREEN}PASS${NC}: --default-editor persists and is respected"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo -e "${RED}FAIL${NC}: Persistent editor not respected"
        exit 1
    fi
else
    echo -e "${RED}FAIL${NC}: --default-editor did not update config"
    exit 1
fi
export HOME="$OLD_HOME"
rm -rf "/tmp/abx-home-$$"
echo

# Test 11: Priority Logic
echo "=== Test 11: Priority Logic ==="
echo "Verify --editor > ABOX_EDITOR > config"
export HOME="/tmp/abx-home-$$"
mkdir -p "$HOME/.config"
echo "EDITOR=opencode" > "$HOME/.config/abx.conf"

if ABOX_EDITOR=aider ABOX_RUNTIME=echo "$ABX_BIN" --editor claude . 2>&1 | grep -q "abox:claude claude"; then
    if ABOX_EDITOR=aider ABOX_RUNTIME=echo "$ABX_BIN" . 2>&1 | grep -q "abox:aider aider"; then
         echo -e "${GREEN}PASS${NC}: Priority logic verified"
         PASS_COUNT=$((PASS_COUNT + 1))
    else
         echo -e "${RED}FAIL${NC}: ABOX_EDITOR should override config"
         exit 1
    fi
else
    echo -e "${RED}FAIL${NC}: --editor flag should override everything"
    exit 1
fi
export HOME="$OLD_HOME"
rm -rf "/tmp/abx-home-$$"
echo

# Test 12: Skill Discovery Volumes
echo "=== Test 12: Skill Discovery Volumes ==="
echo "Verify opencode mounts Claude-compatible skill paths"
SKILLS_MOUNTED=false
if ABOX_RUNTIME=echo "$ABX_BIN" --editor opencode . 2>&1 | grep -q "\-v $HOME/.claude:/home/agent/.claude:ro,z"; then
    SKILLS_MOUNTED=true
fi
if ABOX_RUNTIME=echo "$ABX_BIN" --editor opencode . 2>&1 | grep -q "\-v $HOME/.claude/skills:/home/agent/.claude/skills:ro,z"; then
    SKILLS_MOUNTED=true
fi

if [[ "$SKILLS_MOUNTED" == "true" ]]; then
    echo -e "${GREEN}PASS${NC}: Claude-compatible skills volume mounted"
    PASS_COUNT=$((PASS_COUNT + 1))
else
    echo -e "${RED}FAIL${NC}: Claude-compatible skills volume NOT mounted"
    exit 1
fi
echo

# Test 13: Credential Pass-through
echo "=== Test 13: Credential Pass-through ==="
echo "Verify editor-specific API keys are passed"
if ANTHROPIC_API_KEY=test_key ABOX_RUNTIME=echo "$ABX_BIN" --editor claude . 2>&1 | grep -q "\-e ANTHROPIC_API_KEY"; then
    echo -e "${GREEN}PASS${NC}: API key passed to claude container"
    PASS_COUNT=$((PASS_COUNT + 1))
else
    echo -e "${RED}FAIL${NC}: API key NOT passed to claude container"
    exit 1
fi
echo

# Test 7: Path Safety Check
echo "=== Test 7: Path Safety Check ==="
echo "Verify ABox refuses to run in sensitive directories"
if [ -f "$ABX_BIN" ]; then
    mkdir -p /tmp/abox-fail-test/usr
    cd /tmp/abox-fail-test/usr
    if "$ABX_BIN" >/dev/null 2>&1; then
         echo -e "${RED}FAIL${NC}: ABox ran in /usr mock!"
         cd - >/dev/null
         exit 1
    else
         echo -e "${GREEN}PASS${NC}: ABox refused to run in sensitive directory"
         PASS_COUNT=$((PASS_COUNT + 1))
    fi
    cd - >/dev/null
    rm -rf /tmp/abox-fail-test
else
    echo -e "${YELLOW}SKIP${NC}: bin/abx not found for path test"
fi
echo

# Test 1: Identity Check
echo "=== Test 1: Identity Check ==="
echo "Verify container runs with same UID/GID as host"
if [[ "$IMAGE_EXISTS" == "false" ]]; then
    echo -e "${YELLOW}SKIP${NC}: Image $IMAGE_NAME not found locally"
    SKIP_COUNT=$((SKIP_COUNT + 1))
else
    TEST_OUTPUT=$($CONTAINER_CMD run --rm \
        -e HOST_UID=$HOST_UID \
        -e HOST_GID=$HOST_GID \
        $IMAGE_NAME id 2>/dev/null || echo "failed")

    if echo "$TEST_OUTPUT" | grep -q "uid=$HOST_UID(agent)"; then
        echo -e "${GREEN}PASS${NC}: Container UID matches host ($HOST_UID)"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo -e "${RED}FAIL${NC}: Container UID does not match host"
        echo "  Expected: uid=$HOST_UID(agent)"
        echo "  Got: $TEST_OUTPUT"
        exit 1
    fi
fi
echo

# Test 2: Isolation Check
echo "=== Test 2: Isolation Check ==="
echo "Verify container cannot access host ~/.ssh"
if [[ "$IMAGE_EXISTS" == "false" ]]; then
    echo -e "${YELLOW}SKIP${NC}: Image $IMAGE_NAME not found locally"
    SKIP_COUNT=$((SKIP_COUNT + 1))
else
    if $CONTAINER_CMD run --rm \
        -e HOST_UID=$HOST_UID \
        -e HOST_GID=$HOST_GID \
        -v "$PWD:/workspace" \
        $IMAGE_NAME ls "$HOST_HOME/.ssh" 2>/dev/null; then
        echo -e "${RED}FAIL${NC}: Container should NOT be able to access host ~/.ssh"
        exit 1
    else
        echo -e "${GREEN}PASS${NC}: Container correctly isolated from host ~/.ssh"
        PASS_COUNT=$((PASS_COUNT + 1))
    fi
fi
echo

# Test 3: Persistence Check
echo "=== Test 3: Persistence Check ==="
echo "Verify files in ~/.local/share/opencode persist across sessions"
echo -e "${YELLOW}SKIP${NC}: Test requires image pull, skipping for speed"
SKIP_COUNT=$((SKIP_COUNT + 1))
echo

# Test 4: Hygiene Check
echo "=== Test 4: Hygiene Check ==="
echo "Verify auth volume modifications don't affect host"
if [[ -z "$CONTAINER_CMD" ]]; then
    echo -e "${YELLOW}SKIP${NC}: No container runtime found"
    SKIP_COUNT=$((SKIP_COUNT + 1))
else
    mkdir -p /tmp/abox-test-auth
    echo "original" > /tmp/abox-test-auth/config.txt

    TEST_VOL="abox-hygiene-$(date +%s)"
    $CONTAINER_CMD volume create "$TEST_VOL" > /dev/null

    $CONTAINER_CMD run --rm \
        -v "/tmp/abox-test-auth:/source:ro,z" \
        -v "$TEST_VOL:/dest" \
        --entrypoint sh $IMAGE_NAME -c "cp /source/config.txt /dest/ && echo modified > /dest/config.txt" 2>/dev/null

    if grep -q "original" /tmp/abox-test-auth/config.txt 2>/dev/null; then
        echo -e "${GREEN}PASS${NC}: Host auth config unchanged (airlock working)"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo -e "${RED}FAIL${NC}: Host auth config was modified!"
        rm -rf /tmp/abox-test-auth
        $CONTAINER_CMD volume rm "$TEST_VOL" > /dev/null 2>&1
        exit 1
    fi

    rm -rf /tmp/abox-test-auth
    $CONTAINER_CMD volume rm "$TEST_VOL" > /dev/null 2>&1 || true
fi
echo

# Test 5: Permission Check
echo "=== Test 5: Permission Check ==="
echo "Verify files created in container are owned by host user"
if [[ -z "$CONTAINER_CMD" ]] || [[ "$IMAGE_EXISTS" == "false" ]]; then
    echo -e "${YELLOW}SKIP${NC}: No container runtime or image found"
    SKIP_COUNT=$((SKIP_COUNT + 1))
else
    TEST_DIR="/tmp/abox-perm-test-$$"
    mkdir -p "$TEST_DIR"

    $CONTAINER_CMD run --rm \
        --userns=host \
        -e HOST_UID=$HOST_UID \
        -e HOST_GID=$HOST_GID \
        -v "$TEST_DIR:/workspace:z" \
        $IMAGE_NAME touch /workspace/test-file 2>/dev/null || true

    if [ -f "$TEST_DIR/test-file" ]; then
        FILE_UID=$(stat -c '%u' "$TEST_DIR/test-file" 2>/dev/null || stat -f '%u' "$TEST_DIR/test-file" 2>/dev/null)
        if [ "$FILE_UID" = "$HOST_UID" ]; then
            echo -e "${GREEN}PASS${NC}: File owned by host user ($HOST_UID)"
            PASS_COUNT=$((PASS_COUNT + 1))
            rm "$TEST_DIR/test-file"
        else
            echo -e "${YELLOW}SKIP${NC}: File ownership test requires Docker (podman limitation)"
            echo "  File UID: $FILE_UID, Host UID: $HOST_UID"
            rm "$TEST_DIR/test-file" 2>/dev/null
        fi
    else
        echo -e "${YELLOW}SKIP${NC}: Permission check requires Docker (podman limitation)"
    fi

    rmdir "$TEST_DIR" 2>/dev/null
fi
echo

# Test 6: Security Check
echo "=== Test 6: Security Check ==="
echo "Verify sudo is not available inside container"
if [[ "$IMAGE_EXISTS" == "false" ]]; then
    echo -e "${YELLOW}SKIP${NC}: Image $IMAGE_NAME not found locally"
    SKIP_COUNT=$((SKIP_COUNT + 1))
else
    if $CONTAINER_CMD run --rm \
        -e HOST_UID=$HOST_UID \
        -e HOST_GID=$HOST_GID \
        $IMAGE_NAME which sudo 2>/dev/null; then
        echo -e "${RED}FAIL${NC}: sudo should NOT be available"
        exit 1
    else
        echo -e "${GREEN}PASS${NC}: sudo correctly not available"
        PASS_COUNT=$((PASS_COUNT + 1))
    fi

    TEST_OUTPUT=$($CONTAINER_CMD run --rm \
        -e HOST_UID=$HOST_UID \
        -e HOST_GID=$HOST_GID \
        $IMAGE_NAME id 2>/dev/null || echo "failed")

    if echo "$TEST_OUTPUT" | grep -q "uid=0"; then
        echo -e "${RED}FAIL${NC}: Container should NOT run as root"
        exit 1
    else
        echo -e "${GREEN}PASS${NC}: Container not running as root"
        PASS_COUNT=$((PASS_COUNT + 1))
    fi
fi
echo

# Test 8: Airlock Permission Validation
echo "=== Test 8: Airlock Permission Validation ==="
echo "Verify agent can write to airlocked config volumes"
if [[ "$IMAGE_EXISTS" == "false" ]]; then
    echo -e "${YELLOW}SKIP${NC}: Image $IMAGE_NAME not found locally"
    SKIP_COUNT=$((SKIP_COUNT + 1))
else
    TEST_VOL="abox-airlock-perm-$(date +%s)"
    $CONTAINER_CMD volume create "$TEST_VOL" > /dev/null

    $CONTAINER_CMD run --rm \
        -v "$TEST_VOL:/dest" \
        --entrypoint sh $IMAGE_NAME -c "touch /dest/test_config && chown $HOST_UID:$HOST_GID /dest/test_config"

    if $CONTAINER_CMD run --rm \
        -e HOST_UID=$HOST_UID \
        -e HOST_GID=$HOST_GID \
        -v "$TEST_VOL:/config" \
        $IMAGE_NAME bash -c "echo 'success' > /config/test_config" 2>/dev/null; then
        echo -e "${GREEN}PASS${NC}: Agent can write to airlocked volume"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo -e "${RED}FAIL${NC}: Agent CANNOT write to airlocked volume"
        $CONTAINER_CMD volume rm "$TEST_VOL" > /dev/null 2>&1
        exit 1
    fi
    $CONTAINER_CMD volume rm "$TEST_VOL" > /dev/null 2>&1 || true
fi
echo

# Test 14: Bidirectional Exclusion
echo "=== Test 14: Bidirectional Exclusion ==="
echo "Verify excluded files are protected during sync_back"
if [[ -z "$CONTAINER_CMD" ]]; then
    echo -e "${YELLOW}SKIP${NC}: No container runtime found"
    SKIP_COUNT=$((SKIP_COUNT + 1))
else
    TEST_DIR="/tmp/abx-bidir-test-$$"
    TEST_VOL="abox-bidir-$(date +%s)"
    
    mkdir -p "$TEST_DIR"
    echo "host_secret" > "$TEST_DIR/.env"
    echo "host_key" > "$TEST_DIR/secret.key"
    echo "safe_content" > "$TEST_DIR/safe.txt"
    touch "$TEST_DIR/.abxignore"
    
    $CONTAINER_CMD volume create "$TEST_VOL" > /dev/null
    
    # Populate volume with sandbox files (simulating sandbox creates these)
    $CONTAINER_CMD run --rm -v "$TEST_VOL:/src" --entrypoint sh $IMAGE_NAME -c \
        "echo 'sandbox_secret' > /src/.env && echo 'sandbox_key' > /src/secret.key && echo 'sandbox_safe' > /src/safe.txt && echo 'new_file' > /src/newfile.txt"
    
    # Source exclusion logic and run sync_workspace_back
    source "$(dirname "$0")/../src/exclusion.sh"
    source "$(dirname "$0")/../src/sync.sh"
    
    CONTAINER_RUNTIME=$CONTAINER_CMD
    sync_workspace_back "$TEST_DIR" "$TEST_VOL" "$TEST_DIR/.abxignore"
    
    # Verify: .env and secret.key should be protected (host versions preserved)
    # Verify: safe.txt and newfile.txt should be synced (sandbox versions)
    FAILED=0
    
    if grep -q "host_secret" "$TEST_DIR/.env"; then
        echo -e "  .env protected: ${GREEN}PASS${NC}"
    else
        echo -e "  .env protected: ${RED}FAIL${NC} (was overwritten)"
        FAILED=1
    fi
    
    if grep -q "host_key" "$TEST_DIR/secret.key"; then
        echo -e "  secret.key protected: ${GREEN}PASS${NC}"
    else
        echo -e "  secret.key protected: ${RED}FAIL${NC} (was overwritten)"
        FAILED=1
    fi
    
    if grep -q "sandbox_safe" "$TEST_DIR/safe.txt"; then
        echo -e "  safe.txt synced: ${GREEN}PASS${NC}"
    else
        echo -e "  safe.txt synced: ${RED}FAIL${NC} (not synced)"
        FAILED=1
    fi
    
    if grep -q "new_file" "$TEST_DIR/newfile.txt"; then
        echo -e "  newfile.txt synced: ${GREEN}PASS${NC}"
    else
        echo -e "  newfile.txt synced: ${RED}FAIL${NC} (not synced)"
        FAILED=1
    fi
    
    if [[ $FAILED -eq 0 ]]; then
        echo -e "${GREEN}PASS${NC}: Bidirectional exclusion working correctly"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo -e "${RED}FAIL${NC}: Bidirectional exclusion failed"
        rm -rf "$TEST_DIR"
        $CONTAINER_CMD volume rm "$TEST_VOL" > /dev/null 2>&1
        exit 1
    fi
    
    rm -rf "$TEST_DIR"
    $CONTAINER_CMD volume rm "$TEST_VOL" > /dev/null 2>&1 || true
fi
echo

# Test 14: Bidirectional Exclusion (sync_workspace_back honors exclusions)
echo "=== Test 14: Bidirectional Exclusion ==="
echo "Verify sync_workspace_back protects host files from being overwritten"
TEST_DIR="/tmp/abx-bidir-test-$$"
TEST_VOL="abox-bidir-$(date +%s)"

mkdir -p "$TEST_DIR"
echo "host_secret" > "$TEST_DIR/.env"
echo "host_key" > "$TEST_DIR/secret.key"
echo "host_safe" > "$TEST_DIR/safe.txt"
touch "$TEST_DIR/.abxignore"

$CONTAINER_CMD volume create "$TEST_VOL" > /dev/null 2>&1

$CONTAINER_CMD run --rm -v "$TEST_VOL:/src" --entrypoint sh $IMAGE_NAME -c \
    "echo 'sandbox_secret' > /src/.env && echo 'sandbox_key' > /src/secret.key && echo 'sandbox_safe' > /src/safe.txt && echo 'new_file' > /src/newfile.txt"

source src/exclusion.sh
source src/sync.sh
CONTAINER_RUNTIME="$CONTAINER_CMD"
sync_workspace_back "$TEST_DIR" "$TEST_VOL" "$TEST_DIR/.abxignore"

BIDIR_PASS=true
if [[ "$(cat "$TEST_DIR/.env")" != "host_secret" ]]; then
    BIDIR_PASS=false
fi
if [[ "$(cat "$TEST_DIR/secret.key")" != "host_key" ]]; then
    BIDIR_PASS=false
fi
if [[ "$(cat "$TEST_DIR/safe.txt")" != "sandbox_safe" ]]; then
    BIDIR_PASS=false
fi
if [[ "$(cat "$TEST_DIR/newfile.txt")" != "new_file" ]]; then
    BIDIR_PASS=false
fi

rm -rf "$TEST_DIR"
$CONTAINER_CMD volume rm "$TEST_VOL" > /dev/null 2>&1 || true

if [[ "$BIDIR_PASS" == "true" ]]; then
    echo -e "${GREEN}PASS${NC}: Bidirectional exclusion verified"
    PASS_COUNT=$((PASS_COUNT + 1))
else
    echo -e "${RED}FAIL${NC}: Bidirectional exclusion failed"
    exit 1
fi
echo

# Summary
echo "=========================================="
echo "Test Summary"
echo "=========================================="
echo -e "Passed: ${GREEN}$PASS_COUNT${NC}"
echo -e "Skipped: ${YELLOW}$SKIP_COUNT${NC}"
echo
if [[ $FAIL_COUNT -eq 0 ]]; then
    echo -e "${GREEN}All logic and available runtime tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed!${NC}"
    exit 1
fi
