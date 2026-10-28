# Тестовый стенд: «свежий сервер» на alpine с sshd, openrc и бинарником k3s.
# Пакеты sudo/bash/bash-completion специально не ставятся: их должен
# поставить prepare_server.sh. Бинарник k3s кладётся заранее, чтобы
# официальный установщик работал с INSTALL_K3S_SKIP_DOWNLOAD=true.
FROM alpine:3.22
ARG K3S_VERSION=v1.33.5+k3s1

RUN apk add --no-cache openssh openrc ca-certificates curl \
 && ssh-keygen -A \
 && mkdir -p /etc/ssh/sshd_config.d /root/.ssh \
 && chmod 700 /root/.ssh \
 && printf 'PermitRootLogin yes\nPasswordAuthentication yes\n' > /etc/ssh/sshd_config.d/50-test.conf
RUN grep -q '^Include ' /etc/ssh/sshd_config \
 || sed -i '1i Include /etc/ssh/sshd_config.d/*.conf' /etc/ssh/sshd_config

RUN curl -fsSL -o /usr/local/bin/k3s \
      "https://github.com/k3s-io/k3s/releases/download/$(printf '%s' "$K3S_VERSION" | sed 's/+/%2B/')/k3s" \
 && chmod +x /usr/local/bin/k3s

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 22 6443
ENTRYPOINT ["/entrypoint.sh"]
