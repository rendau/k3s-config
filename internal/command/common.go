// Package command — команды k3sctl: prepare, install, access.
package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/samber/lo"

	"github.com/rendau/k3s-config/internal/config"
	"github.com/rendau/k3s-config/internal/sshx"
	"github.com/rendau/k3s-config/internal/util"
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

// loadPubkeys возвращает публичные ключи, которые кладутся пользователю на
// сервер: основной из SSH_PUBKEY (init записывает его в файл сервера, для
// файлов вручную config подставляет SSH_KEY + ".pub") и дополнительные из
// SSH_EXTRA_PUBKEYS — через запятую, путь к файлу или сам ключ строкой.
// Повторы убираются, порядок сохраняется.
func loadPubkeys(c *config.Config) ([]string, error) {
	if c.SSHPubkey == "" {
		return nil, config.Missing("SSH_KEY / SSH_PUBKEY")
	}

	keys := []string{c.SSHPubkey}
	for _, item := range strings.Split(c.SSHExtraPubkeys, ",") {
		if item = strings.TrimSpace(item); item != "" {
			keys = append(keys, item)
		}
	}

	result := make([]string, 0, len(keys))
	for _, item := range keys {
		key, err := loadPubkey(item)
		if err != nil {
			return nil, err
		}
		result = append(result, key)
	}

	return lo.Uniq(result), nil
}

// loadPubkey — публичный ключ из строки: либо сам ключ, либо путь к файлу с ним.
func loadPubkey(item string) (string, error) {
	if isPubkey(item) {
		return item, nil
	}
	path := util.ExpandHome(item)

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
