# k3sctl (k3s-config)

Go-программа для нового сервера с k3s:
`k3sctl init | cloud-init | prepare | install | access | setup | list <сервер>`.
Аргумент команды — имя сервера (файл `servers/<имя>.env` относительно
текущего каталога; папка `servers/` в `.gitignore`) или путь к файлу; разбор
в `config.Resolve`.
Шаблон файла — `internal/config/server.env.tpl` (встроен, `text/template`
по `Config`): `init` записывает его уже заполненным, все переменные активны,
пустой остаётся только `SSH_HOST`. Умолчания — теги `envDefault` плюс то,
что `init` находит сам: ssh-ключ (`findKey`, первый из `~/.ssh/id_rsa`,
`id_ed25519`, `id_ecdsa`; ищется только при `init`), версия k3s
(`k3s.StableVersion`: канал `update.k3s.io/v1-release/channels/stable`
отвечает редиректом на страницу релиза, версия — последний элемент адреса;
без сети — предупреждение и пустое поле), публичный ключ (`SSH_KEY` + `.pub`)
и имя контекста (имя сервера) — последние два `fillDerived` дописывает и
при `Load` для файлов, написанных вручную.
Файлы серверов и `~/.kube/config` содержат учётные данные, не читать.

- `init` — создать файл сервера из образца; `list` — заведённые серверы.
- `cloud-init` — записать user-data по шаблону в `servers/<имя>.cloud-init.yaml`
  (путь считает `config.CloudInitPath`, рядом с файлом настроек).
- `prepare` — подготовка свежего сервера по SSH. Вход выбирает
  `command.bootstrap`: `SSH_USER` по ключу → `BOOTSTRAP_USER` по ключу →
  `BOOTSTRAP_USER` по паролю (`BOOTSTRAP_PASSWORD`: пусто / `ask` /
  значение). Пароль уходит в ssh через помощника: `sshx.Target.Password`
  ставит `SSH_ASKPASS=<сам k3sctl>`, `SSH_ASKPASS_REQUIRE=force` и путь к
  временному файлу 0600 в `K3SCTL_ASKPASS_FILE`; `app.Run` в самом начале
  проверяет `sshx.Askpass` и в этом режиме только печатает пароль. При входе
  по паролю скрипт получает `ROOT_KEY=true` и кладёт первый ключ root-у.
  Шаги: пользователь, ключи (`SSH_PUBKEY` + `SSH_EXTRA_PUBKEYS` через запятую,
  путь или ключ строкой; `command.loadPubkeys`, в скрипт уходят по одному
  на строку в `SSH_PUBKEY_CONTENT`), запрет паролей, пакеты, sysctl, часовой пояс,
  `/etc/cron.d/k3sctl-reboot` (перезагрузка по `PREPARE_REBOOT_CRON`, если
  есть `/var/run/reboot-required`; на Alpine нет `/etc/cron.d` — пропуск),
  `/etc/systemd/journald.conf.d/90-k3sctl.conf` (`SystemMaxUse` из
  `JOURNAL_MAX_USE`; без journald — пропуск). В конце, если есть
  `/var/run/reboot-required`: `PREPARE_REBOOT` auto (перезагрузить, только
  пока k3s не установлен) / true / false; скрипт печатает на stdout
  `REBOOT=yes` и уходит в `reboot` фоном, Go-часть (`waitAfterReboot`) ждёт
  возврата по ssh до 3 минут и повторяет проверку входа и sudo.
