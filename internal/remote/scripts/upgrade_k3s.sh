# Серверная часть upgrade_k3s.sh. Выполняется от root, POSIX sh.
# Переменные ставит в начало локальный скрипт: K3S_VERSION, K3S_SERVICE
# (k3s или k3s-agent). Меняет только бинарник: службу, файл окружения и
# config.yaml не трогает, поэтому годится и для серверов, настроенных руками.
set -eu
PATH=$PATH:/usr/local/bin

log()  { printf '  [сервер] %s\n' "$*" >&2; }
die()  { printf '  [сервер] ОШИБКА: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" = 0 ] || die "нужен root"
: "${K3S_VERSION:?}" "${K3S_SERVICE:?}"
command -v curl >/dev/null 2>&1 || die "на сервере нужен curl"

is_active() { systemctl is-active -q "$K3S_SERVICE" 2>/dev/null || rc-service "$K3S_SERVICE" status >/dev/null 2>&1; }

bin=$(readlink -f "$(command -v k3s)") || die "не нашёл k3s"
case $(uname -m) in
  x86_64)        arch=amd64 suffix= ;;
  aarch64|arm64) arch=arm64 suffix=-arm64 ;;
  armv7l|armv8l) arch=arm   suffix=-armhf ;;
  s390x)         arch=s390x suffix=-s390x ;;
  *) die "неизвестная архитектура $(uname -m)" ;;
esac

# С 1.35 kubelet на cgroup v1 не запускается.
minor=$(printf '%s' "$K3S_VERSION" | sed -E 's/^v1\.([0-9]+)\..*/\1/')
if [ "$minor" -ge 35 ] && [ ! -f /sys/fs/cgroup/cgroup.controllers ]; then
  die "на сервере cgroup v1, k3s $K3S_VERSION на нём не запустится"
fi

was_active=false
is_active && was_active=true

# --- 1. снапшот etcd ---
# Etcd при обновлении меняет формат данных: откатить master можно только
# старым бинарником из этого снапшота.
if [ "$K3S_SERVICE" = k3s ] && [ "$was_active" = true ] && [ -d /var/lib/rancher/k3s/server/db/etcd ]; then
  snapshot="pre-$(printf '%s' "$K3S_VERSION" | tr '+' '-')"
  log "снапшот etcd $snapshot"
  k3s etcd-snapshot save --name "$snapshot" >&2 || die "снапшот etcd не снялся, ничего не менял"
fi

# --- 2. скачивание и проверка суммы ---
url="https://github.com/k3s-io/k3s/releases/download/$(printf '%s' "$K3S_VERSION" | sed 's/+/%2B/')"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
log "скачиваю k3s $K3S_VERSION ($arch)"
curl -fsSL -o "$tmp/k3s$suffix" "$url/k3s$suffix" || die "не скачался $url/k3s$suffix"
curl -fsSL -o "$tmp/sha256sum.txt" "$url/sha256sum-$arch.txt" || die "не скачались контрольные суммы"
(cd "$tmp" && grep " k3s$suffix\$" sha256sum.txt | sha256sum -c - >&2) || die "контрольная сумма не сошлась"

# --- 3. замена бинарника ---
cp -p "$bin" "$bin.prev"
cp "$tmp/k3s$suffix" "$bin.new"
chown root:root "$bin.new"
chmod 0755 "$bin.new"
mv -f "$bin.new" "$bin"
if command -v restorecon >/dev/null 2>&1; then
  restorecon "$bin"
fi
log "бинарник заменён, старый лежит в $bin.prev"

# --- 4. перезапуск и проверка ---
if [ "$was_active" != true ]; then
  log "служба $K3S_SERVICE не работала, не запускаю"
  exit 0
fi

log "перезапускаю службу $K3S_SERVICE"
if command -v systemctl >/dev/null 2>&1; then
  systemctl restart "$K3S_SERVICE" || die "служба не перезапустилась, смотрите journalctl -u $K3S_SERVICE; старый бинарник: $bin.prev"
else
  rc-service "$K3S_SERVICE" restart >/dev/null || die "служба не перезапустилась; старый бинарник: $bin.prev"
fi

ready() {
  is_active || return 1
  [ "$K3S_SERVICE" = k3s ] || return 0
  k3s kubectl get --raw /readyz >/dev/null 2>&1
}
i=0
until ready; do
  i=$((i + 1))
  [ "$i" -lt 60 ] || die "k3s не поднялся за 5 минут, смотрите журнал службы; старый бинарник: $bin.prev"
  sleep 5
done

# Падающий по кругу k3s отвечает на секунду и снова гаснет: процесс службы
# должен остаться тем же.
if command -v systemctl >/dev/null 2>&1; then
  pid1=$(systemctl show "$K3S_SERVICE" -p ExecMainPID --value)
  sleep 8
  pid2=$(systemctl show "$K3S_SERVICE" -p ExecMainPID --value)
  { [ "$pid1" = "$pid2" ] && ready; } || die "служба $K3S_SERVICE перезапускается по кругу, смотрите journalctl -u $K3S_SERVICE; старый бинарник: $bin.prev"
fi
log "служба работает на $K3S_VERSION"
