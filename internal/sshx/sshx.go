// Package sshx выполняет команды и скрипты на сервере через системный ssh:
// так работают ~/.ssh/config, ssh-agent и ProxyJump без лишнего кода.
package sshx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/rendau/k3s_config_merge/internal/util"
)

// Target — куда и под кем подключаться.
type Target struct {
	Host      string
	Port      int
	User      string
	Key       string // путь к приватному ключу; пусто = ключи по умолчанию / агент
	ExtraOpts string // дополнительные опции ssh одной строкой, напр. "-o ProxyJump=bastion"

	// Stderr — куда идёт stderr сервера (по умолчанию os.Stderr).
	Stderr io.Writer
}

func (t Target) String() string {
	return fmt.Sprintf("%s@%s:%d", t.User, t.Host, t.port())
}

func (t Target) port() int {
	if t.Port == 0 {
		return 22
	}

	return t.Port
}

func (t Target) stderr() io.Writer {
	if t.Stderr == nil {
		return os.Stderr
	}

	return t.Stderr
}

func (t Target) args() []string {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=15",
		"-p", strconv.Itoa(t.port()),
	}
	if t.Key != "" {
		args = append(args, "-i", util.ExpandHome(t.Key), "-o", "IdentitiesOnly=yes")
	}
	args = append(args, strings.Fields(t.ExtraOpts)...)

	return append(args, t.User+"@"+t.Host)
}

// RunScript выполняет sh-скрипт на сервере и возвращает его stdout.
// Скрипт сначала целиком сохраняется во временный файл и только потом
// запускается, поэтому heredoc и чтение stdin внутри него безопасны.
// С asRoot скрипт идёт через sudo -n, если вошли не под root.
func (t Target) RunScript(ctx context.Context, script string, asRoot bool) (string, error) {
	runner := `sh "$f"`
	if asRoot {
		runner = `if [ "$(id -u)" = 0 ]; then sh "$f"; else sudo -n sh "$f"; fi`
	}
	remote := fmt.Sprintf(`f=$(mktemp) && cat >"$f" && { %s; }; rc=$?; rm -f "$f"; exit $rc`, runner)

	cmd := exec.CommandContext(ctx, "ssh", append(t.args(), remote)...)
	cmd.Stdin = strings.NewReader(script)
	cmd.Stderr = t.stderr()

	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("ssh %s: %w", t, err)
	}

	return out.String(), nil
}

// Output выполняет одну команду на сервере и возвращает её stdout без
// завершающих переводов строки.
func (t Target) Output(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "ssh", append(append([]string{"-n"}, t.args()...), command)...)
	cmd.Stderr = t.stderr()

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ssh %s: %s: %w", t, command, err)
	}

	return strings.TrimRight(string(out), "\r\n"), nil
}
