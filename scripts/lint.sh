#!/usr/bin/env bash
#
# lint.sh - 本地运行 golangci-lint，与 CI 保持一致
#
# Usage:  ./scripts/lint.sh          # lint 整个后端
#         ./scripts/lint.sh fix      # lint + 自动修复（gofmt 等）
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BACKEND_DIR="$(cd "$SCRIPT_DIR/../backend" && pwd)"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*"; }

# 查找 golangci-lint
find_lint() {
    if command -v golangci-lint &>/dev/null; then
        echo "golangci-lint"
        return
    fi
    if [ -x /opt/homebrew/bin/golangci-lint ]; then
        echo "/opt/homebrew/bin/golangci-lint"
        return
    fi
    return 1
}

LINT_BIN=$(find_lint) || {
    error "golangci-lint 未安装"
    echo ""
    echo "  安装方式："
    echo "    brew install golangci-lint"
    echo ""
    exit 1
}

LINT_VERSION=$("$LINT_BIN" version 2>&1 | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' || echo "unknown")
info "golangci-lint $LINT_VERSION ($LINT_BIN)"

cd "$BACKEND_DIR"

# 先跑 go build 确保编译通过
info "检查编译..."
if ! go build ./... 2>&1; then
    error "编译失败，请先修复编译错误"
    exit 1
fi

# 跑测试
info "运行测试..."
if ! go test ./... 2>&1; then
    error "测试失败"
    exit 1
fi

# 跑 lint
LINT_ARGS="--timeout=5m"
if [ "${1:-}" = "fix" ]; then
    LINT_ARGS="$LINT_ARGS --fix"
    info "运行 lint（自动修复模式）..."
else
    info "运行 lint..."
fi

if "$LINT_BIN" run $LINT_ARGS ./... 2>&1; then
    echo ""
    info "全部通过 ✓"
else
    EXIT_CODE=$?
    echo ""
    error "lint 发现问题，请修复后再提交"
    echo ""
    echo "  提示：运行 ./scripts/lint.sh fix 可自动修复格式问题"
    echo ""
    exit $EXIT_CODE
fi
