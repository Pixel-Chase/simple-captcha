#!/usr/bin/env sh
# 本地开发启动脚本
#
# 变量优先级（高 → 低）：
#   1. 命令行 / 当前 shell 已有的环境变量，例如 PORT=4000 ./run-dev.sh
#   2. .env               个人或生产覆盖项，不入库
#   3. .env.development   仓库内的开发默认值（Redis 本机、管理后台开启、admin / demo123）
# 文件格式：每行 KEY=VALUE，# 开头为注释；含空格或 $ 的值请用单引号包裹。
set -eu
cd "$(dirname "$0")"

# 只导出尚未设置的变量：先加载的文件优先，而已存在于环境中的值永远不被覆盖
load_env_file() {
  file="$1"
  [ -f "$file" ] || return 0
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in ''|'#'*) continue ;; esac
    key=${line%%=*}
    case "$key" in ''|*[!A-Za-z0-9_]*) continue ;; esac
    if eval "[ -z \"\${$key+x}\" ]"; then
      eval "export $line"
    fi
  done < "$file"
  echo "已加载 $file"
}

load_env_file .env
load_env_file .env.development

# Go 不在 PATH 时尝试默认安装位置
command -v go >/dev/null 2>&1 || PATH="$PATH:/usr/local/go/bin"
if ! command -v go >/dev/null 2>&1; then
  echo "未找到 go 命令，请安装 Go 1.23+ 或把它加入 PATH" >&2
  exit 1
fi

echo "启动后端: PORT=${PORT:-8080} GIN_MODE=${GIN_MODE:-release} REDIS=${REDIS_HOST:-localhost}:${REDIS_PORT:-6379} ADMIN_ENABLED=${ADMIN_ENABLED:-false}"
exec go run . "$@"
