# Тестовый стенд: нода как настоящая, Ubuntu с systemd, sshd и sudo по паролю.
# k3s здесь нет: run_adopt_tests.sh ставит его руками, как на кластере, который
# настраивали без k3sctl.
FROM ubuntu:24.04

ENV container=docker
RUN apt-get update \
 && apt-get install -y --no-install-recommends systemd systemd-sysv dbus openssh-server \
      sudo curl ca-certificates iproute2 iptables kmod procps needrestart cron bash-completion \
 && rm -rf /var/lib/apt/lists/*
RUN ssh-keygen -A \
 && systemctl enable ssh \
 && useradd -m -s /bin/bash -G sudo user

STOPSIGNAL SIGRTMIN+3
CMD ["/lib/systemd/systemd"]
