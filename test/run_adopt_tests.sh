#!/usr/bin/env bash
# Сквозной тест на настроенном руками кластере (см. CLAUDE.md, «Тестовый стенд»):
# две ноды Ubuntu с systemd, k3s поставлен без k3sctl, master с etcd, у user
# ключ есть, а sudo спрашивает пароль. Дальше весь путь: init, prepare, install,
# access, upgrade. KEEP=1 оставляет контейнеры и временный каталог.
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/.." && pwd)

IMAGE=k3s-adopt-test
NET=k3s-adopt-net
MASTER=k3s-adopt-master
WORKER=k3s-adopt-worker
MASTER_SSH=2223
WORKER_SSH=2224
API_PORT=16444
K3S_VERSION=${K3S_VERSION:-v1.33.5+k3s1}
K3S_NEXT_VERSION=${K3S_NEXT_VERSION:-v1.34.1+k3s1}
TMP=$(mktemp -d)

cleanup() {
  rc=$?
  if [ "${KEEP:-0}" = 1 ]; then
    echo "KEEP=1: контейнеры $MASTER и $WORKER оставлены, файлы в $TMP"
  else
    docker rm -f -v "$MASTER" "$WORKER" >/dev/null 2>&1 || true
    docker network rm "$NET" >/dev/null 2>&1 || true
    rm -rf "$TMP"
  fi
  exit "$rc"
}
trap cleanup EXIT

pass() { printf 'OK   %s\n' "$*"; }
fail() { printf 'FAIL %s\n' "$*" >&2; exit 1; }
check() {
  local desc=$1
  shift
  if "$@" >/dev/null 2>&1; then pass "$desc"; else fail "$desc"; fi
}
# refuses ОПИСАНИЕ КОМАНДА... — команда должна завершиться с кодом 1
refuses() {
  local desc=$1
  shift
  local rc=0
  "$@" >/dev/null 2>&1 </dev/null || rc=$?
  if [ "$rc" = 1 ]; then pass "$desc"; else fail "$desc (код $rc)"; fi
}
on() { local c=$1; shift; docker exec "$c" sh -c "$*"; }
kube() { on "$MASTER" k3s kubectl "$@"; }
kc() { kubectl --kubeconfig "$TMP/kubeconfig" --context master "$@"; }
k() { (cd "$TMP/work" && "$K3SCTL" "$@"); }
# set_var ФАЙЛ ИМЯ ЗНАЧЕНИЕ — поправить переменную в файле сервера
set_var() {
  sed -e "s|^$2=.*|$2=$3|" "$TMP/work/servers/$1.env" > "$TMP/env.tmp"
  cp "$TMP/env.tmp" "$TMP/work/servers/$1.env"
}
has() { grep -qF -- "$2" "$TMP/work/servers/$1.env"; }
lacks() { ! has "$1" "$2"; }
version() { on "$1" k3s --version | awk 'NR == 1 { print $3 }'; }
sum() { on "$1" "cat $2" | shasum | awk '{ print $1 }'; }
wait_ready() {
  for _ in $(seq 1 90); do
    [ "$(kube get nodes --no-headers 2>/dev/null | awk '$2 == "Ready"' | wc -l | tr -d ' ')" = 2 ] && return 0
    sleep 2
  done
  kube get nodes -o wide >&2 || true
  return 1
}

command -v docker >/dev/null || fail "нужен docker"
command -v kubectl >/dev/null || fail "нужен kubectl"
docker info >/dev/null 2>&1 || fail "docker не запущен"

echo "--- сборка ---"
(cd "$ROOT" && CGO_ENABLED=0 go build -o "$TMP/k3sctl" ./cmd/k3sctl)
K3SCTL=$TMP/k3sctl
docker build -q -t "$IMAGE" "$HERE/ubuntu" >/dev/null

ssh-keygen -q -t ed25519 -N '' -C k3s-adopt -f "$TMP/id_test"
PASSWORD=$(od -An -N12 -tx1 /dev/urandom | tr -d ' \n')

