# Серверная часть install_k3s.sh. Выполняется от root, POSIX sh.
# Переменные ставит в начало локальный скрипт: K3S_ROLE, SSH_USER, K3S_VERSION,
# K3S_TLS_SAN, K3S_EXTRA_ARGS, K3S_INSTALL_ENV, K3S_SERVER_URL, K3S_TOKEN,
# K3S_SYSTEM_RESERVED, K3S_EVICTION_HARD, K3S_LOG_MAX_SIZE, K3S_LOG_MAX_FILES.
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
K3S_SYSTEM_RESERVED=${K3S_SYSTEM_RESERVED:-}
K3S_EVICTION_HARD=${K3S_EVICTION_HARD:-}
K3S_LOG_MAX_SIZE=${K3S_LOG_MAX_SIZE:-}
K3S_LOG_MAX_FILES=${K3S_LOG_MAX_FILES:-}
command -v curl >/dev/null 2>&1 || die "на сервере нужен curl"

# --- 1. запас ресурсов для системы ---
# Без запаса поды могут выесть всю память и диск, и первой умирает сама ОС
# вместе с ssh. system-reserved вычитается из того, что видно подам,
# eviction-hard выселяет поды, когда свободного остаётся меньше порога.
clamp() { # clamp ЗНАЧЕНИЕ МИН МАКС
  v=$1; [ "$v" -lt "$2" ] && v=$2; [ "$v" -gt "$3" ] && v=$3; echo "$v"
}
if [ -z "$K3S_SYSTEM_RESERVED" ]; then
  mem_kb=$(awk '/^MemTotal:/ { print $2 }' /proc/meminfo)
  mem_mi=$(clamp $((mem_kb / 1024 / 10)) 256 2048)
  cores=$(nproc 2>/dev/null || echo 1)
  cpu_m=$(clamp $((cores * 100)) 100 1000)
  K3S_SYSTEM_RESERVED="cpu=${cpu_m}m,memory=${mem_mi}Mi"
  log "запас для системы по размеру машины: $K3S_SYSTEM_RESERVED"
else
  log "запас для системы: $K3S_SYSTEM_RESERVED"
fi

# --- 2. config.yaml ---
# kubelet-arg нужен обеим ролям; tls-san и write-kubeconfig-mode — только server.
mkdir -p /etc/rancher/k3s
cfg=/etc/rancher/k3s/config.yaml
if [ -f "$cfg" ]; then
  bak="$cfg.bak.$(date +%Y%m%d%H%M%S)"
  cp "$cfg" "$bak"
  warn "$cfg уже был, старая версия сохранена в $bak"
fi
{
  echo '# Сгенерировано install_k3s.sh'
  if [ "$K3S_ROLE" = server ]; then
    echo 'write-kubeconfig-mode: "0600"'
    if [ -n "$K3S_TLS_SAN" ]; then
      echo 'tls-san:'
      printf '%s\n' "$K3S_TLS_SAN" | tr ',' '\n' \
        | sed 's/^[[:space:]]*//;s/[[:space:]]*$//;/^$/d;s/^/  - /'
    fi
  fi
  echo 'kubelet-arg:'
  echo "  - system-reserved=$K3S_SYSTEM_RESERVED"
  if [ -n "$K3S_EVICTION_HARD" ]; then
    echo "  - eviction-hard=$K3S_EVICTION_HARD"
  fi
  # Логи контейнеров: болтливый под за месяцы забьёт диск.
  if [ -n "$K3S_LOG_MAX_SIZE" ]; then
    echo "  - container-log-max-size=$K3S_LOG_MAX_SIZE"
  fi
  if [ -n "$K3S_LOG_MAX_FILES" ] && [ "$K3S_LOG_MAX_FILES" -gt 0 ]; then
    echo "  - container-log-max-files=$K3S_LOG_MAX_FILES"
  fi
} > "$cfg"
chmod 0600 "$cfg"
log "записан $cfg"

# --- 3. официальный установщик ---
# Если k3s уже работает, установщик при той же версии службу не перезапускает,
# и новый config.yaml не применяется — перезапускаем сами после установки.
svc=k3s
[ "$K3S_ROLE" = agent ] && svc=k3s-agent
was_active=false
if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet "$svc" 2>/dev/null; then
  was_active=true
elif command -v rc-service >/dev/null 2>&1 && rc-service "$svc" status >/dev/null 2>&1; then
  was_active=true
fi

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

if [ "$was_active" = true ]; then
  log "k3s уже работал — перезапускаю службу $svc, чтобы применить config.yaml"
  if command -v systemctl >/dev/null 2>&1; then
    systemctl restart "$svc"
  else
    rc-service "$svc" restart >/dev/null
  fi
fi

# --- 4. sudo на kubectl для SSH_USER (только server) ---
if [ "$K3S_ROLE" = server ] && [ "$SSH_USER" != root ]; then
  kubectl_bin=$(command -v kubectl || echo /usr/local/bin/kubectl)
  printf '%s ALL=(root) NOPASSWD: %s\n' "$SSH_USER" "$kubectl_bin" > /etc/sudoers.d/k3s-kubectl
  chmod 0440 /etc/sudoers.d/k3s-kubectl
  visudo -cq -f /etc/sudoers.d/k3s-kubectl || die "sudoers для kubectl не прошёл проверку"
  log "sudo -n kubectl разрешён пользователю $SSH_USER"
fi

# --- 5. ждём готовности (только server, если установщик запускал службу) ---
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
