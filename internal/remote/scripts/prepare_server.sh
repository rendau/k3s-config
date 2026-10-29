# Серверная часть prepare_server.sh. Выполняется от root, POSIX sh.
# Переменные ставит в начало локальный скрипт: SSH_USER, SSH_PUBKEY_CONTENT
# (публичные ключи, по одному на строку),
# PREPARE_UPGRADE, PREPARE_DISABLE_PASSWORD, SERVER_TIMEZONE, PREPARE_REBOOT_CRON,
# JOURNAL_MAX_USE.
set -eu

log()  { printf '  [сервер] %s\n' "$*" >&2; }
warn() { printf '  [сервер] ВНИМАНИЕ: %s\n' "$*" >&2; }
die()  { printf '  [сервер] ОШИБКА: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" = 0 ] || die "нужен root"
: "${SSH_USER:?}" "${SSH_PUBKEY_CONTENT:?}"
PREPARE_UPGRADE=${PREPARE_UPGRADE:-true}
PREPARE_DISABLE_PASSWORD=${PREPARE_DISABLE_PASSWORD:-true}
SERVER_TIMEZONE=${SERVER_TIMEZONE:-}
PREPARE_REBOOT_CRON=${PREPARE_REBOOT_CRON:-}
JOURNAL_MAX_USE=${JOURNAL_MAX_USE:-}

# --- 1. пакеты ---
if command -v apt-get >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a
  log "apt-get update"
  apt-get -qq update
  if [ "$PREPARE_UPGRADE" = true ]; then
    log "apt-get upgrade"
    apt-get -y -qq -o Dpkg::Options::=--force-confold upgrade
  fi
  apt-get -y -qq install sudo curl ca-certificates bash-completion openssh-server cron
elif command -v apk >/dev/null 2>&1; then
  log "apk update"
  apk -q update
  if [ "$PREPARE_UPGRADE" = true ]; then
    log "apk upgrade"
    apk -q upgrade
  fi
  apk -q add sudo curl ca-certificates bash bash-completion openssh shadow tzdata
else
  warn "неизвестный менеджер пакетов: пакеты не обновляю"
fi

# --- 2. пользователь ---
shell=/bin/sh
[ -x /bin/bash ] && shell=/bin/bash
if id "$SSH_USER" >/dev/null 2>&1; then
  log "пользователь $SSH_USER уже есть"
else
  log "создаю пользователя $SSH_USER"
  useradd -m -U -s "$shell" "$SSH_USER"
fi
# useradd оставляет в /etc/shadow «!» — без PAM (alpine) sshd считает такую
# учётку запертой и не пускает даже по ключу. «*» = пароля нет, но вход
# по ключу разрешён. Настоящий пароль, если он есть, не трогаем.
case "$(awk -F: -v u="$SSH_USER" '$1 == u { print $2 }' /etc/shadow)" in
  ""|"!"|"!!") usermod -p '*' "$SSH_USER" ;;
esac

# --- 3. sudo без пароля ---
if getent group sudo >/dev/null 2>&1; then
  usermod -aG sudo "$SSH_USER"
elif getent group wheel >/dev/null 2>&1; then
  usermod -aG wheel "$SSH_USER"
fi
mkdir -p /etc/sudoers.d
sudoers_file=/etc/sudoers.d/90-$SSH_USER
printf '%s ALL=(ALL) NOPASSWD:ALL\n' "$SSH_USER" > "$sudoers_file"
chmod 0440 "$sudoers_file"
grep -Eq '^[#@]includedir[[:space:]]+/etc/sudoers.d' /etc/sudoers \
  || printf '\n#includedir /etc/sudoers.d\n' >> /etc/sudoers
visudo -cq -f "$sudoers_file" || die "файл sudoers не прошёл проверку"
log "sudo без пароля для $SSH_USER настроен"

# --- 4. ssh-ключ ---
home=$(getent passwd "$SSH_USER" | cut -d: -f6)
[ -n "$home" ] || die "не нашёл домашний каталог $SSH_USER"
mkdir -p "$home/.ssh"
touch "$home/.ssh/authorized_keys"
printf '%s\n' "$SSH_PUBKEY_CONTENT" | while IFS= read -r key; do
  [ -n "$key" ] || continue
  if grep -qxF "$key" "$home/.ssh/authorized_keys"; then
    log "ключ уже в authorized_keys: ${key##* }"
  else
    printf '%s\n' "$key" >> "$home/.ssh/authorized_keys"
    log "ключ добавлен в authorized_keys: ${key##* }"
  fi
done
chmod 700 "$home/.ssh"
chmod 600 "$home/.ssh/authorized_keys"
chown -R "$SSH_USER:" "$home/.ssh"

# --- 5. sysctl для агентов логов ---
cat > /etc/sysctl.d/90-k3s.conf <<'SYSCTL'
# Лимиты inotify: fluent-bit и другие агенты логов следят за файлами
# контейнеров через inotify, дефолтных лимитов не хватает.
fs.inotify.max_user_instances = 512
fs.inotify.max_user_watches = 524288
SYSCTL
sysctl -p /etc/sysctl.d/90-k3s.conf >/dev/null 2>&1 \
  || warn "sysctl не применился сейчас (в контейнере это нормально), подействует после перезагрузки"

