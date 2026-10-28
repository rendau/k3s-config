#!/usr/bin/env bash
# Установка k3s официальным установщиком (https://get.k3s.io) по SSH.
# Роль задаёт K3S_ROLE: server (master) или agent (worker). Для server
# пишется /etc/rancher/k3s/config.yaml с tls-san и выдаётся sudo на kubectl
# пользователю SSH_USER. Для agent токен кластера читается с master по SSH
# (K3S_SERVER_SSH_HOST) или берётся из K3S_TOKEN. Серверная часть лежит в
# lib/remote_install_k3s.sh и работает в POSIX sh.
#
#   ./install_k3s.sh [--env-file FILE]
set -euo pipefail
. "$(dirname "$0")/lib/common.sh"

usage() { sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'; }

parse_env_file_arg "$@"
for arg in ${REST_ARGS[@]+"${REST_ARGS[@]}"}; do
  case "$arg" in
    -h|--help) usage; exit 0 ;;
    *) die "неизвестный аргумент: $arg" ;;
  esac
done

load_env "$ENV_FILE"
require_vars SSH_HOST SSH_USER K3S_ROLE
K3S_VERSION=${K3S_VERSION:-}
K3S_TLS_SAN=${K3S_TLS_SAN:-}
K3S_EXTRA_ARGS=${K3S_EXTRA_ARGS:-}
K3S_INSTALL_ENV=${K3S_INSTALL_ENV:-}
K3S_SERVER_URL=${K3S_SERVER_URL:-}
K3S_TOKEN=${K3S_TOKEN:-}

case "$K3S_ROLE" in
  server)
    K3S_TLS_SAN=${K3S_TLS_SAN:-$SSH_HOST}
    ;;
  agent)
    K3S_SERVER_SSH_HOST=${K3S_SERVER_SSH_HOST:-}
    [ -n "$K3S_SERVER_URL" ] || [ -n "$K3S_SERVER_SSH_HOST" ] \
      || die "для agent задайте K3S_SERVER_SSH_HOST или K3S_SERVER_URL"
    K3S_SERVER_URL=${K3S_SERVER_URL:-https://$K3S_SERVER_SSH_HOST:6443}
    if [ -z "$K3S_TOKEN" ]; then
      [ -n "$K3S_SERVER_SSH_HOST" ] || die "для agent задайте K3S_TOKEN или K3S_SERVER_SSH_HOST"
      log "читаю токен кластера с master $K3S_SERVER_SSH_HOST"
      K3S_TOKEN=$(
        SSH_HOST=$K3S_SERVER_SSH_HOST SSH_PORT=${K3S_SERVER_SSH_PORT:-${SSH_PORT:-22}} \
          remote_cmd "${K3S_SERVER_SSH_USER:-$SSH_USER}" 'sudo -n cat /var/lib/rancher/k3s/server/node-token'
      )
      [ -n "$K3S_TOKEN" ] || die "не получил токен с master"
    fi
    ;;
  *) die "K3S_ROLE должен быть server или agent, а не '$K3S_ROLE'" ;;
esac

log "ставлю k3s ($K3S_ROLE) на $SSH_HOST"
{
  remote_vars K3S_ROLE SSH_USER K3S_VERSION K3S_TLS_SAN K3S_EXTRA_ARGS K3S_INSTALL_ENV K3S_SERVER_URL K3S_TOKEN
  cat "$LIB_DIR/remote_install_k3s.sh"
} | remote_sh "$SSH_USER" root
log "готово"
