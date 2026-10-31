package command

import (
	"context"
	"fmt"
	"strconv"

	"github.com/rendau/k3s-config/internal/config"
	"github.com/rendau/k3s-config/internal/remote"
	"github.com/rendau/k3s-config/internal/sshx"
	"github.com/rendau/k3s-config/internal/ui"
)

const nodeTokenPath = "/var/lib/rancher/k3s/server/node-token"

// Install ставит k3s официальным установщиком. Роль K3S_ROLE: server
// (master) или agent (worker). Для agent токен кластера читается с master
// по SSH, если не задан K3S_TOKEN.
func Install(ctx context.Context, c *config.Config) error {
	if c.SSHHost == "" {
		return config.Missing("SSH_HOST")
	}
	if c.SSHUser == "" {
		return config.Missing("SSH_USER")
	}
	if c.K3sShutdownGrace < 0 || c.K3sShutdownGraceCritical < 0 || c.K3sShutdownGraceCritical > c.K3sShutdownGrace {
		return fmt.Errorf("K3S_SHUTDOWN_GRACE_CRITICAL (%d) должен быть от 0 до K3S_SHUTDOWN_GRACE (%d)",
			c.K3sShutdownGraceCritical, c.K3sShutdownGrace)
	}

	if err := checkExisting(ctx, c); err != nil {
		return err
	}

	serverURL, token, tlsSan := "", "", ""

	switch c.K3sRole {
	case "server":
		tlsSan = c.K3sTLSSan
		if tlsSan == "" {
			tlsSan = c.SSHHost
		}
	case "agent":
		var err error
		if serverURL, token, err = resolveMaster(ctx, c); err != nil {
			return err
		}
	default:
		return fmt.Errorf("K3S_ROLE должен быть server или agent, а не %q", c.K3sRole)
	}

	script, err := remote.Script(remote.ScriptInstallK3s, map[string]string{
		"K3S_ROLE":        c.K3sRole,
		"SSH_USER":        c.SSHUser,
		"K3S_VERSION":     c.K3sVersion,
		"K3S_TLS_SAN":     tlsSan,
		"K3S_EXTRA_ARGS":  c.K3sExtraArgs,
		"K3S_INSTALL_ENV": c.K3sInstallEnv,
		"K3S_SERVER_URL":  serverURL,
		"K3S_TOKEN":       token,

		"K3S_SYSTEM_RESERVED": c.K3sSystemReserved,
		"K3S_EVICTION_HARD":   c.K3sEvictionHard,
		"K3S_LOG_MAX_SIZE":    c.K3sLogMaxSize,
		"K3S_LOG_MAX_FILES":   strconv.Itoa(c.K3sLogMaxFiles),

		"K3S_SHUTDOWN_GRACE":          strconv.Itoa(c.K3sShutdownGrace),
		"K3S_SHUTDOWN_GRACE_CRITICAL": strconv.Itoa(c.K3sShutdownGraceCritical),
	})
	if err != nil {
		return err
	}

	ui.Info("ставлю k3s (%s) на %s", c.K3sRole, c.SSHHost)
	if _, err = target(c, c.SSHUser).RunScript(ctx, script, true); err != nil {
		return err
	}
	ui.Info("готово")

	return nil
}

// checkExisting смотрит, что уже стоит на сервере: на ноду с другой ролью
// install не пойдёт, а на чужую, поставленную не через k3sctl, — только с
// согласия, потому что заменит её config.yaml и службу настройками из файла.
func checkExisting(ctx context.Context, c *config.Config) error {
	info, err := inspect(ctx, target(c, c.SSHUser))
	if err != nil {
		return fmt.Errorf("%w (нужен sudo без пароля: k3sctl prepare)", err)
	}
	if info.Role == "" {
		return nil
	}
	if info.Role != c.K3sRole {
		return fmt.Errorf("на %s стоит роль %s, а в файле K3S_ROLE=%s", c.SSHHost, info.Role, c.K3sRole)
	}
	if info.Managed {
		return nil
	}

	ui.Warn("k3s на %s поставлен не через k3sctl: config.yaml и настройки службы будут заменены настройками из файла", c.SSHHost)
	if !ui.Confirm("Продолжить?") {
		return fmt.Errorf("отменено, ничего не менял")
	}

	return nil
}

// resolveMaster возвращает адрес API master и токен кластера для agent.
func resolveMaster(ctx context.Context, c *config.Config) (string, string, error) {
	if c.K3sServerURL == "" && c.K3sServerSSHHost == "" {
		return "", "", fmt.Errorf("для agent задайте K3S_SERVER_SSH_HOST или K3S_SERVER_URL")
	}

	serverURL := c.K3sServerURL
	if serverURL == "" {
		serverURL = fmt.Sprintf("https://%s:6443", c.K3sServerSSHHost)
	}

	if c.K3sToken != "" {
		return serverURL, c.K3sToken, nil
	}
	if c.K3sServerSSHHost == "" {
		return "", "", fmt.Errorf("для agent задайте K3S_TOKEN или K3S_SERVER_SSH_HOST")
	}

	master := masterTarget(c)

	ui.Info("читаю токен кластера с master %s", master)
	token, err := master.Output(ctx, "sudo -n cat "+nodeTokenPath)
	if err != nil {
		return "", "", err
	}
	if token == "" {
		return "", "", fmt.Errorf("на master %s пустой токен в %s", master, nodeTokenPath)
	}

	return serverURL, token, nil
}

// masterTarget собирает параметры SSH-подключения к master для agent.
func masterTarget(c *config.Config) sshx.Target {
	user := c.K3sServerSSHUser
	if user == "" {
		user = c.SSHUser
	}

	return sshx.Target{
		Host:      c.K3sServerSSHHost,
		Port:      c.K3sServerSSHPort,
		User:      user,
		Key:       c.SSHKey,
		ExtraOpts: c.SSHExtraOpts,
	}
}
