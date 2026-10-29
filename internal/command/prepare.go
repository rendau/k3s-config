package command

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/rendau/k3s_config_merge/internal/config"
	"github.com/rendau/k3s_config_merge/internal/remote"
	"github.com/rendau/k3s_config_merge/internal/ui"
)

// CloudInit записывает в файл path user-data для cloud-init: та же подготовка
// сервера, что делает Prepare, но силами провайдера при создании виртуалки.
// Существующий файл перезаписывает — он целиком выводится из настроек.
func CloudInit(c *config.Config, path string) error {
	if c.SSHUser == "" {
		return config.Missing("SSH_USER")
	}
	pubkey, err := loadPubkey(c)
	if err != nil {
		return err
	}

	text, err := remote.CloudInit(remote.CloudInitParams{
		SSHUser:       c.SSHUser,
		SSHPubkey:     pubkey,
		Timezone:      c.ServerTimezone,
		RebootCron:    c.PrepareRebootCron,
		JournalMaxUse: c.JournalMaxUse,
	})
	if err != nil {
		return err
	}
	if err = os.WriteFile(path, []byte(text), 0o600); err != nil {
		return fmt.Errorf("запись %s: %w", path, err)
	}
	ui.Info("записан %s", path)
	ui.Info("вставьте его содержимое в поле user-data (cloud-init) при создании виртуалки целиком: первой строкой должна быть «#cloud-config», иначе cloud-init пропустит весь файл")

	return nil
}

// Prepare готовит свежий сервер: пользователь с sudo и ключом, запрет
// паролей, пакеты, sysctl, часовой пояс. Заходит под BOOTSTRAP_USER.
func Prepare(ctx context.Context, c *config.Config) error {
	if c.SSHHost == "" {
		return config.Missing("SSH_HOST")
	}
	if c.SSHUser == "" {
		return config.Missing("SSH_USER")
	}
	pubkey, err := loadPubkey(c)
	if err != nil {
		return err
	}

	script, err := remote.Script(remote.ScriptPrepareServer, map[string]string{
		"SSH_USER":                 c.SSHUser,
		"SSH_PUBKEY_CONTENT":       pubkey,
		"PREPARE_UPGRADE":          strconv.FormatBool(c.PrepareUpgrade),
		"PREPARE_DISABLE_PASSWORD": strconv.FormatBool(c.PrepareDisablePassword),
		"SERVER_TIMEZONE":          c.ServerTimezone,
		"PREPARE_REBOOT_CRON":      c.PrepareRebootCron,
		"JOURNAL_MAX_USE":          c.JournalMaxUse,
	})
	if err != nil {
		return err
	}

	ui.Info("подготавливаю %s (вход как %s, пользователь %s)", c.SSHHost, c.BootstrapUser, c.SSHUser)
	if _, err = target(c, c.BootstrapUser).RunScript(ctx, script, true); err != nil {
		return err
	}

	ui.Info("проверяю вход как %s по ключу и sudo без пароля", c.SSHUser)
	if _, err = target(c, c.SSHUser).Output(ctx, "sudo -n true"); err != nil {
		return fmt.Errorf("вход как %s или sudo не работает: %w", c.SSHUser, err)
	}

	ui.Info("готово: ssh -p %d %s@%s", c.SSHPort, c.SSHUser, c.SSHHost)

	return nil
}
