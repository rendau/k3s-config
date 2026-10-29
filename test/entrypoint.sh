#!/bin/sh
# Кладёт тестовый ключ root-у (ROOT_AUTHORIZED_KEY) и/или ставит ему пароль
# (ROOT_PASSWORD — как у провайдера) и запускает sshd на переднем плане.
set -eu
if [ -n "${ROOT_AUTHORIZED_KEY:-}" ]; then
  printf '%s\n' "$ROOT_AUTHORIZED_KEY" > /root/.ssh/authorized_keys
  chmod 600 /root/.ssh/authorized_keys
fi
if [ -n "${ROOT_PASSWORD:-}" ]; then
  printf 'root:%s\n' "$ROOT_PASSWORD" | chpasswd
fi
exec /usr/sbin/sshd -D -e
