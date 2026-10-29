package command

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/rendau/k3s-config/internal/config"
	"github.com/rendau/k3s-config/internal/remote"
	"github.com/rendau/k3s-config/internal/sshx"
	"github.com/rendau/k3s-config/internal/ui"
)

// CloudInit записывает в файл path user-data для cloud-init: та же подготовка
// сервера, что делает Prepare, но силами провайдера при создании виртуалки.
// Существующий файл перезаписывает — он целиком выводится из настроек.
func CloudInit(c *config.Config, path string) error {
	if c.SSHUser == "" {
		return config.Missing("SSH_USER")
	}
	pubkeys, err := loadPubkeys(c)
	if err != nil {
		return err
	}

	text, err := remote.CloudInit(remote.CloudInitParams{
		SSHUser:       c.SSHUser,
		SSHPubkeys:    pubkeys,
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

const (
	rebootWait  = 3 * time.Minute
	rebootPause = 10 * time.Second // даём серверу уйти в перезагрузку, иначе первая проверка попадёт в ещё живую систему
	rebootRetry = 5 * time.Second
)

// waitAfterReboot ждёт, пока сервер после перезагрузки снова пустит по ssh.
func waitAfterReboot(ctx context.Context, t sshx.Target) error {
	deadline := time.Now().Add(rebootWait)
	if err := sleepCtx(ctx, rebootPause); err != nil {
		return err
	}
	for {
		if _, err := t.Output(ctx, "true"); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("сервер не вернулся после перезагрузки за %s", rebootWait)
		}
		if err := sleepCtx(ctx, rebootRetry); err != nil {
			return err
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// bootstrap выбирает, как зайти на сервер для подготовки, по порядку:
//  1. SSH_USER по ключу — сервер уже подготовлен, пароль не нужен;
//  2. BOOTSTRAP_USER по ключу — ключ положил провайдер или cloud-init;
//  3. BOOTSTRAP_USER по паролю из BOOTSTRAP_PASSWORD (ask = спросить).
//
// Возвращает цель и признак, что вошли по паролю.
func bootstrap(ctx context.Context, c *config.Config) (sshx.Target, bool, error) {
	probe := func(t sshx.Target, cmd string) bool {
		t.Stderr = io.Discard
		_, err := t.Output(ctx, cmd)

		return err == nil
	}

	if c.SSHUser != c.BootstrapUser {
		if t := target(c, c.SSHUser); probe(t, "sudo -n true") {
			ui.Info("%s уже заходит по ключу с sudo — продолжаю под ним", c.SSHUser)

			return t, false, nil
		}
	}
	if t := target(c, c.BootstrapUser); probe(t, "true") {
		return t, false, nil
	}
	if c.BootstrapPassword == "" {
		return sshx.Target{}, false, fmt.Errorf("не смог зайти по ключу ни как %s, ни как %s; "+
			"если провайдер дал пароль %s, укажите BOOTSTRAP_PASSWORD=ask", c.SSHUser, c.BootstrapUser, c.BootstrapUser)
	}

	pw := c.BootstrapPassword
	if pw == "ask" {
		var err error
		if pw, err = askPassword(fmt.Sprintf("пароль %s@%s: ", c.BootstrapUser, c.SSHHost)); err != nil {
			return sshx.Target{}, false, err
		}
	}
	t := target(c, c.BootstrapUser)
	t.Password = pw
	if !probe(t, "true") {
		return sshx.Target{}, false, fmt.Errorf("вход как %s по паролю не удался", c.BootstrapUser)
	}
	ui.Info("вошёл как %s по паролю; положу ему ключ, дальше пароль не понадобится", c.BootstrapUser)

	return t, true, nil
}

// askPassword спрашивает пароль в терминале скрытым вводом.
func askPassword(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", fmt.Errorf("BOOTSTRAP_PASSWORD=ask, но терминала нет: впишите пароль в файл или запустите в терминале")
	}
	fmt.Fprint(os.Stderr, prompt)
	pw, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("чтение пароля: %w", err)
	}
	if len(pw) == 0 {
		return "", fmt.Errorf("пустой пароль")
	}

	return string(pw), nil
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
	pubkeys, err := loadPubkeys(c)
	if err != nil {
		return err
	}
	switch c.PrepareReboot {
	case "auto", "true", "false":
	default:
		return fmt.Errorf("PREPARE_REBOOT=%q: допустимо auto, true или false", c.PrepareReboot)
	}

	var script string
	boot, viaPassword, err := bootstrap(ctx, c)
	if err != nil {
		return err
	}
	// При входе по паролю у root ключа нет — кладём, чтобы после запрета
	// паролей root остался доступен по ключу (PermitRootLogin prohibit-password).
	script, err = remote.Script(remote.ScriptPrepareServer, map[string]string{
		"SSH_USER":                 c.SSHUser,
		"SSH_PUBKEY_CONTENT":       strings.Join(pubkeys, "\n"),
		"PREPARE_UPGRADE":          strconv.FormatBool(c.PrepareUpgrade),
		"PREPARE_DISABLE_PASSWORD": strconv.FormatBool(c.PrepareDisablePassword),
		"SERVER_TIMEZONE":          c.ServerTimezone,
		"PREPARE_REBOOT_CRON":      c.PrepareRebootCron,
		"JOURNAL_MAX_USE":          c.JournalMaxUse,
		"PREPARE_REBOOT":           c.PrepareReboot,
		"ROOT_KEY":                 strconv.FormatBool(viaPassword),
	})
	if err != nil {
		return err
	}

	ui.Info("подготавливаю %s (вход как %s, пользователь %s)", c.SSHHost, boot.User, c.SSHUser)
	out, err := boot.RunScript(ctx, script, true)
	if err != nil {
		return err
	}

	user := target(c, c.SSHUser)
	if strings.Contains(out, "REBOOT=yes") {
		ui.Info("сервер перезагружается, жду его возвращения (до %s)", rebootWait)
		if err = waitAfterReboot(ctx, user); err != nil {
			return err
		}
		ui.Info("сервер снова в сети")
	}

	ui.Info("проверяю вход как %s по ключу и sudo без пароля", c.SSHUser)
	if _, err = user.Output(ctx, "sudo -n true"); err != nil {
		return fmt.Errorf("вход как %s или sudo не работает: %w", c.SSHUser, err)
	}

	ui.Info("готово: ssh -p %d %s@%s", c.SSHPort, c.SSHUser, c.SSHHost)

	return nil
}
