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

	"github.com/rendau/k3s-config/internal/util"
)

// AskpassEnv — переменная окружения с путём к файлу пароля для режима
// помощника SSH_ASKPASS (см. Askpass).
const AskpassEnv = "K3SCTL_ASKPASS_FILE"

// Target — куда и под кем подключаться.
type Target struct {
	Host      string
	Port      int
	User      string
	Key       string // путь к приватному ключу; пусто = ключи по умолчанию / агент
	Password  string // пароль вместо ключа (первый вход под root от провайдера)
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
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=15",
		"-p", strconv.Itoa(t.port()),
	}
	if t.Password != "" {
		// Пароль отдаёт помощник (SSH_ASKPASS), ключи не пробуем.
		args = append(args,
			"-o", "PubkeyAuthentication=no",
			"-o", "PreferredAuthentications=password,keyboard-interactive",
			"-o", "NumberOfPasswordPrompts=1",
		)
	} else {
		args = append(args, "-o", "BatchMode=yes")
		if t.Key != "" {
			args = append(args, "-i", util.ExpandHome(t.Key), "-o", "IdentitiesOnly=yes")
		}
	}
	args = append(args, strings.Fields(t.ExtraOpts)...)

	return append(args, t.User+"@"+t.Host)
}

// command собирает процесс ssh. При входе по паролю ssh получает пароль
// не с клавиатуры, а от помощника: SSH_ASKPASS указывает на сам k3sctl,
// который в режиме Askpass печатает содержимое файла из AskpassEnv.
// Файл с паролем временный, 0600, удаляется функцией cleanup.
func (t Target) command(ctx context.Context, args ...string) (*exec.Cmd, func(), error) {
	cmd := exec.CommandContext(ctx, "ssh", append(t.args(), args...)...)
	cmd.Stderr = t.stderr()
	cleanup := func() {}

	if t.Password == "" {
		return cmd, cleanup, nil
	}

	self, err := os.Executable()
	if err != nil {
		return nil, nil, fmt.Errorf("путь к k3sctl для SSH_ASKPASS: %w", err)
	}
	f, err := os.CreateTemp("", "k3sctl-askpass-*")
	if err != nil {
		return nil, nil, fmt.Errorf("временный файл пароля: %w", err)
	}
	cleanup = func() { os.Remove(f.Name()) }
	if _, err = f.WriteString(t.Password); err != nil {
		f.Close()
		cleanup()

		return nil, nil, fmt.Errorf("временный файл пароля: %w", err)
	}
	if err = f.Close(); err != nil {
		cleanup()

		return nil, nil, fmt.Errorf("временный файл пароля: %w", err)
	}

	cmd.Env = append(os.Environ(),
		"SSH_ASKPASS="+self,
		"SSH_ASKPASS_REQUIRE=force",
		AskpassEnv+"="+f.Name(),
	)

	return cmd, cleanup, nil
}

// Askpass — режим помощника для ssh: если задана AskpassEnv, печатает
// пароль из указанного файла в out и возвращает true. Вызывается в самом
// начале программы, до разбора команд.
func Askpass(out io.Writer) (bool, error) {
	path := os.Getenv(AskpassEnv)
	if path == "" {
		return false, nil
	}
	pw, err := os.ReadFile(path)
	if err != nil {
		return true, err
	}
	_, err = fmt.Fprintln(out, strings.TrimRight(string(pw), "\r\n"))

	return true, err
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
	// LC_ALL=C.UTF-8: клиентский ssh (macOS) передаёт серверу свои LANG/LC_*,
	// которых там обычно нет, и apt/perl засыпают вывод предупреждениями о локали.
	remote := fmt.Sprintf(`export LC_ALL=C.UTF-8 LANG=C.UTF-8; f=$(mktemp) && cat >"$f" && { %s; }; rc=$?; rm -f "$f"; exit $rc`, runner)

	cmd, cleanup, err := t.command(ctx, remote)
	if err != nil {
		return "", err
	}
	defer cleanup()
	cmd.Stdin = strings.NewReader(script)

	var out bytes.Buffer
	cmd.Stdout = &out

	if err = cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("ssh %s: %w", t, err)
	}

	return out.String(), nil
}

// Output выполняет одну команду на сервере и возвращает её stdout без
// завершающих переводов строки.
func (t Target) Output(ctx context.Context, command string) (string, error) {
	cmd, cleanup, err := t.command(ctx, command)
	if err != nil {
		return "", err
	}
	defer cleanup()
	cmd.Stdin = nil // как ssh -n: stdin серверу не нужен

	var out bytes.Buffer
	cmd.Stdout = &out

	if err = cmd.Run(); err != nil {
		return "", fmt.Errorf("ssh %s: %s: %w", t, command, err)
	}

	return strings.TrimRight(out.String(), "\r\n"), nil
}