# --- 6. часовой пояс ---
if [ -n "$SERVER_TIMEZONE" ]; then
  if command -v timedatectl >/dev/null 2>&1 && timedatectl set-timezone "$SERVER_TIMEZONE" 2>/dev/null; then
    log "часовой пояс: $SERVER_TIMEZONE"
  elif [ -f "/usr/share/zoneinfo/$SERVER_TIMEZONE" ]; then
    ln -sf "/usr/share/zoneinfo/$SERVER_TIMEZONE" /etc/localtime
    printf '%s\n' "$SERVER_TIMEZONE" > /etc/timezone
    log "часовой пояс: $SERVER_TIMEZONE"
  else
    warn "часовой пояс $SERVER_TIMEZONE не найден, пропускаю"
  fi
fi

# --- 7. запрет входа по паролю ---
if [ "$PREPARE_DISABLE_PASSWORD" = true ]; then
  conf="PasswordAuthentication no
ChallengeResponseAuthentication no
PermitRootLogin prohibit-password
PubkeyAuthentication yes"
  if grep -Eq '^[[:space:]]*Include[[:space:]]+/etc/ssh/sshd_config\.d/' /etc/ssh/sshd_config; then
    # Имя 00-* читается раньше провайдерского 50-cloud-init.conf, а у sshd
    # побеждает первое значение, поэтому правило не слетит после перезагрузки.
    mkdir -p /etc/ssh/sshd_config.d
    printf '%s\n' "$conf" > /etc/ssh/sshd_config.d/00-no-password.conf
  else
    sed -i -E '/^[[:space:]]*#?[[:space:]]*(PasswordAuthentication|ChallengeResponseAuthentication|KbdInteractiveAuthentication|PermitRootLogin|PubkeyAuthentication)([[:space:]]|$)/d' /etc/ssh/sshd_config
    { printf '%s\n' "$conf"; cat /etc/ssh/sshd_config; } > /etc/ssh/sshd_config.tmp
    mv /etc/ssh/sshd_config.tmp /etc/ssh/sshd_config
  fi
  # Чтобы cloud-init провайдера не включил пароли обратно.
  if [ -d /etc/cloud/cloud.cfg.d ]; then
    printf 'ssh_pwauth: false\ndisable_root: true\n' > /etc/cloud/cloud.cfg.d/99-no-password.cfg
  fi
  sshd_bin=$(command -v sshd || echo /usr/sbin/sshd)
  # На Debian/Ubuntu sshd -t требует каталог привилегий /run/sshd; systemd
  # удаляет его при остановке службы (например, пока apt обновлял openssh),
  # поэтому перед проверкой создаём сами.
  mkdir -p /run/sshd
  "$sshd_bin" -t || die "конфиг sshd не прошёл проверку, перезапуск отменён"
  if command -v systemctl >/dev/null 2>&1 && systemctl restart ssh 2>/dev/null; then :
  elif command -v systemctl >/dev/null 2>&1 && systemctl restart sshd 2>/dev/null; then :
  elif command -v rc-service >/dev/null 2>&1 && rc-service sshd restart >/dev/null 2>&1; then :
  elif [ -f /run/sshd.pid ] || [ -f /var/run/sshd.pid ]; then
    kill -HUP "$(cat /run/sshd.pid 2>/dev/null || cat /var/run/sshd.pid)"
  else
    warn "не смог перезапустить sshd, перезапустите вручную"
  fi
  log "вход по паролю отключён"
fi

# --- 8. перезагрузка после обновлений по расписанию ---
# Обновления ставит unattended-upgrades ежедневно, но сам не перезагружает;
# это задание перезагружает по расписанию, только если система просит
# (/var/run/reboot-required — признак Debian/Ubuntu, на Alpine его нет).
cron_file=/etc/cron.d/k3sctl-reboot
if [ -n "$PREPARE_REBOOT_CRON" ] && [ -d /etc/cron.d ]; then
  printf 'SHELL=/bin/sh\n%s root [ -f /var/run/reboot-required ] && /sbin/shutdown -r now "k3sctl: перезагрузка после обновлений"\n' \
    "$PREPARE_REBOOT_CRON" > "$cron_file"
  chmod 644 "$cron_file"
  log "перезагрузка после обновлений по расписанию «$PREPARE_REBOOT_CRON» (время сервера)"
elif [ -n "$PREPARE_REBOOT_CRON" ]; then
  warn "нет /etc/cron.d, перезагрузку по расписанию не завожу"
else
  rm -f "$cron_file"
fi

# --- 9. потолок системного журнала ---
# journald по умолчанию растёт до 10% диска; на Alpine journald нет — пропуск.
journal_conf=/etc/systemd/journald.conf.d/90-k3sctl.conf
if [ -n "$JOURNAL_MAX_USE" ] && command -v journalctl >/dev/null 2>&1; then
  mkdir -p /etc/systemd/journald.conf.d
  printf '[Journal]\nSystemMaxUse=%s\n' "$JOURNAL_MAX_USE" > "$journal_conf"
  systemctl restart systemd-journald 2>/dev/null || warn "не смог перезапустить journald, лимит применится после перезагрузки"
  log "системный журнал ограничен $JOURNAL_MAX_USE"
elif [ -n "$JOURNAL_MAX_USE" ]; then
  warn "journald нет, потолок журнала не ставлю"
fi

# --- 10. итог ---
if [ -f /var/run/reboot-required ]; then
  warn "после обновления пакетов нужна перезагрузка (/var/run/reboot-required)"
fi
log "сервер подготовлен"
