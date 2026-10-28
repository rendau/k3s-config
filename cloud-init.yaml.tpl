#cloud-config
# Шаблон user-data для нового сервера (Ubuntu/Debian). Готовый файл печатает
#   ./prepare_server.sh --print-cloud-init > user-data.yaml
# Его содержимое вставляется в поле user-data при создании виртуалки.
timezone: __TIMEZONE__
users:
  - name: __SSH_USER__
    groups: [sudo]
    shell: /bin/bash
    sudo: "ALL=(ALL) NOPASSWD:ALL"
    lock_passwd: true
    ssh_authorized_keys:
      - __SSH_PUBKEY__
ssh_pwauth: false
disable_root: true
package_update: true
package_upgrade: true
packages: [sudo, curl, ca-certificates, bash-completion]
write_files:
  # Лимиты inotify: fluent-bit и другие агенты логов следят за файлами
  # контейнеров через inotify, дефолтных лимитов не хватает.
  - path: /etc/sysctl.d/90-k3s.conf
    content: |
      fs.inotify.max_user_instances = 512
      fs.inotify.max_user_watches = 524288
  # Имя 00-* читается раньше провайдерского 50-cloud-init.conf, а у sshd
  # побеждает первое значение, поэтому запрет паролей не слетает.
  - path: /etc/ssh/sshd_config.d/00-no-password.conf
    content: |
      PasswordAuthentication no
      ChallengeResponseAuthentication no
      PermitRootLogin prohibit-password
runcmd:
  - sysctl --system
  - systemctl restart ssh || systemctl restart sshd
