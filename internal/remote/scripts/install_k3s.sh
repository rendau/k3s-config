# Серверная часть install_k3s.sh. Выполняется от root, POSIX sh.
# Переменные ставит в начало локальный скрипт: K3S_ROLE, SSH_USER, K3S_VERSION,
# K3S_TLS_SAN, K3S_EXTRA_ARGS, K3S_INSTALL_ENV, K3S_SERVER_URL, K3S_TOKEN.
set -eu

log()  { printf '  [сервер] %s\n' "$*" >&2; }
warn() { printf '  [сервер] ВНИМАНИЕ: %s\n' "$*" >&2; }
die()  { printf '  [сервер] ОШИБКА: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" = 0 ] || die "нужен root"
: "${K3S_ROLE:?}" "${SSH_USER:?}"
K3S_VERSION=${K3S_VERSION:-}
K3S_TLS_SAN=${K3S_TLS_SAN:-}
K3S_EXTRA_ARGS=${K3S_EXTRA_ARGS:-}
K3S_INSTALL_ENV=${K3S_INSTALL_ENV:-}
K3S_SERVER_URL=${K3S_SERVER_URL:-}
K3S_TOKEN=${K3S_TOKEN:-}
command -v curl >/dev/null 2>&1 || die "на сервере нужен curl"

# --- 1. config.yaml (только для server) ---
if [ "$K3S_ROLE" = server ]; then
  mkdir -p /etc/rancher/k3s
  cfg=/etc/rancher/k3s/config.yaml
  if [ -f "$cfg" ]; then
    bak="$cfg.bak.$(date +%Y%m%d%H%M%S)"
    cp "$cfg" "$bak"
    warn "$cfg уже был, старая версия сохранена в $bak"
  fi
  {
    echo '# Сгенерировано install_k3s.sh'
    echo 'write-kubeconfig-mode: "0600"'
    if [ -n "$K3S_TLS_SAN" ]; then
      echo 'tls-san:'
      printf '%s\n' "$K3S_TLS_SAN" | tr ',' '\n' \
        | sed 's/^[[:space:]]*//;s/[[:space:]]*$//;/^$/d;s/^/  - /'
    fi
  } > "$cfg"
  chmod 0600 "$cfg"
  log "записан $cfg"
fi

# --- 2. официальный установщик ---
log "запускаю установщик k3s: роль $K3S_ROLE, версия ${K3S_VERSION:-последняя стабильная}"
installer=$(mktemp)
curl -sfL https://get.k3s.io -o "$installer" || die "не скачался установщик с https://get.k3s.io"
# shellcheck disable=SC2086
if [ "$K3S_ROLE" = server ]; then
  env INSTALL_K3S_VERSION="$K3S_VERSION" $K3S_INSTALL_ENV sh "$installer" server $K3S_EXTRA_ARGS
else
  env INSTALL_K3S_VERSION="$K3S_VERSION" K3S_URL="$K3S_SERVER_URL" K3S_TOKEN="$K3S_TOKEN" \
    $K3S_INSTALL_ENV sh "$installer" agent $K3S_EXTRA_ARGS
fi
rm -f "$installer"

# --- 3. sudo на kubectl для SSH_USER (только server) ---
if [ "$K3S_ROLE" = server ] && [ "$SSH_USER" != root ]; then
  kubectl_bin=$(command -v kubectl || echo /usr/local/bin/kubectl)
  printf '%s ALL=(root) NOPASSWD: %s\n' "$SSH_USER" "$kubectl_bin" > /etc/sudoers.d/k3s-kubectl
  chmod 0440 /etc/sudoers.d/k3s-kubectl
  visudo -cq -f /etc/sudoers.d/k3s-kubectl || die "sudoers для kubectl не прошёл проверку"
  log "sudo -n kubectl разрешён пользователю $SSH_USER"
fi

# --- 4. ждём готовности (только server, если установщик запускал службу) ---
if [ "$K3S_ROLE" = server ]; then
  case " $K3S_INSTALL_ENV " in
    *INSTALL_K3S_SKIP_START=true*) log "служба не запускалась (INSTALL_K3S_SKIP_START), готовность не жду" ;;
    *)
      i=0
      until kubectl get --raw /readyz >/dev/null 2>&1; do
        i=$((i + 1))
        if [ "$i" -ge 60 ]; then
          warn "API не ответил за 120 с, смотрите: journalctl -u k3s"
          break
        fi
        sleep 2
      done
      kubectl get nodes -o wide >&2 || true
      ;;
  esac
  log "k3s server установлен"
else
  log "k3s agent установлен и подключается к $K3S_SERVER_URL; проверьте на master: kubectl get nodes"
fi
