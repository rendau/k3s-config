# Настройки k3sctl для сервера «{{.Name}}», создано командой k3sctl init.
# Все значения уже проставлены: найденный ssh-ключ и умолчания. Обязательно
# заполнить только SSH_HOST, остальное править по необходимости.
# Комментарий пишите отдельной строкой: «X=   # текст» прочитается как
# значение «# текст».

# ---------- SSH до сервера ----------
# адрес или имя сервера (обязательно)
SSH_HOST=
SSH_PORT={{.SSHPort}}
# обычный пользователь с sudo, его заводит k3sctl prepare / cloud-init
SSH_USER={{.SSHUser}}
# приватный и публичный ключ; init берёт первый найденный из
# ~/.ssh/id_rsa, id_ed25519, id_ecdsa (пусто = не нашёл, впишите свой)
SSH_KEY={{.SSHKey}}
SSH_PUBKEY={{.SSHPubkey}}
# дополнительные опции ssh, напр. -o ProxyJump=bastion
SSH_EXTRA_OPTS=

# ---------- k3sctl prepare / cloud-init ----------
# под кем заходить на свежий сервер
BOOTSTRAP_USER={{.BootstrapUser}}
# обновить пакеты
PREPARE_UPGRADE={{.PrepareUpgrade}}
# запретить вход по паролю всем, включая root
PREPARE_DISABLE_PASSWORD={{.PrepareDisablePassword}}
# часовой пояс сервера; пусто = не трогать
SERVER_TIMEZONE={{.ServerTimezone}}

# ---------- k3sctl install ----------
# server (master) или agent (worker)
K3S_ROLE={{.K3sRole}}
# напр. v1.33.5+k3s1; пусто = последняя стабильная
K3S_VERSION=
# адреса в сертификате API через запятую, напр. k3s.example.com,1.2.3.4;
# пусто = SSH_HOST
K3S_TLS_SAN=
# дополнительные аргументы k3s, напр. --disable traefik
K3S_EXTRA_ARGS=
# переменные установщика get.k3s.io, напр. INSTALL_K3S_CHANNEL=latest
K3S_INSTALL_ENV=
# Только для agent: откуда взять адрес master и токен кластера.
# SSH-хост master, токен читается с него через sudo
K3S_SERVER_SSH_HOST=
K3S_SERVER_SSH_PORT={{.K3sServerSSHPort}}
# пусто = SSH_USER
K3S_SERVER_SSH_USER=
# пусто = https://K3S_SERVER_SSH_HOST:6443
K3S_SERVER_URL=
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
