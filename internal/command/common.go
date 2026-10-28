// Package command — команды k3sctl: prepare, install, access.
package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/rendau/k3s_config_merge/internal/config"
	"github.com/rendau/k3s_config_merge/internal/sshx"
	"github.com/rendau/k3s_config_merge/internal/util"
)

// target собирает параметры SSH-подключения к серверу из настроек под заданным пользователем.
func target(c *config.Config, user string) sshx.Target {
	return sshx.Target{
		Host:      c.SSHHost,
		Port:      c.SSHPort,
		User:      user,
		Key:       c.SSHKey,
		ExtraOpts: c.SSHExtraOpts,
	}
}

// loadPubkey читает публичный ключ из SSH_PUBKEY (по умолчанию SSH_KEY + ".pub").
func loadPubkey(c *config.Config) (string, error) {
	path := c.SSHPubkey
	if path == "" && c.SSHKey != "" {
		path = c.SSHKey + ".pub"
	}
	if path == "" {
		return "", config.Missing("SSH_PUBKEY (путь к публичному ключу)")
	}
	path = util.ExpandHome(path)

	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("публичный ключ: %w", err)
	}
	defer f.Close()

	line := ""
	if sc := bufio.NewScanner(f); sc.Scan() {
		line = strings.TrimSpace(sc.Text())
	}
	if !isPubkey(line) {
		return "", fmt.Errorf("в %s не похоже на публичный ключ ssh", path)
	}

	return line, nil
}

func isPubkey(line string) bool {
	for _, prefix := range []string{"ssh-", "ecdsa-", "sk-"} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}

	return false
}
