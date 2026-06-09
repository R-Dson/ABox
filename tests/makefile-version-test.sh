#!/usr/bin/env bash
set -euo pipefail

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

go_build=$(make -n go-build CLI_VERSION=test-cli ABX_EDITOR=opencode)
[[ "$go_build" == *"-X main.version=test-cli"* ]] || fail "go-build did not use CLI_VERSION: $go_build"
[[ "$go_build" != *"-X main.version=1.16.2"* ]] || fail "go-build used editor version: $go_build"

image_build=$(make -n build EDITOR_VERSION=test-editor ABX_EDITOR=opencode)
[[ "$image_build" == *'--build-arg VERSION="test-editor"'* ]] || fail "image build did not use EDITOR_VERSION: $image_build"

echo "PASS: Makefile separates CLI and editor versions"
