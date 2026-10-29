#!/usr/bin/env bash
# Сквозной тест k3sctl на docker-стенде (см. CLAUDE.md, «Тестовый стенд»).
# Собирает бинарник, поднимает контейнер-«сервер», прогоняет k3sctl prepare,
# install и access, проверяет результат и убирает за собой. KEEP=1 оставляет
# контейнер и временный каталог после прогона.
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/.." && pwd)

IMAGE=k3s-sa-test
CONTAINER=k3s-sa-test
SSH_PORT=2222
API_PORT=16443
K3S_VERSION=${K3S_VERSION:-v1.33.5+k3s1}
TMP=$(mktemp -d)

cleanup() {
  rc=$?
  if [ "${KEEP:-0}" = 1 ]; then
    echo "KEEP=1: контейнер $CONTAINER оставлен, ключи и kubeconfig в $TMP"
  else
    docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
    rm -rf "$TMP"
  fi
  exit "$rc"
}
trap cleanup EXIT

pass() { printf 'OK   %s\n' "$*"; }
fail() { printf 'FAIL %s\n' "$*" >&2; exit 1; }
# check ОПИСАНИЕ КОМАНДА... — команда должна завершиться успешно
check() {
  local desc=$1
  shift
  if "$@" >/dev/null 2>&1; then pass "$desc"; else fail "$desc"; fi
}
tssh() {
  ssh -p "$SSH_PORT" -i "$TMP/id_test" -o BatchMode=yes -o StrictHostKeyChecking=no \
      -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR "$@"
}
kc() { kubectl --kubeconfig "$TMP/kubeconfig" "$@"; }

command -v docker >/dev/null || fail "нужен docker"
command -v kubectl >/dev/null || fail "нужен kubectl"
command -v go >/dev/null || fail "нужен go"
docker info >/dev/null 2>&1 || fail "docker не запущен"

echo "--- сборка k3sctl ---"
(cd "$ROOT" && CGO_ENABLED=0 go build -o "$TMP/k3sctl" ./cmd/k3sctl)
K3SCTL=$TMP/k3sctl

echo "--- k3sctl init / list ---"
mkdir -p "$TMP/work"
check "init создаёт файл сервера в servers/" bash -c 'cd "$1" && "$2" init demo' _ "$TMP/work" "$K3SCTL"
check "файл servers/demo.env появился" test -f "$TMP/work/servers/demo.env"
check "list показывает сервер" test "$(cd "$TMP/work" && "$K3SCTL" list)" = demo
check "повторный init не перезаписывает" test "$(cd "$TMP/work" && "$K3SCTL" init demo >/dev/null 2>&1; echo $?)" = 1

echo "--- сборка образа $IMAGE (k3s $K3S_VERSION) ---"
docker build -q --build-arg "K3S_VERSION=$K3S_VERSION" -t "$IMAGE" "$HERE" >/dev/null

ssh-keygen -q -t ed25519 -N '' -C k3s-sa-test -f "$TMP/id_test"
docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
docker run -d --name "$CONTAINER" --privileged --tmpfs /run --tmpfs /var/run \
  -p "127.0.0.1:$SSH_PORT:22" -p "127.0.0.1:$API_PORT:6443" \
  -e "ROOT_AUTHORIZED_KEY=$(cat "$TMP/id_test.pub")" "$IMAGE" >/dev/null

cat > "$TMP/test.env" <<ENV
SSH_HOST=127.0.0.1
SSH_PORT=$SSH_PORT
SSH_USER=user
SSH_KEY=$TMP/id_test
SSH_PUBKEY=$TMP/id_test.pub
SSH_EXTRA_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"
BOOTSTRAP_USER=root
SERVER_TIMEZONE=Asia/Almaty
K3S_ROLE=server
K3S_TLS_SAN=127.0.0.1,localhost
K3S_EXTRA_ARGS=--disable-agent
K3S_INSTALL_ENV="INSTALL_K3S_SKIP_DOWNLOAD=true INSTALL_K3S_SKIP_START=true"
API_URL=https://127.0.0.1:$API_PORT
SA_NAME=tester-admin
SA_NAMESPACE=kube-system
CLUSTER_ROLE=cluster-admin
CONTEXT_NAME=k3s-sa-test
LOCAL_KUBECONFIG=$TMP/kubeconfig
ENV

echo "--- жду sshd ---"
for _ in $(seq 1 30); do
  tssh root@127.0.0.1 true 2>/dev/null && break
  sleep 1
done
tssh root@127.0.0.1 true || fail "sshd в контейнере не поднялся"

