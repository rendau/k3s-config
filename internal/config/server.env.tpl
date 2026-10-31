# Настройки k3sctl для сервера «{{.Name}}», создано командой k3sctl init.
# Все значения уже проставлены: стандарт k3sctl, а для настроенного сервера
# ещё то, без чего кластер сломается (роль, версия, адрес master, важные
# флаги). Обязателен только SSH_HOST, остальное править по необходимости.
# Комментарий пишите отдельной строкой: «X=   # текст» прочитается как
# значение «# текст».
{{- range .Notes }}
# С сервера не перенесено: {{ . }}
{{- end }}

# ---------- SSH до сервера ----------
# адрес или имя сервера (обязательно)
SSH_HOST={{.SSHHost}}
SSH_PORT={{.SSHPort}}
# обычный пользователь с sudo, его заводит k3sctl prepare / cloud-init
SSH_USER={{.SSHUser}}
# приватный и публичный ключ; init берёт первый найденный из
# ~/.ssh/id_rsa, id_ed25519, id_ecdsa (пусто = не нашёл, впишите свой)
SSH_KEY={{.SSHKey}}
SSH_PUBKEY={{.SSHPubkey}}
# дополнительные публичные ключи (коллеги, запасной) через запятую: путь
# к файлу или сам ключ строкой; prepare и cloud-init кладут их пользователю
# вместе с основным, напр. ~/.ssh/backup.pub,ssh-ed25519 AAAA... kolya
SSH_EXTRA_PUBKEYS=
# дополнительные опции ssh, напр. -o ProxyJump=bastion
SSH_EXTRA_OPTS={{.SSHExtraOpts}}

# ---------- k3sctl prepare / cloud-init ----------
# под кем заходить на сервер в первый раз
BOOTSTRAP_USER={{.BootstrapUser}}
# пароль этого пользователя: для входа, если ключа ещё нет, и для sudo, если
# он его спрашивает. Пусто = только по ключу, ask = спросить в терминале,
# иначе сам пароль. Нужен один раз: prepare положит ключ и настроит sudo без пароля
BOOTSTRAP_PASSWORD={{.BootstrapPassword}}
# обновить пакеты
PREPARE_UPGRADE={{.PrepareUpgrade}}
# запретить вход по паролю всем, включая root (и в prepare, и в cloud-init);
# false = оставить как есть. На настроенном сервере init ставит то, что нашёл
PREPARE_DISABLE_PASSWORD={{.PrepareDisablePassword}}
# часовой пояс сервера; пусто = не трогать
SERVER_TIMEZONE={{.ServerTimezone}}
# перезагрузить в конце prepare, если после обновлений это нужно:
# auto = только пока k3s ещё не установлен (свежий сервер), true = всегда,
# false = никогда, только предупредить
PREPARE_REBOOT={{.PrepareReboot}}
# перезагрузка после обновлений, только если система её просит: расписание
# cron по времени сервера (по умолчанию воскресенье 04:00); пусто = не заводить
PREPARE_REBOOT_CRON={{.PrepareRebootCron}}
# потолок системного журнала (journald), напр. 500M или 1G; пусто = не трогать
JOURNAL_MAX_USE={{.JournalMaxUse}}

# ---------- k3sctl install ----------
# server (master) или agent (worker)
K3S_ROLE={{.K3sRole}}
# init записывает текущую стабильную с update.k3s.io; пусто = последняя
# стабильная на момент install. На настроенном сервере init пишет то, что
# стоит. k3sctl upgrade сам находит следующую версию и записывает её сюда;
# впишите новее установленной, если нужна конкретная
K3S_VERSION={{.K3sVersion}}
# адреса в сертификате API через запятую, напр. k3s.example.com,1.2.3.4;
# пусто = SSH_HOST
K3S_TLS_SAN={{.K3sTLSSan}}
# дополнительные аргументы k3s, напр. --disable traefik
K3S_EXTRA_ARGS={{.K3sExtraArgs}}
# переменные установщика get.k3s.io, напр. INSTALL_K3S_CHANNEL=latest
K3S_INSTALL_ENV={{.K3sInstallEnv}}
# запас процессора и памяти для самой ОС и k3s, подам не отдаётся,
# напр. cpu=200m,memory=512Mi; пусто = по размеру машины (10% памяти,
# не меньше 256Mi и не больше 2Gi; 100m на ядро, не больше 1000m)
K3S_SYSTEM_RESERVED=
# порог, при котором kubelet выселяет поды, чтобы не задушить систему
K3S_EVICTION_HARD={{.K3sEvictionHard}}
# логи контейнеров: размер одного файла и сколько файлов хранить на контейнер
K3S_LOG_MAX_SIZE={{.K3sLogMaxSize}}
K3S_LOG_MAX_FILES={{.K3sLogMaxFiles}}
# при перезагрузке/выключении ноды kubelet сначала штатно останавливает
# поды: секунд на все поды и из них на системные; 0 = выключить
K3S_SHUTDOWN_GRACE={{.K3sShutdownGrace}}
K3S_SHUTDOWN_GRACE_CRITICAL={{.K3sShutdownGraceCritical}}
# Только для agent: откуда взять адрес master и токен кластера.
# SSH-хост master, токен читается с него через sudo
K3S_SERVER_SSH_HOST={{.K3sServerSSHHost}}
K3S_SERVER_SSH_PORT={{.K3sServerSSHPort}}
# пусто = SSH_USER
K3S_SERVER_SSH_USER=
# пусто = https://K3S_SERVER_SSH_HOST:6443
K3S_SERVER_URL={{.K3sServerURL}}
# токен кластера вместо чтения с master
K3S_TOKEN=

# ---------- k3sctl access ----------
# адрес API в kubeconfig; пусто = https://SSH_HOST:6443
API_URL=
# имя учётки (ServiceAccount) и её права
SA_NAME={{.SAName}}
SA_NAMESPACE={{.SANamespace}}
CLUSTER_ROLE={{.ClusterRole}}
# срок токена, напр. 8760h; пусто = бессрочный
SA_TOKEN_TTL=
# имя контекста в kubeconfig
CONTEXT_NAME={{.ContextName}}
LOCAL_KUBECONFIG={{.LocalKubeconfig}}
# переключить текущий контекст на новый
SET_CURRENT_CONTEXT={{.SetCurrentContext}}
