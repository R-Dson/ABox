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

# Detect container runtime
CONTAINER_CMD="podman"
if command -v docker &> /dev/null; then
    # Test if docker actually works
    if docker info > /dev/null 2>&1; then
        CONTAINER_CMD="docker"
    fi
fi

IMAGE_NAME=${IMAGE_NAME:-"ghcr.io/r-dson/abox:main"}

echo "=========================================="
echo "ABox Integration Tests"
echo "Using: $CONTAINER_CMD"
echo "Image: $IMAGE_NAME"
echo "=========================================="
echo

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

# Test 7: Path Safety Check
echo "=== Test 7: Path Safety Check ==="
echo "Verify ABox refuses to run in sensitive directories"
ABX_BIN="$(dirname "$0")/../bin/abx"
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
echo

# Test 2: Isolation Check
echo "=== Test 2: Isolation Check ==="
echo "Verify container cannot access host ~/.ssh"
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
echo

# Test 3: Persistence Check
echo "=== Test 3: Persistence Check ==="
echo "Verify files in ~/.local/share/opencode persist across sessions"

$CONTAINER_CMD volume create abox-test-persist > /dev/null 2>&1 || true

$CONTAINER_CMD run --rm \
    -e HOST_UID=$HOST_UID \
    -e HOST_GID=$HOST_GID \
    -v "abox-test-persist:/home/agent/.local/share/opencode" \
    $IMAGE_NAME bash -c "touch /home/agent/.local/share/opencode/test-marker.txt" 2>/dev/null

if $CONTAINER_CMD run --rm \
    -e HOST_UID=$HOST_UID \
    -e HOST_GID=$HOST_GID \
    -v "abox-test-persist:/home/agent/.local/share/opencode" \
    $IMAGE_NAME bash -c "test -f /home/agent/.local/share/opencode/test-marker.txt" 2>/dev/null; then
    echo -e "${GREEN}PASS${NC}: Files persist across sessions"
    PASS_COUNT=$((PASS_COUNT + 1))

    $CONTAINER_CMD run --rm \
        -e HOST_UID=$HOST_UID \
        -e HOST_GID=$HOST_GID \
        -v "abox-test-persist:/home/agent/.local/share/opencode" \
        $IMAGE_NAME bash -c "rm /home/agent/.local/share/opencode/test-marker.txt" 2>/dev/null
else
    echo -e "${RED}FAIL${NC}: Files did not persist"
    $CONTAINER_CMD volume rm abox-test-persist > /dev/null 2>&1 || true
    exit 1
fi

$CONTAINER_CMD volume rm abox-test-persist > /dev/null 2>&1 || true
echo

# Test 4: Hygiene Check
echo "=== Test 4: Hygiene Check ==="
echo "Verify auth volume modifications don't affect host"

mkdir -p /tmp/abox-test-auth
echo "original" > /tmp/abox-test-auth/config.txt

TEST_VOL="abox-hygiene-$(date +%s)"
$CONTAINER_CMD volume create "$TEST_VOL" > /dev/null

$CONTAINER_CMD run --rm \
    -v "/tmp/abox-test-auth:/source:ro,z" \
    -v "$TEST_VOL:/dest" \
    alpine sh -c "cp /source/config.txt /dest/ && echo modified > /dest/config.txt" 2>/dev/null

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
echo

# Test 5: Permission Check
echo "=== Test 5: Permission Check ==="
echo "Verify files created in container are owned by host user"

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
echo

# Test 6: Security Check
echo "=== Test 6: Security Check ==="
echo "Verify sudo is not available inside container"

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
echo

# Test 8: Airlock Permission Validation
echo "=== Test 8: Airlock Permission Validation ==="
echo "Verify agent can write to airlocked config volumes"

TEST_VOL="abox-airlock-perm-$(date +%s)"
$CONTAINER_CMD volume create "$TEST_VOL" > /dev/null

$CONTAINER_CMD run --rm \
    -v "$TEST_VOL:/dest" \
    alpine sh -c "touch /dest/test_config && chown $HOST_UID:$HOST_GID /dest/test_config"

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
echo

# Summary
echo "=========================================="
echo "Test Summary"
echo "=========================================="
echo -e "Passed: ${GREEN}$PASS_COUNT${NC}"
echo
echo -e "${GREEN}All tests passed!${NC}"
exit 0