docker rm -f -v "$MASTER" "$WORKER" >/dev/null 2>&1 || true
docker network rm "$NET" >/dev/null 2>&1 || true
docker network create "$NET" >/dev/null

run_node() {
  docker run -d --name "$1" --hostname "$1" --network "$NET" --privileged \
    --tmpfs /run --tmpfs /run/lock --tmpfs /tmp -v /var/lib/rancher \
    -p "127.0.0.1:$2:22" ${3:+-p "127.0.0.1:$3:6443"} "$IMAGE" >/dev/null
  for _ in $(seq 1 30); do
    on "$1" 'systemctl is-active -q ssh.socket || systemctl is-active -q ssh' 2>/dev/null && break
    sleep 1
  done
  on "$1" "install -d -m 700 -o user -g user /home/user/.ssh \
    && echo '$(cat "$TMP/id_test.pub")' > /home/user/.ssh/authorized_keys \
    && chown user:user /home/user/.ssh/authorized_keys && chmod 600 /home/user/.ssh/authorized_keys \
    && echo 'user:$PASSWORD' | chpasswd"
}
echo "--- ноды ---"
run_node "$MASTER" "$MASTER_SSH" "$API_PORT"
run_node "$WORKER" "$WORKER_SSH"

echo "--- k3s руками ---"
on "$MASTER" "mkdir -p /etc/rancher/k3s && printf 'kubelet-arg:\n  - maximum-dead-containers=50\n' > /etc/rancher/k3s/config.yaml"
on "$MASTER" "curl -sfL https://get.k3s.io | INSTALL_K3S_VERSION='$K3S_VERSION' sh -s - server \
  --cluster-init --tls-san 127.0.0.1 --disable traefik --disable servicelb --disable metrics-server \
  --kubelet-arg image-gc-low-threshold=70" >/dev/null
TOKEN=$(on "$MASTER" cat /var/lib/rancher/k3s/server/node-token)
on "$WORKER" "curl -sfL https://get.k3s.io | INSTALL_K3S_VERSION='$K3S_VERSION' \
  K3S_URL='https://$MASTER:6443' K3S_TOKEN='$TOKEN' sh -s - agent \
  --kubelet-arg maximum-dead-containers=100" >/dev/null
check "кластер поднят руками, обе ноды Ready" wait_ready

kube create configmap adopt-marker --from-literal=a=b >/dev/null
kube run canary --image=rancher/mirrored-pause:3.6 --restart=Never \
  --overrides="'{\"spec\":{\"nodeName\":\"$WORKER\"}}'" >/dev/null
for _ in $(seq 1 60); do
  [ "$(kube get pod canary -o 'jsonpath={.status.phase}')" = Running ] && break
  sleep 2
done
canary() { kube get pod canary -o "'jsonpath={.metadata.uid} {.status.containerStatuses[0].containerID} {.status.containerStatuses[0].restartCount}'"; }
CANARY=$(canary)
check "под-свидетель работает на worker" test -n "$(echo "$CANARY" | awk '{ print $2 }')"

mkdir -p "$TMP/work"
export SSH_KEY=$TMP/id_test
export SSH_EXTRA_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"
export LOCAL_KUBECONFIG=$TMP/kubeconfig

