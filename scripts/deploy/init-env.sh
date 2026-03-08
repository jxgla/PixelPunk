#!/usr/bin/env sh
set -eu

# 初始化 .env：自动生成强随机密码（不覆盖已有值）

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
ENV_FILE="${ROOT_DIR}/.env"
EXAMPLE_FILE="${ROOT_DIR}/.env.example"

if [ ! -f "$EXAMPLE_FILE" ]; then
  echo "[ERROR] 未找到示例文件: $EXAMPLE_FILE"
  exit 1
fi

if [ ! -f "$ENV_FILE" ]; then
  cp "$EXAMPLE_FILE" "$ENV_FILE"
  echo "[INFO] 已创建 .env"
fi

gen_secret() {
  # 48 bytes => 64 chars base64url-ish after filtering
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 48 | tr -d '=+/' | cut -c1-48
    return
  fi

  # fallback
  dd if=/dev/urandom bs=48 count=1 2>/dev/null | base64 | tr -d '=+/' | cut -c1-48
}

upsert_kv() {
  key="$1"
  value="$2"

  if grep -Eq "^${key}=" "$ENV_FILE"; then
    current="$(grep -E "^${key}=" "$ENV_FILE" | tail -n 1 | cut -d '=' -f2-)"
    if [ -n "$current" ] && ! printf "%s" "$current" | grep -q "change_me"; then
      echo "[SKIP] ${key} 已存在，保持不变"
      return
    fi

    # 兼容 busybox/macOS sed
    tmp_file="${ENV_FILE}.tmp.$$"
    awk -v k="$key" -v v="$value" -F= 'BEGIN{OFS="="} { if ($1==k) {$0=k"="v} print }' "$ENV_FILE" > "$tmp_file"
    mv "$tmp_file" "$ENV_FILE"
    echo "[OK] 已更新 ${key}"
    return
  fi

  printf "\n%s=%s\n" "$key" "$value" >> "$ENV_FILE"
  echo "[OK] 已写入 ${key}"
}

MYSQL_ROOT_PASSWORD="$(gen_secret)"
MYSQL_PASSWORD="$(gen_secret)"
REDIS_PASSWORD="$(gen_secret)"

upsert_kv "MYSQL_ROOT_PASSWORD" "$MYSQL_ROOT_PASSWORD"
upsert_kv "MYSQL_PASSWORD" "$MYSQL_PASSWORD"
upsert_kv "REDIS_PASSWORD" "$REDIS_PASSWORD"

echo ""
echo "[DONE] .env 已初始化完成"
echo "[TIP] 请妥善备份 .env，勿提交到 Git"
