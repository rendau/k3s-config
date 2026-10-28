#!/usr/bin/env bash
# Доступ к k3s через ServiceAccount без копирования админского k3s.yaml.
# По SSH под SSH_USER выполняет kubectl на сервере: заводит учётку
# (ServiceAccount) с ролью CLUSTER_ROLE, получает её токен и корневой
# сертификат кластера, а локально записывает кластер, пользователя и
# контекст в LOCAL_KUBECONFIG. Повторный запуск безопасен. Серверная часть
# лежит в lib/remote_setup_sa.sh и работает в POSIX sh.
#
#   ./setup_sa_access.sh [--env-file FILE]
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
require_vars SSH_HOST SSH_USER SA_NAME CONTEXT_NAME
API_URL=${API_URL:-https://$SSH_HOST:6443}
SA_NAMESPACE=${SA_NAMESPACE:-kube-system}
CLUSTER_ROLE=${CLUSTER_ROLE:-cluster-admin}
SA_TOKEN_TTL=${SA_TOKEN_TTL:-}
LOCAL_KUBECONFIG=$(expand_path "${LOCAL_KUBECONFIG:-~/.kube/config}")
SET_CURRENT_CONTEXT=${SET_CURRENT_CONTEXT:-false}
CLUSTER_NAME=$CONTEXT_NAME
USER_NAME=$SA_NAME@$CONTEXT_NAME

command -v kubectl >/dev/null 2>&1 || die "локально нужен kubectl"

b64decode() {
  if base64 -d </dev/null >/dev/null 2>&1; then base64 -d; else base64 -D; fi
}

log "настраиваю учётку $SA_NAMESPACE/$SA_NAME на $SSH_HOST"
out=$(
  {
    remote_vars SA_NAME SA_NAMESPACE CLUSTER_ROLE SA_TOKEN_TTL
    cat "$LIB_DIR/remote_setup_sa.sh"
  } | remote_sh "$SSH_USER"
)
ca_b64=$(printf '%s\n' "$out" | sed -n 's/^CA=//p')
token=$(printf '%s\n' "$out" | sed -n 's/^TOKEN=//p')
[ -n "$ca_b64" ] && [ -n "$token" ] || die "сервер не вернул сертификат и токен"

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT
printf '%s' "$ca_b64" | b64decode > "$tmpdir/ca.crt"
grep -q 'BEGIN CERTIFICATE' "$tmpdir/ca.crt" || die "сертификат пришёл в неожиданном виде"

log "записываю контекст $CONTEXT_NAME в $LOCAL_KUBECONFIG"
mkdir -p "$(dirname "$LOCAL_KUBECONFIG")"
if [ ! -f "$LOCAL_KUBECONFIG" ]; then
  (umask 077; : > "$LOCAL_KUBECONFIG")
fi
kc() { kubectl --kubeconfig "$LOCAL_KUBECONFIG" "$@"; }
kc config set-cluster "$CLUSTER_NAME" --server="$API_URL" \
  --certificate-authority="$tmpdir/ca.crt" --embed-certs=true >/dev/null
kc config set-credentials "$USER_NAME" --token="$token" >/dev/null
kc config set-context "$CONTEXT_NAME" --cluster="$CLUSTER_NAME" --user="$USER_NAME" >/dev/null
if [ "$SET_CURRENT_CONTEXT" = true ]; then
  kc config use-context "$CONTEXT_NAME" >/dev/null
fi

log "проверяю доступ"
kc --context "$CONTEXT_NAME" get nodes >&2
log "готово: kubectl --context $CONTEXT_NAME get pods -A"