echo "--- init: настроенный master ---"
BOOTSTRAP_PASSWORD=$PASSWORD k init master <<<"$(printf '127.0.0.1:%s\nuser\n\n' "$MASTER_SSH")"
check "порт из адреса" has master "SSH_PORT=$MASTER_SSH"
check "роль с ноды" has master "K3S_ROLE=server"
check "версия с ноды" has master "K3S_VERSION=$K3S_VERSION"
check "важные флаги с ноды" has master "K3S_EXTRA_ARGS=--cluster-init --disable traefik --disable servicelb --disable metrics-server"
check "tls-san с ноды" has master "K3S_TLS_SAN=127.0.0.1"
check "ручные настройки kubelet видны в заметке" has master "# С сервера не перенесено: ручные настройки kubelet"
check "и из config.yaml, и из строки запуска" bash -c "grep -q 'maximum-dead-containers=50' '$TMP/work/servers/master.env' && grep -q 'image-gc-low-threshold=70' '$TMP/work/servers/master.env'"
check "часовой пояс стандартный, не с ноды" has master "SERVER_TIMEZONE=Asia/Almaty"
check "лимит логов стандартный" has master "K3S_LOG_MAX_SIZE=10Mi"
check "штатная остановка подов стандартная" has master "K3S_SHUTDOWN_GRACE=60"
check "на ноде пароль включён, поэтому не закрываем" has master "PREPARE_DISABLE_PASSWORD=false"
check "BOOTSTRAP_PASSWORD записан как ask" has master "BOOTSTRAP_PASSWORD=ask"
check "самого пароля в файле нет" lacks master "$PASSWORD"
check "токена в файле нет" lacks master "$TOKEN"
check "init ничего не менял: sudo всё ещё спрашивает пароль" test "$(ssh -p "$MASTER_SSH" -i "$TMP/id_test" $SSH_EXTRA_OPTS user@127.0.0.1 'sudo -n true 2>/dev/null; echo $?')" = 1

set_var master BOOTSTRAP_PASSWORD "$PASSWORD"
set_var master PREPARE_UPGRADE false
set_var master API_URL "https://127.0.0.1:$API_PORT"

echo "--- prepare: master ---"
pid=$(on "$MASTER" 'systemctl show k3s -p MainPID --value')
k prepare master
check "sudo без пароля настроен" test "$(ssh -p "$MASTER_SSH" -i "$TMP/id_test" $SSH_EXTRA_OPTS user@127.0.0.1 'sudo -n id -u')" = 0
check "вход по паролю остался" on "$MASTER" "sshd -T | grep -q '^passwordauthentication yes'"
check "защита k3s от needrestart на месте" on "$MASTER" "grep -q k3s /etc/needrestart/conf.d/k3s.conf"
check "prepare не перезапускал k3s" test "$(on "$MASTER" 'systemctl show k3s -p MainPID --value')" = "$pid"

echo "--- install: master ---"
refuses "без согласия install не трогает ручную ноду" k install master
check "и config.yaml остался ручным" on "$MASTER" "grep -q maximum-dead-containers=50 /etc/rancher/k3s/config.yaml"
echo y | k install master
check "config.yaml теперь от k3sctl" on "$MASTER" "head -1 /etc/rancher/k3s/config.yaml | grep -q 'Сгенерировано install_k3s.sh'"
check "стандарт применён: лимит логов" on "$MASTER" "grep -q container-log-max-size=10Mi /etc/rancher/k3s/config.yaml"
check "стандарт применён: штатная остановка подов" on "$MASTER" "grep -q 'shutdownGracePeriod: 60s' /var/lib/rancher/k3s/agent/etc/kubelet.conf.d/90-k3sctl.conf"
check "важные флаги в службе остались" on "$MASTER" "grep -q cluster-init /etc/systemd/system/k3s.service && grep -q traefik /etc/systemd/system/k3s.service"
check "ручной kubelet-arg ушёл" on "$MASTER" "! grep -rq maximum-dead-containers /etc/rancher/k3s/config.yaml /etc/systemd/system/k3s.service"
check "обе ноды Ready" wait_ready
check "данные кластера на месте" kube get configmap adopt-marker
check "traefik так и не появился" test "$(kube -n kube-system get deploy -o name | grep -c traefik || true)" = 0
echo "--- повторный install: уже наша нода, без вопросов ---"
k install master </dev/null
k access master

