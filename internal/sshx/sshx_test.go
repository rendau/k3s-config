package sshx

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTarget_args(t *testing.T) {
	tg := Target{Host: "1.2.3.4", User: "user"}
	assert.Equal(t, []string{
		"-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=15",
		"-p", "22", "-o", "BatchMode=yes", "user@1.2.3.4",
	}, tg.args())

	tg = Target{Host: "h", Port: 2222, User: "root", Key: "/k/id", ExtraOpts: "-o LogLevel=ERROR -J bastion"}
	assert.Equal(t, []string{
		"-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=15",
		"-p", "2222", "-o", "BatchMode=yes", "-i", "/k/id", "-o", "IdentitiesOnly=yes",
		"-o", "LogLevel=ERROR", "-J", "bastion", "root@h",
	}, tg.args())
	assert.Equal(t, "root@h:2222", tg.String())

	tg = Target{Host: "h", User: "root", Key: "/k/id", Password: "secret"}
	assert.Equal(t, []string{
		"-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=15",
		"-p", "22", "-o", "PubkeyAuthentication=no",
		"-o", "PreferredAuthentications=password,keyboard-interactive",
		"-o", "NumberOfPasswordPrompts=1", "root@h",
	}, tg.args(), "с паролем: без BatchMode и без ключа")
}

func TestRemoteRunner(t *testing.T) {
	assert.Contains(t, remoteRunner(false, false), `{ sh "$f"; }`)
	assert.Contains(t, remoteRunner(true, false), `sudo -n sh "$f"`)

	withPassword := remoteRunner(true, true)
	assert.Contains(t, withPassword, `IFS= read -r p; f=$(mktemp)`, "пароль — первая строка stdin, до скрипта")
	assert.Contains(t, withPassword, `sudo -S -p '' sh "$f"`)
	assert.NotContains(t, withPassword, "sudo -n")
}

func TestCommand_Password(t *testing.T) {
	tg := Target{Host: "h", User: "root", Password: "s3cret"}
	cmd, cleanup, err := tg.command(t.Context(), "true")
	require.NoError(t, err)

	env := map[string]string{}
	for _, kv := range cmd.Env {
		if k, v, ok := bytes.Cut([]byte(kv), []byte("=")); ok {
			env[string(k)] = string(v)
		}
	}
	assert.Equal(t, "force", env["SSH_ASKPASS_REQUIRE"])
	assert.NotEmpty(t, env["SSH_ASKPASS"], "помощник — сам k3sctl")
	path := env[AskpassEnv]
	require.NotEmpty(t, path)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	pw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "s3cret", string(pw))

	cleanup()
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "файл пароля удалён")

	tg = Target{Host: "h", User: "root"}
	cmd, _, err = tg.command(t.Context(), "true")
	require.NoError(t, err)
	assert.Nil(t, cmd.Env, "без пароля окружение не трогаем")
}

func TestAskpass(t *testing.T) {
	var out bytes.Buffer
	t.Setenv(AskpassEnv, "")
	ok, err := Askpass(&out)
	require.NoError(t, err)
	assert.False(t, ok, "без переменной — обычный запуск")

	path := filepath.Join(t.TempDir(), "pw")
	require.NoError(t, os.WriteFile(path, []byte("p@ss\n"), 0o600))
	t.Setenv(AskpassEnv, path)
	ok, err = Askpass(&out)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "p@ss\n", out.String())
}
