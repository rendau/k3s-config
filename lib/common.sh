#!/usr/bin/env bash
# Общие функции локальных скриптов. Подключение:
#   . "$(dirname "$0")/lib/common.sh"
# Должно работать в bash 3.2 (штатный на macOS).

LIB_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(cd "$LIB_DIR/.." && pwd)

log()  { printf '==> %s\n' "$*" >&2; }
warn() { printf 'ВНИМАНИЕ: %s\n' "$*" >&2; }
die()  { printf 'ОШИБКА: %s\n' "$*" >&2; exit 1; }

# load_env FILE — читает файл настроек (формат KEY=value, образец .env.example).
load_env() {
  [ -f "$1" ] || die "файл настроек не найден: $1 (образец: $ROOT_DIR/.env.example)"
  set -a
  # shellcheck disable=SC1090
  . "$1"
  set +a
}

# require_vars NAME... — проверяет, что переменные заданы и не пусты.
require_vars() {
  local name value
  for name in "$@"; do
    eval "value=\${$name:-}"
    [ -n "$value" ] || die "в файле настроек не задано $name"
  done
}

# expand_path PATH — раскрывает ведущую ~.
expand_path() {
  case "$1" in
    "~")   printf '%s' "$HOME" ;;
    "~/"*) printf '%s' "$HOME/${1#\~/}" ;;
    *)     printf '%s' "$1" ;;
  esac
}

# shq VALUE — заключает значение в одинарные кавычки для sh.
shq() {
  printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
}

# remote_vars NAME... — печатает строки NAME='value', которые ставятся
# в начало серверного скрипта, чтобы передать ему настройки.
remote_vars() {
  local name value
  for name in "$@"; do
    eval "value=\${$name:-}"
    printf '%s=%s\n' "$name" "$(shq "$value")"
  done
}

# ssh_args USER — заполняет массив SSH_ARGS параметрами подключения.
ssh_args() {
  SSH_ARGS=(-o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=15 -p "${SSH_PORT:-22}")
  if [ -n "${SSH_KEY:-}" ]; then
    SSH_ARGS=("${SSH_ARGS[@]}" -i "$(expand_path "$SSH_KEY")" -o IdentitiesOnly=yes)
  fi
  if [ -n "${SSH_EXTRA_OPTS:-}" ]; then
    # shellcheck disable=SC2206
    SSH_ARGS=("${SSH_ARGS[@]}" $SSH_EXTRA_OPTS)
  fi
  SSH_ARGS=("${SSH_ARGS[@]}" "$1@$SSH_HOST")
}

# remote_sh USER [root] < script — выполняет sh-скрипт со stdin на сервере.
# Скрипт сначала целиком сохраняется во временный файл и только потом
# запускается, поэтому команды внутри могут свободно читать свой stdin.
# С аргументом root скрипт запускается через sudo -n (если вошли не root).
remote_sh() {
  local runner
  ssh_args "$1"
  if [ "${2:-}" = root ]; then
    runner='if [ "$(id -u)" = 0 ]; then sh "$f"; else sudo -n sh "$f"; fi'
  else
    runner='sh "$f"'
  fi
  ssh "${SSH_ARGS[@]}" "f=\$(mktemp) && cat >\"\$f\" && { $runner; }; rc=\$?; rm -f \"\$f\"; exit \$rc"
}

# remote_cmd USER CMD — одна команда на сервере, без stdin.
remote_cmd() {
  ssh_args "$1"
  ssh -n "${SSH_ARGS[@]}" "$2"
}

# parse_env_file_arg — разбирает --env-file из аргументов, остальные
# аргументы складывает в REST_ARGS. Вызов: parse_env_file_arg "$@".
ENV_FILE=$ROOT_DIR/.env
REST_ARGS=()
parse_env_file_arg() {
  REST_ARGS=()
  while [ $# -gt 0 ]; do
    case "$1" in
      --env-file)   [ -n "${2:-}" ] || die "--env-file: нужен путь"; ENV_FILE=$2; shift 2 ;;
      --env-file=*) ENV_FILE=${1#*=}; shift ;;
      *)            REST_ARGS=(${REST_ARGS[@]+"${REST_ARGS[@]}"} "$1"); shift ;;
    esac
  done
}