echo "--- init: настроенный worker ---"
BOOTSTRAP_PASSWORD=$PASSWORD k init worker <<<"$(printf '127.0.0.1:%s\nuser\nmaster\n' "$WORKER_SSH")"
check "роль agent с ноды" has worker "K3S_ROLE=agent"
check "адрес master с ноды" has worker "K3S_SERVER_URL=https://$MASTER:6443"
check "ssh-адрес master с ноды" has worker "K3S_SERVER_SSH_HOST=$MASTER"
check "контекст" has worker "CONTEXT_NAME=master"
check "ручная настройка видна в заметке" has worker "maximum-dead-containers=100"
check "токена в файле нет" lacks worker "$TOKEN"

set_var worker BOOTSTRAP_PASSWORD "$PASSWORD"
set_var worker PREPARE_UPGRADE false
set_var worker K3S_SERVER_SSH_HOST 127.0.0.1
set_var worker K3S_SERVER_SSH_PORT "$MASTER_SSH"

echo "--- prepare и install: worker ---"
k prepare worker
sed -e 's/^K3S_ROLE=.*/K3S_ROLE=server/' "$TMP/work/servers/worker.env" > "$TMP/work/servers/wrongrole.env"
refuses "install с чужой ролью отказывает" k install wrongrole
echo y | k install worker
check "служба k3s-agent работает" on "$WORKER" "systemctl is-active -q k3s-agent"
check "службы k3s (master) на worker нет" on "$WORKER" "! systemctl is-active -q k3s"
check "worker не потерял master" on "$WORKER" "grep -q 'K3S_URL=.*$MASTER' /etc/systemd/system/k3s-agent.service.env"
check "стандарт применён на worker" on "$WORKER" "grep -q container-log-max-size=10Mi /etc/rancher/k3s/config.yaml"
check "обе ноды Ready" wait_ready
check "под-свидетель пережил install" test "$(canary)" = "$CANARY"

echo "--- upgrade ---"
set_var master K3S_VERSION "$K3S_NEXT_VERSION"
refuses "worker нельзя обновлять раньше master, даже на следующую версию" k upgrade worker
check "версия worker прежняя" test "$(version "$WORKER")" = "$K3S_VERSION"

master_cfg=$(sum "$MASTER" /etc/rancher/k3s/config.yaml)
master_unit=$(sum "$MASTER" /etc/systemd/system/k3s.service)
worker_unit=$(sum "$WORKER" /etc/systemd/system/k3s-agent.service)

echo n | k upgrade master
check "ответ n ничего не меняет" test "$(version "$MASTER")" = "$K3S_VERSION"

echo y | k upgrade master
check "master обновлён" test "$(version "$MASTER")" = "$K3S_NEXT_VERSION"
check "снапшот etcd снят" on "$MASTER" "ls /var/lib/rancher/k3s/server/db/snapshots | grep -q ^pre-"
check "старый бинарник сохранён" on "$MASTER" "test -x /usr/local/bin/k3s.prev"
check "config.yaml не тронут" test "$(sum "$MASTER" /etc/rancher/k3s/config.yaml)" = "$master_cfg"
check "служба не тронута" test "$(sum "$MASTER" /etc/systemd/system/k3s.service)" = "$master_unit"

echo y | k upgrade worker
check "worker сам догнал master, а не взял версию из канала" test "$(version "$WORKER")" = "$K3S_NEXT_VERSION"
check "новая версия записана в файл worker" has worker "K3S_VERSION=$K3S_NEXT_VERSION"
check "служба worker не тронута" test "$(sum "$WORKER" /etc/systemd/system/k3s-agent.service)" = "$worker_unit"
check "обе ноды Ready" wait_ready
check "обе ноды на новой версии" test "$(kc get nodes --no-headers | awk -v v="$K3S_NEXT_VERSION" '$5 == v' | wc -l | tr -d ' ')" = 2
check "данные кластера на месте" kc get configmap adopt-marker
check "под-свидетель пережил обновление: тот же контейнер, без перезапусков" test "$(canary)" = "$CANARY"

echo
echo "ВСЕ ТЕСТЫ ПРОШЛИ"
