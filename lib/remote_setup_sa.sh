# Серверная часть setup_sa_access.sh. Выполняется от SSH_USER (kubectl через
# sudo -n), POSIX sh. Переменные ставит в начало локальный скрипт: SA_NAME,
# SA_NAMESPACE, CLUSTER_ROLE, SA_TOKEN_TTL.
# В stdout печатает ровно две строки: CA=<base64 PEM> и TOKEN=<токен>;
# всё остальное уходит в stderr.
set -eu

log()  { printf '  [сервер] %s\n' "$*" >&2; }
die()  { printf '  [сервер] ОШИБКА: %s\n' "$*" >&2; exit 1; }

: "${SA_NAME:?}" "${SA_NAMESPACE:?}" "${CLUSTER_ROLE:?}"
SA_TOKEN_TTL=${SA_TOKEN_TTL:-}

if [ "$(id -u)" = 0 ]; then
  k() { kubectl "$@"; }
else
  k() { sudo -n kubectl "$@"; }
fi
k version --client >/dev/null 2>&1 || die "kubectl недоступен (нужен sudo -n на kubectl, его даёт install_k3s.sh)"

# --- 1. учётка и права ---
log "создаю ServiceAccount $SA_NAMESPACE/$SA_NAME с ролью $CLUSTER_ROLE"
k apply -f - >&2 <<MANIFEST
apiVersion: v1
kind: ServiceAccount
metadata:
  name: $SA_NAME
  namespace: $SA_NAMESPACE
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: $SA_NAME-$CLUSTER_ROLE
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: $CLUSTER_ROLE
subjects:
  - kind: ServiceAccount
    name: $SA_NAME
    namespace: $SA_NAMESPACE
MANIFEST

# --- 2. токен ---
if [ -z "$SA_TOKEN_TTL" ]; then
  log "выпускаю бессрочный токен (секрет $SA_NAME-token)"
  k apply -f - >&2 <<MANIFEST
apiVersion: v1
kind: Secret
metadata:
  name: $SA_NAME-token
  namespace: $SA_NAMESPACE
  annotations:
    kubernetes.io/service-account.name: $SA_NAME
type: kubernetes.io/service-account-token
MANIFEST
  i=0
  token_b64=""
  while [ -z "$token_b64" ]; do
    token_b64=$(k -n "$SA_NAMESPACE" get secret "$SA_NAME-token" -o jsonpath='{.data.token}' 2>/dev/null || true)
    [ -n "$token_b64" ] && break
    i=$((i + 1))
    [ "$i" -lt 30 ] || die "токен в секрете $SA_NAME-token так и не появился"
    sleep 1
  done
  TOKEN=$(printf '%s' "$token_b64" | base64 -d)
else
  log "выпускаю токен на срок $SA_TOKEN_TTL"
  TOKEN=$(k -n "$SA_NAMESPACE" create token "$SA_NAME" --duration "$SA_TOKEN_TTL")
fi
[ -n "$TOKEN" ] || die "пустой токен"

# --- 3. корневой сертификат кластера (не секретный) ---
# Конфигмап kube-root-ca.crt создаёт контроллер, сразу после старта кластера
# его может ещё не быть, поэтому ждём до 30 с.
CA=""
i=0
while [ -z "$CA" ]; do
  CA=$(k -n kube-system get configmap kube-root-ca.crt -o jsonpath='{.data.ca\.crt}' 2>/dev/null | base64 | tr -d '\n' || true)
  [ -n "$CA" ] && break
  i=$((i + 1))
  [ "$i" -lt 30 ] || die "конфигмап kube-system/kube-root-ca.crt так и не появился"
  sleep 1
done

printf 'CA=%s\nTOKEN=%s\n' "$CA" "$TOKEN"
