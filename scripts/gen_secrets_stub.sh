#!/usr/bin/env bash
# 生成 secrets stub：用空密钥替换本地开发密钥，避免泄露到构建产物/发布二进制。
# ci.yml 与 release.yml 共用，避免重复维护。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cat > "${ROOT}/backend/internal/domain/secrets.go" <<'EOF'
package domain

func defaultAiApiKey() string    { return "" }
func defaultPan115Cookie() string { return "" }
EOF