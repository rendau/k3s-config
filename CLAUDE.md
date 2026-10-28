# k3sctl (k3s_config_merge)

Go-программа для нового сервера с k3s:
`k3sctl init | cloud-init | prepare | install | access | list <сервер>`.
Аргумент команды — имя сервера (файл `~/.config/k3sctl/<имя>.env`), путь к
файлу или ничего (`.env` в текущем каталоге); разбор в `config.Resolve`.
Образец файла — `internal/config/example.env` (встроен, его копирует `init`).
Файлы серверов и `~/.kube/config` содержат учётные данные, не читать.

- `init` — создать файл сервера из образца; `list` — заведённые серверы.
- `cloud-init` — напечатать user-data по шаблону.
- `prepare` — подготовка свежего сервера по SSH под `BOOTSTRAP_USER` (root):
  пользователь, ключ, запрет паролей, пакеты, sysctl, часовой пояс.
- `install` — k3s официальным установщиком, роль `server`/`agent`.
- `access` — доступ через ServiceAccount, админский `k3s.yaml` на локальную
  машину не попадает.

## Устройство

- `cmd/k3sctl/main.go` → `internal/app.Run` (команда + один позиционный
  аргумент, без флагов).
- `internal/command` — команды; каждая собирает серверный скрипт через
  `remote.Script(имя, переменные)` и запускает его `sshx.Target.RunScript`.
- `internal/remote/scripts/*.sh` — серверные части на POSIX `sh`
  (Ubuntu/Debian и Alpine), встроены через `embed`. Скрипт на сервере
  сначала целиком сохраняется во временный файл и только потом запускается,
  поэтому heredoc и чтение stdin внутри него безопасны. Переменные
  передаются строками `ИМЯ='значение'` в начале (экранирование `remote.ShellQuote`).
- `internal/remote/templates/cloud-init.yaml.tpl` — `text/template`.
- `internal/sshx` — системный `ssh` через `os/exec` (работают `~/.ssh/config`,
  агент, ProxyJump); stderr сервера идёт в stderr программы, stdout
  возвращается вызывающему.
- `internal/kubecfg` — `clientcmd` из client-go: Upsert кластера/пользователя/
  контекста, Verify через SelfSubjectReview и ServerVersion.
- `internal/config` — godotenv + caarlos0/env. Значения из файла кладутся
  поверх переменных окружения процесса. Особенность godotenv: комментарий
  после пустого значения (`X=   # текст`) читается как значение, поэтому в
  `example.env` такие комментарии стоят строкой выше. Каталог настроек —
  `$XDG_CONFIG_HOME/k3sctl`, иначе `~/.config/k3sctl` (намеренно не
  `os.UserConfigDir`, который на macOS даёт Library/Application Support).

Проверки кода: `make check` (`gofmt -l`, `go vet`, `go test`). golangci-lint
не запускать.

## Тестовый стенд

`make e2e` (`./test/run_tests.sh`) — собирает бинарник, всё поднимает и
убирает сам; `KEEP=1` оставляет контейнер и временный каталог после прогона.

| Что | Значение |
|---|---|
| образ и контейнер | `k3s-sa-test` (из `test/Dockerfile`: alpine 3.22 + sshd + openrc + бинарник k3s `v1.33.5+k3s1` с GitHub; sudo/bash ставит сам `k3sctl prepare`) |
| SSH | `127.0.0.1:2222`, пользователь `root` по ключу; `user` заводит `k3sctl prepare` |
| API k3s | `https://127.0.0.1:16443`, только control plane (`--disable-agent`), поэтому `get nodes` пустой |
| ключ SSH | генерируется на каждый прогон во временном каталоге, нигде не хранится |
| kubeconfig и файл настроек | во временном каталоге (`LOCAL_KUBECONFIG`, командам передаётся путь к файлу); `~/.kube/config` и `~/.config/k3sctl` не используются, `init`/`list` проверяются с подменённым `HOME` |
| контекст / ServiceAccount | `k3s-sa-test` / `kube-system/tester-admin`; второй контекст `k3s-sa-test-ttl` проверяет токен со сроком |

Установщик k3s в контейнере запускается с `INSTALL_K3S_SKIP_DOWNLOAD=true
INSTALL_K3S_SKIP_START=true` (через `K3S_INSTALL_ENV`): службу он создаёт,
но не стартует, k3s запускает сам тест командой `k3s server --disable-agent`.
Контейнер `--privileged`, иначе k3s не стартует.
