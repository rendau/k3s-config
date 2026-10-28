# k3sctl

Одна программа для нового сервера с k3s: подготовить сервер, поставить k3s
и получить к нему доступ без копирования админского `k3s.yaml` на свою машину.

| Команда | Что делает |
|---|---|
| `k3sctl prepare` | Готовит свежий сервер: пользователь с sudo и вашим ssh-ключом, запрет входа по паролю, обновление пакетов, sysctl для агентов логов, часовой пояс. Ключ `--print-cloud-init` печатает то же самое в виде user-data для cloud-init. |
| `k3sctl install` | Ставит k3s официальным установщиком: master (`server`) или worker (`agent`). |
| `k3sctl access` | Заводит в кластере учётку (ServiceAccount), получает её токен и записывает контекст в `~/.kube/config`. |

Настройки читаются из файла `.env` в текущем каталоге (в git не входит,
образец `.env.example`). Один `.env` описывает один сервер; для другого
сервера передайте `--env-file другой.env`.

## Установка

```bash
make install
```

Бинарник `k3sctl` попадает в `$(go env GOPATH)/bin`. Сборка в `build/`:
`make build`.

## Требования

- На Mac: `ssh` (системный, со всеми настройками из `~/.ssh/config`), Go для
  сборки.
- На сервере: Ubuntu/Debian (apt) или Alpine (apk), `curl`, доступ в
  интернет до `get.k3s.io`.

## Порядок работы

### 1. Подготовить сервер

Вариант А, при создании виртуалки (Yandex Cloud и другие провайдеры с
cloud-init): вставьте результат в поле user-data.

```bash
k3sctl prepare --print-cloud-init > user-data.yaml
```

Вариант Б, на уже созданном сервере: заходит под `BOOTSTRAP_USER` (обычно
`root` с ключом от провайдера) и делает всё то же по SSH.

```bash
k3sctl prepare
```

Запрет паролей пишется в `/etc/ssh/sshd_config.d/00-no-password.conf`.
Имя `00-*` читается раньше провайдерского `50-cloud-init.conf`, а у sshd
побеждает первое значение, поэтому правило не слетает после перезагрузки.
Для верности в `/etc/cloud/cloud.cfg.d/` кладётся `ssh_pwauth: false`.

### 2. Поставить k3s

Master: `K3S_ROLE=server`. В сертификат API попадут адреса из `K3S_TLS_SAN`
(по умолчанию `SSH_HOST`), они пишутся в `/etc/rancher/k3s/config.yaml`.
Пользователю `SSH_USER` выдаётся `sudo -n kubectl`.

Worker: `K3S_ROLE=agent` и `K3S_SERVER_SSH_HOST` (SSH-адрес master). Токен
кластера программа сама читает с master по SSH, на вашу машину он не
сохраняется. Можно вместо этого задать `K3S_TOKEN` явно.

```bash
k3sctl install
```

Откройте на файрволе порт 6443 только для своих адресов.

### 3. Получить доступ

```bash
k3sctl access
```

Создаёт ServiceAccount `SA_NAMESPACE/SA_NAME` с ролью `CLUSTER_ROLE`,
выпускает токен (бессрочный через секрет, либо на срок `SA_TOKEN_TTL`,
например `8760h`), берёт корневой сертификат кластера из конфигмапа
`kube-root-ca.crt` и записывает кластер, пользователя и контекст
`CONTEXT_NAME` в `LOCAL_KUBECONFIG`. Другие контексты не трогает. В конце
подключается к кластеру и печатает, под кем вошли. Повторный запуск
безопасен: учётка та же, токен обновляется.

Проверка:

```bash
kubectl --context my-k3s get nodes
```

## Устройство

- `cmd/k3sctl` — точка входа; `internal/app` — разбор команд и ключей.
- `internal/command` — сами команды prepare / install / access.
- `internal/remote` — серверные скрипты на POSIX `sh` и шаблон cloud-init,
  встроенные в бинарник. Скрипт уезжает на сервер по SSH целиком вместе с
  настройками (строки `ИМЯ='значение'` в начале), поэтому на сервере ничего
  заранее класть не нужно.
- `internal/sshx` — запуск через системный `ssh`.
- `internal/kubecfg` — запись в kubeconfig и проверка доступа через client-go.
- `internal/config` — настройки из `.env`.

## Проверки

```bash
make check   # gofmt, go vet, unit-тесты
make e2e     # docker-стенд: свежий alpine + sshd + k3s, все три команды
```

Подробности стенда в `CLAUDE.md`.