echo "--- k3sctl prepare ---"
"$K3SCTL" prepare "$TMP/test.env"
check "пользователь user входит по ключу и имеет sudo без пароля" \
  test "$(tssh user@127.0.0.1 'sudo -n id -u')" = 0
check "bash-completion установлен" tssh user@127.0.0.1 'test -f /usr/share/bash-completion/bash_completion'
check "sysctl для inotify записан" tssh root@127.0.0.1 'grep -q max_user_watches /etc/sysctl.d/90-k3s.conf'
check "часовой пояс Asia/Almaty" tssh root@127.0.0.1 'readlink /etc/localtime | grep -q Asia/Almaty'
out=$(ssh -p "$SSH_PORT" -o PubkeyAuthentication=no -o BatchMode=yes -o StrictHostKeyChecking=no \
        -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR root@127.0.0.1 true 2>&1 || true)
case "$out" in
  *"Permission denied (publickey)"*) pass "вход по паролю отключён (sshd предлагает только publickey)" ;;
  *) fail "вход по паролю не отключён, sshd ответил: $out" ;;
esac
check "повторный k3sctl prepare проходит" "$K3SCTL" prepare "$TMP/test.env"
check "k3sctl cloud-init записывает файл рядом с настройками" "$K3SCTL" cloud-init "$TMP/test.env"
check "файл test.cloud-init.yaml появился" test -f "$TMP/test.cloud-init.yaml"
cloud_init=$(cat "$TMP/test.cloud-init.yaml")
check "cloud-init начинается с #cloud-config" test "$(head -1 "$TMP/test.cloud-init.yaml")" = '#cloud-config'
check "cloud-init содержит пользователя" grep -q '^  - name: user$' <<<"$cloud_init"
check "cloud-init подставил ключ" grep -q 'ssh-ed25519 .* k3s-sa-test$' <<<"$cloud_init"
check "cloud-init подставил часовой пояс" grep -q '^timezone: Asia/Almaty$' <<<"$cloud_init"

echo "--- k3sctl install ---"
"$K3SCTL" install "$TMP/test.env"
check "config.yaml с tls-san записан" tssh root@127.0.0.1 'grep -q "^  - localhost" /etc/rancher/k3s/config.yaml'
check "установщик создал службу k3s" tssh root@127.0.0.1 'test -f /etc/init.d/k3s'
check "kubectl установлен" tssh root@127.0.0.1 'test -L /usr/local/bin/kubectl'
check "sudo на kubectl для user выдан" tssh root@127.0.0.1 'test -f /etc/sudoers.d/k3s-kubectl'

echo "--- запуск k3s в контейнере ---"
tssh root@127.0.0.1 'nohup k3s server --disable-agent >/var/log/k3s.log 2>&1 </dev/null &'
for _ in $(seq 1 90); do
  tssh user@127.0.0.1 'sudo -n kubectl get --raw /readyz' >/dev/null 2>&1 && break
  sleep 2
done
check "k3s отвечает, sudo -n kubectl работает для user" tssh user@127.0.0.1 'sudo -n kubectl get --raw /readyz'

echo "--- k3sctl access ---"
"$K3SCTL" access "$TMP/test.env"
check "в кластер входим учёткой tester-admin" \
  test "$(kc --context k3s-sa-test auth whoami -o jsonpath='{.status.userInfo.username}')" = system:serviceaccount:kube-system:tester-admin
check "cluster-admin: список namespace читается" kc --context k3s-sa-test get ns
check "kubeconfig создан с правами 600" test "$(stat -f %Lp "$TMP/kubeconfig" 2>/dev/null || stat -c %a "$TMP/kubeconfig")" = 600
check "повторный k3sctl access проходит" "$K3SCTL" access "$TMP/test.env"
check "после повторного запуска доступ остался" kc --context k3s-sa-test get ns

sed -e 's/^CONTEXT_NAME=.*/CONTEXT_NAME=k3s-sa-test-ttl/' -e 's/^SA_NAME=.*/SA_NAME=tester-ttl/' "$TMP/test.env" > "$TMP/ttl.env"
echo 'SA_TOKEN_TTL=1h' >> "$TMP/ttl.env"
check "k3sctl access с SA_TOKEN_TTL проходит" "$K3SCTL" access "$TMP/ttl.env"
check "токен со сроком работает" kc --context k3s-sa-test-ttl get ns
check "первый контекст не пострадал" kc --context k3s-sa-test get ns
check "текущий контекст не переключён (SET_CURRENT_CONTEXT=false)" \
  test "$(kc config current-context 2>/dev/null || true)" = ""

echo
echo "ВСЕ ТЕСТЫ ПРОШЛИ"
