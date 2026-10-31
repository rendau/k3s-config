#cloud-config
# user-data для нового сервера (Ubuntu/Debian), напечатано командой
# k3sctl cloud-init <имя>. Вставляется в поле user-data (cloud-init)
# при создании виртуалки ЦЕЛИКОМ: без первой строки «#cloud-config»
# cloud-init молча пропустит весь файл.
{{- if .Timezone }}
timezone: {{ .Timezone }}
{{- end }}
users:
  - name: {{ .SSHUser }}
    groups: [sudo]
    shell: /bin/bash
    sudo: "ALL=(ALL) NOPASSWD:ALL"
    lock_passwd: true
    ssh_authorized_keys:
{{- range .SSHPubkeys }}
      - {{ . }}
{{- end }}
{{- if .DisablePassword }}
ssh_pwauth: false
disable_root: true
{{- else }}
ssh_pwauth: true
{{- end }}
package_update: true
package_upgrade: true
packages: [sudo, curl, ca-certificates, bash-completion, cron]
write_files:
  # Лимиты inotify: fluent-bit и другие агенты логов следят за файлами
  # контейнеров через inotify, дефолтных лимитов не хватает.
  - path: /etc/sysctl.d/90-k3s.conf
    content: |
      fs.inotify.max_user_instances = 512
      fs.inotify.max_user_watches = 524288
  # needrestart не должен перезапускать k3s после обновления пакетов: у него
  # статический бинарник, а каждый перезапуск — это пропавший на время API.
  - path: /etc/needrestart/conf.d/k3s.conf
    content: |
      $nrconf{override_rc}{qr(^k3s(-agent)?\.service$)} = 0;
{{- if .DisablePassword }}
  # Имя 00-* читается раньше провайдерского 50-cloud-init.conf, а у sshd
  # побеждает первое значение, поэтому запрет паролей не слетает.
  - path: /etc/ssh/sshd_config.d/00-no-password.conf
    content: |
      PasswordAuthentication no
      ChallengeResponseAuthentication no
      PermitRootLogin prohibit-password
{{- end }}
{{- if .RebootCron }}
  # Перезагрузка после обновлений по расписанию, только если система её
  # просит (/var/run/reboot-required); unattended-upgrades сам не перезагружает.
  - path: /etc/cron.d/k3sctl-reboot
    content: |
      SHELL=/bin/sh
      {{ .RebootCron }} root [ -f /var/run/reboot-required ] && /sbin/shutdown -r now "k3sctl: перезагрузка после обновлений"
{{- end }}
{{- if .JournalMaxUse }}
  # Потолок системного журнала, иначе journald растёт до 10% диска.
  - path: /etc/systemd/journald.conf.d/90-k3sctl.conf
    content: |
      [Journal]
      SystemMaxUse={{ .JournalMaxUse }}
{{- end }}
runcmd:
  - sysctl --system
{{- if .JournalMaxUse }}
  - systemctl restart systemd-journald
{{- end }}
  - systemctl restart ssh || systemctl restart sshd