- `install` — k3s официальным установщиком, роль `server`/`agent`;
  `config.yaml` пишется обеим ролям: `kubelet-arg` с `system-reserved`
  (`K3S_SYSTEM_RESERVED`, пусто = скрипт считает по `/proc/meminfo` и
  `nproc`: 10 % памяти в 256Mi..2Gi, 100m на ядро до 1000m) и
  `eviction-hard` (`K3S_EVICTION_HARD`), `container-log-max-size/-files`
  (`K3S_LOG_MAX_SIZE`, `K3S_LOG_MAX_FILES`); `tls-san` и
  `write-kubeconfig-mode` — только server. Штатная остановка подов при
  выключении (`K3S_SHUTDOWN_GRACE`, `K3S_SHUTDOWN_GRACE_CRITICAL`, секунды,
  0 = выкл) — **не флаг** (у kubelet его нет, k3s падает по кругу с
  «unknown flag»), а drop-in `KubeletConfiguration` в
  `/var/lib/rancher/k3s/agent/etc/kubelet.conf.d/90-k3sctl.conf`
  (`shutdownGracePeriod`, `shutdownGracePeriodCriticalPods`) плюс
  `InhibitDelayMaxSec` в `/etc/systemd/logind.conf.d/90-k3sctl.conf`, иначе
  logind даёт только 5 с. После ожидания `/readyz` скрипт 8 с ждёт и
  сверяет `ExecMainPID` службы и `/readyz` ещё раз: падающий по кругу k3s
  отвечает на секунду, и без этой проверки install «проходил»
  (`NRestarts` не годится — накопительный). Если служба уже работала,
  скрипт после установщика перезапускает её сам: при той же версии
  установщик этого не делает, и новый `config.yaml` не применяется.
- `access` — доступ через ServiceAccount, админский `k3s.yaml` на локальную
  машину не попадает.
- `setup` — `Prepare` + `Install` + `Access` (последний только для
  `K3S_ROLE=server`), `command.Setup`.

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
- `internal/httpx` — фабрика http-клиентов с явными таймаутами (все клиенты
  только через неё); `internal/k3s` — версия k3s с update.k3s.io.
- `internal/kubecfg` — `clientcmd` из client-go: Upsert кластера/пользователя/
  контекста, Verify через SelfSubjectReview и ServerVersion.
- `internal/config` — godotenv + caarlos0/env. Значения из файла кладутся
  поверх переменных окружения процесса. Особенность godotenv: комментарий
  после пустого значения (`X=   # текст`) читается как значение, поэтому в
  `server.env.tpl` все комментарии стоят отдельной строкой. Файлы серверов
  намеренно лежат в `servers/` текущего каталога, а не в `~/.config`: после
  `init` файл сразу под рукой для заполнения.

Проверки кода: `make check` (`gofmt -l`, `go vet`, `go test`). golangci-lint
не запускать.

## Тестовый стенд

`make e2e` (`./test/run_tests.sh`) — собирает бинарник, всё поднимает и
убирает сам; `KEEP=1` оставляет контейнер и временный каталог после прогона.

| Что | Значение |
|---|---|
| образ и контейнер | `k3s-sa-test` (из `test/Dockerfile`: alpine 3.22 + sshd + openrc + бинарник k3s `v1.33.5+k3s1` с GitHub; sudo/bash ставит сам `k3sctl prepare`) |
| SSH | `127.0.0.1:2222`; root стартует только с паролем (`ROOT_PASSWORD` контейнеру, как у провайдера), `prepare` заходит по нему через `BOOTSTRAP_PASSWORD` и кладёт root-у ключ; `user` заводит `k3sctl prepare` |
| API k3s | `https://127.0.0.1:16443`, только control plane (`--disable-agent`), поэтому `get nodes` пустой |
| ключ SSH | генерируется на каждый прогон во временном каталоге, нигде не хранится |
| kubeconfig и файл настроек | во временном каталоге (`LOCAL_KUBECONFIG`, командам передаётся путь к файлу); `~/.kube/config` не используется, `init`/`list` проверяются во временной рабочей папке |
| контекст / ServiceAccount | `k3s-sa-test` / `kube-system/tester-admin`; второй контекст `k3s-sa-test-ttl` проверяет токен со сроком |

Установщик k3s в контейнере запускается с `INSTALL_K3S_SKIP_DOWNLOAD=true
INSTALL_K3S_SKIP_START=true` (через `K3S_INSTALL_ENV`): службу он создаёт,
но не стартует, k3s запускает сам тест командой `k3s server --disable-agent`.
Контейнер `--privileged`, иначе k3s не стартует.
