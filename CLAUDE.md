# k3s_config_merge

Скрипты для нового сервера с k3s, настройки в `.env` рядом (в git не входит,
образец `.env.example`). `.env` и `~/.kube/config` содержат учётные данные,
не читать.

- `prepare_server.sh` — подготовка свежего сервера по SSH под `BOOTSTRAP_USER`
  (root): пользователь, ключ, запрет паролей, пакеты, sysctl, часовой пояс;
  `--print-cloud-init` рендерит `cloud-init.yaml.tpl`.
- `install_k3s.sh` — k3s официальным установщиком, роль `server`/`agent`.
- `setup_sa_access.sh` — доступ через ServiceAccount, админский `k3s.yaml`
  на локальную машину не попадает.

Устройство: локальная часть на bash 3.2 (штатный на macOS), общие функции в
`lib/common.sh`; серверная часть в `lib/remote_*.sh` на POSIX `sh`
(Ubuntu/Debian и Alpine), передаётся по SSH через stdin вместе с
переменными (`remote_vars` + `remote_sh`). Серверный скрипт сначала целиком
сохраняется во временный файл и только потом запускается, поэтому heredoc и
чтение stdin внутри него безопасны.

## Тестовый стенд

`./test/run_tests.sh` — всё поднимает и убирает сам; `KEEP=1` оставляет
контейнер и временный каталог после прогона.

| Что | Значение |
|---|---|
| образ и контейнер | `k3s-sa-test` (из `test/Dockerfile`: alpine 3.22 + sshd + openrc + бинарник k3s `v1.33.5+k3s1` с GitHub; sudo/bash ставит сам `prepare_server.sh`) |
| SSH | `127.0.0.1:2222`, пользователь `root` по ключу; `user` заводит `prepare_server.sh` |
| API k3s | `https://127.0.0.1:16443`, только control plane (`--disable-agent`), поэтому `get nodes` пустой |
| ключ SSH | генерируется на каждый прогон во временном каталоге, нигде не хранится |
| kubeconfig и файл настроек | во временном каталоге (`LOCAL_KUBECONFIG`, `--env-file`); `~/.kube/config` и `.env` не используются |
| контекст / ServiceAccount | `k3s-sa-test` / `kube-system/tester-admin`; второй контекст `k3s-sa-test-ttl` проверяет токен со сроком |

Установщик k3s в контейнере запускается с `INSTALL_K3S_SKIP_DOWNLOAD=true
INSTALL_K3S_SKIP_START=true` (через `K3S_INSTALL_ENV`): службу он создаёт,
но не стартует, k3s запускает сам тест командой `k3s server --disable-agent`.
Контейнер `--privileged`, иначе k3s не стартует.
