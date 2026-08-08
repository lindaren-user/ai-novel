#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
DEPLOY_DIR="$SCRIPT_DIR"
RUNTIME_DIR="$DEPLOY_DIR/runtime"
LOG_DIR="$DEPLOY_DIR/data/logs"
ENV_FILE="$DEPLOY_DIR/.env"
CONFIG_FILE="$RUNTIME_DIR/config.prod.yaml"
REDIS_ACL_FILE="$RUNTIME_DIR/redis/users.acl.conf"
POSTGRES_VOLUME="ai_novel_postgres_data"
REDIS_VOLUME="ai_novel_redis_data"

random_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 24
    return
  fi
  date +%s | sha256sum | cut -d' ' -f1 | cut -c1-48
}

ensure_dirs() {
  mkdir -p "$RUNTIME_DIR/redis" "$LOG_DIR"
}

ensure_volumes() {
  docker volume inspect "$POSTGRES_VOLUME" >/dev/null 2>&1 || docker volume create "$POSTGRES_VOLUME" >/dev/null
  docker volume inspect "$REDIS_VOLUME" >/dev/null 2>&1 || docker volume create "$REDIS_VOLUME" >/dev/null
}

init_env_file() {
  if [ -f "$ENV_FILE" ]; then
    return
  fi

  cp "$DEPLOY_DIR/.env.example" "$ENV_FILE"

  POSTGRES_PASSWORD=$(random_secret)
  REDIS_PASSWORD=$(random_secret)
  JWT_SECRET=$(random_secret)

  sed -i \
    -e "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=$POSTGRES_PASSWORD/" \
    -e "s/^REDIS_PASSWORD=.*/REDIS_PASSWORD=$REDIS_PASSWORD/" \
    -e "s/^JWT_SECRET=.*/JWT_SECRET=$JWT_SECRET/" \
    "$ENV_FILE"
}

load_env() {
  set -a
  . "$ENV_FILE"
  set +a
}

write_redis_acl() {
  cat > "$REDIS_ACL_FILE" <<EOF
user default off
user ${REDIS_USERNAME} on >${REDIS_PASSWORD} ~* &* +@all
EOF
}

write_backend_config() {
  cat > "$CONFIG_FILE" <<EOF
env: "prod"

http:
  host: "0.0.0.0"
  port: 8080

pprof:
  enabled: ${PPROF_ENABLED:-false}
  addr: "${PPROF_ADDR:-0.0.0.0:6060}"

postgres:
  host: "postgres"
  port: 5432
  user: "${POSTGRES_USER}"
  password: "${POSTGRES_PASSWORD}"
  database: "${POSTGRES_DB}"
  sslMode: "disable"
  maxOpenConns: 20
  maxIdleConns: 5
  connMaxLifetime: "30m"

redis:
  host: "redis"
  port: 6379
  username: "${REDIS_USERNAME}"
  password: "${REDIS_PASSWORD}"
  db: 0
  dialTimeout: "5s"
  readTimeout: "3s"
  writeTimeout: "3s"
  poolSize: 10

auth:
  secret: "${JWT_SECRET}"
  turnstile:
    siteKey: "${TURNSTILE_SITE_KEY}"
    secretKey: "${TURNSTILE_SECRET_KEY}"

storage:
  s3:
    endpoint: "${S3_ENDPOINT}"
    region: "${S3_REGION}"
    bucket: "${S3_BUCKET}"
    accessKey: "${S3_ACCESS_KEY}"
    secretKey: "${S3_SECRET_KEY}"
    publicBaseUrl: "${S3_PUBLIC_BASE_URL}"
    prefix: "${S3_PREFIX}"

mail:
  provider: "${MAIL_PROVIDER}"

  smtp:
    host: "${SMTP_HOST}"
    port: ${SMTP_PORT}
    username: "${SMTP_USERNAME}"
    password: "${SMTP_PASSWORD}"

  resend:
    api_key: "${RESEND_API_KEY}"
    from: "${RESEND_FROM}"
EOF
}

main() {
  ensure_dirs
  init_env_file
  load_env
  ensure_volumes
  write_redis_acl
  write_backend_config

  echo "Deployment files prepared."
  echo "Edit $ENV_FILE if you still need to fill S3, SMTP, Resend, or Turnstile values."

  docker compose --env-file "$ENV_FILE" -f "$DEPLOY_DIR/docker-compose.prod.yml" up -d --build

  echo "Deployment started."
  echo "Check status with:"
  echo "  docker compose --env-file $ENV_FILE -f $DEPLOY_DIR/docker-compose.prod.yml ps"
}

main "$@"
