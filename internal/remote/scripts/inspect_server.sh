# Серверная часть inspect_server.sh. Выполняется от root, POSIX sh, на сервере
# ничего не меняет. Печатает на stdout строки ИМЯ=значение: что за k3s стоит.
# Значение токена из файла окружения службы сюда не попадает: оттуда читаются
# только K3S_URL, K3S_NODE_NAME и имена остальных переменных.
set -u
PATH=$PATH:/usr/local/bin:/usr/sbin:/sbin

active()   { systemctl is-active -q "$1" 2>/dev/null || rc-service "$1" status >/dev/null 2>&1; }
has_unit() { [ -f "/etc/systemd/system/$1.service" ] || [ -f "/etc/init.d/$1" ]; }

role=
if active k3s; then
  role=server
elif active k3s-agent; then
  role=agent
elif has_unit k3s; then
  role=server
elif has_unit k3s-agent; then
  role=agent
fi

pw=$(sshd -T 2>/dev/null | awk '$1 == "passwordauthentication" { print $2; exit }')
echo "PASSWORD_AUTH=$pw"
echo "ROLE=$role"
[ -n "$role" ] || exit 0

svc=k3s
[ "$role" = agent ] && svc=k3s-agent
active "$svc" && echo "ACTIVE=true"
echo "VERSION=$(k3s --version 2>/dev/null | awk 'NR == 1 { print $3 }')"

cfg=/etc/rancher/k3s/config.yaml
[ -f "$cfg" ] && head -1 "$cfg" | grep -q 'Сгенерировано install_k3s.sh' && echo "MANAGED=true"
[ -d /var/lib/rancher/k3s/server/db/etcd ] && echo "ETCD=true"

envfile=
for f in "/etc/systemd/system/$svc.service.env" "/etc/rancher/k3s/$svc.env"; do
  [ -f "$f" ] && envfile=$f
done
env_value() { sed -n "s/^$1=//p" "$envfile" | head -1 | sed -e "s/^[\"']//" -e "s/[\"']\$//"; }
node_name=
if [ -n "$envfile" ]; then
  echo "SERVER_URL=$(env_value K3S_URL)"
  node_name=$(env_value K3S_NODE_NAME)
  echo "NODE_NAME=$node_name"
  echo "ENV_KEYS=$(sed -n 's/^\([A-Za-z_][A-Za-z0-9_]*\)=.*/\1/p' "$envfile" | tr '\n' ',')"
fi

# Флаги k3s берём из аннотации ноды: в ней и строка запуска службы, и
# config.yaml, а токены k3s заменяет на звёздочки.
node=${node_name:-$(hostname)}
echo "NODE=$node"
kubeconfig=
[ "$role" = agent ] && kubeconfig="--kubeconfig=/var/lib/rancher/k3s/agent/kubelet.kubeconfig"
# shellcheck disable=SC2086
echo "NODE_ARGS=$(k3s kubectl $kubeconfig get node "$node" -o jsonpath='{.metadata.annotations.k3s\.io/node-args}' 2>/dev/null)"
