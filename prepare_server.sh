#!/usr/bin/env bash
# Подготовка свежего сервера под k3s: пользователь с sudo и вашим ssh-ключом,
# запрет входа по паролю, обновление пакетов, sysctl для агентов логов,
# часовой пояс. Заходит по SSH под BOOTSTRAP_USER (обычно root), серверная
# часть лежит в lib/remote_prepare_server.sh и работает в POSIX sh.
#
#   ./prepare_server.sh [--env-file FILE]      подготовить сервер по SSH
#   ./prepare_server.sh --print-cloud-init     напечатать user-data для cloud-init
set -euo pipefail
. "$(dirname "$0")/lib/common.sh"

usage() { sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'; }

PRINT_CLOUD_INIT=false
parse_env_file_arg "$@"
for arg in ${REST_ARGS[@]+"${REST_ARGS[@]}"}; do
  case "$arg" in
    --print-cloud-init) PRINT_CLOUD_INIT=true ;;
    -h|--help) usage; exit 0 ;;
    *) die "неизвестный аргумент: $arg" ;;
  esac
done

load_env "$ENV_FILE"
require_vars SSH_USER
SSH_PUBKEY=${SSH_PUBKEY:-${SSH_KEY:+$SSH_KEY.pub}}
[ -n "$SSH_PUBKEY" ] || die "задайте SSH_PUBKEY (путь к публичному ключу)"
pubkey_file=$(expand_path "$SSH_PUBKEY")
[ -f "$pubkey_file" ] || die "публичный ключ не найден: $pubkey_file"
SSH_PUBKEY_CONTENT=$(head -n1 "$pubkey_file")
case "$SSH_PUBKEY_CONTENT" in
  ssh-*|ecdsa-*|sk-*) ;;
  *) die "в $pubkey_file не похоже на публичный ключ ssh" ;;
esac
SERVER_TIMEZONE=${SERVER_TIMEZONE:-}

if [ "$PRINT_CLOUD_INIT" = true ]; then
  sed -e "s|__SSH_USER__|$SSH_USER|g" \
      -e "s|__SSH_PUBKEY__|$SSH_PUBKEY_CONTENT|g" \
      -e "s|__TIMEZONE__|$SERVER_TIMEZONE|g" \
      "$ROOT_DIR/cloud-init.yaml.tpl" \
    | if [ -z "$SERVER_TIMEZONE" ]; then grep -v '^timezone:'; else cat; fi
  exit 0
fi

require_vars SSH_HOST
BOOTSTRAP_USER=${BOOTSTRAP_USER:-root}
PREPARE_UPGRADE=${PREPARE_UPGRADE:-true}
PREPARE_DISABLE_PASSWORD=${PREPARE_DISABLE_PASSWORD:-true}

log "подготавливаю $SSH_HOST (вход как $BOOTSTRAP_USER, пользователь $SSH_USER)"
{
  remote_vars SSH_USER SSH_PUBKEY_CONTENT PREPARE_UPGRADE PREPARE_DISABLE_PASSWORD SERVER_TIMEZONE
  cat "$LIB_DIR/remote_prepare_server.sh"
} | remote_sh "$BOOTSTRAP_USER" root

log "проверяю вход как $SSH_USER по ключу и sudo без пароля"
remote_cmd "$SSH_USER" 'sudo -n true' || die "вход как $SSH_USER или sudo не работает"
log "готово: ssh -p ${SSH_PORT:-22} $SSH_USER@$SSH_HOST"
