package remote

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShellQuote(t *testing.T) {
	assert.Equal(t, `'abc'`, ShellQuote("abc"))
	assert.Equal(t, `''`, ShellQuote(""))
	assert.Equal(t, `'it'\''s'`, ShellQuote("it's"))
	assert.Equal(t, `'a b $c "d"'`, ShellQuote(`a b $c "d"`))
}

func TestScript(t *testing.T) {
	s, err := Script(ScriptSetupSA, map[string]string{
		"SA_NAME":      "mdb-admin",
		"CLUSTER_ROLE": "cluster-admin",
		"SA_TOKEN_TTL": "",
	})
	require.NoError(t, err)

	lines := strings.SplitN(s, "\n", 4)
	assert.Equal(t, "CLUSTER_ROLE='cluster-admin'", lines[0], "переменные идут по алфавиту")
	assert.Equal(t, "SA_NAME='mdb-admin'", lines[1])
	assert.Equal(t, "SA_TOKEN_TTL=''", lines[2])
	assert.Contains(t, lines[3], "set -eu", "дальше тело скрипта")
	assert.Contains(t, s, "kube-root-ca.crt")
}

func TestScript_Unknown(t *testing.T) {
	_, err := Script("nope.sh", nil)
	require.Error(t, err)
}

func TestCloudInit(t *testing.T) {
	s, err := CloudInit(CloudInitParams{SSHUser: "user", SSHPubkeys: []string{"ssh-ed25519 AAAA test", "ssh-rsa BBBB colleague"}, Timezone: "Asia/Almaty", RebootCron: "0 4 * * 0", JournalMaxUse: "500M"})
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(s, "#cloud-config\n"))
	assert.Contains(t, s, "\ntimezone: Asia/Almaty\n")
	assert.Contains(t, s, "\n  - name: user\n")
	assert.Contains(t, s, "\n      - ssh-ed25519 AAAA test\n      - ssh-rsa BBBB colleague\n", "все ключи по порядку")
	assert.Contains(t, s, "\n  - path: /etc/cron.d/k3sctl-reboot\n")
	assert.Contains(t, s, "\n      0 4 * * 0 root [ -f /var/run/reboot-required ] && /sbin/shutdown -r now")
	assert.Contains(t, s, "\n  - path: /etc/systemd/journald.conf.d/90-k3sctl.conf\n")
	assert.Contains(t, s, "\n      SystemMaxUse=500M\n")
	assert.Contains(t, s, "\n  - systemctl restart systemd-journald")

	s, err = CloudInit(CloudInitParams{SSHUser: "user", SSHPubkeys: []string{"ssh-ed25519 AAAA"}})
	require.NoError(t, err)
	assert.NotContains(t, s, "timezone:", "без часового пояса строки нет")
	assert.NotContains(t, s, "k3sctl-reboot", "без расписания задания нет")
	assert.NotContains(t, s, "journald", "без потолка журнала настройки нет")
	assert.Contains(t, s, "\n  - path: /etc/needrestart/conf.d/k3s.conf\n", "защита k3s от перезапуска при обновлении пакетов")
}

func TestCloudInit_Password(t *testing.T) {
	s, err := CloudInit(CloudInitParams{SSHUser: "user", SSHPubkeys: []string{"ssh-ed25519 AAAA"}, DisablePassword: true})
	require.NoError(t, err)
	assert.Contains(t, s, "\nssh_pwauth: false\ndisable_root: true\n")
	assert.Contains(t, s, "/etc/ssh/sshd_config.d/00-no-password.conf")

	s, err = CloudInit(CloudInitParams{SSHUser: "user", SSHPubkeys: []string{"ssh-ed25519 AAAA"}})
	require.NoError(t, err)
	assert.Contains(t, s, "\nssh_pwauth: true\n")
	assert.NotContains(t, s, "disable_root")
	assert.NotContains(t, s, "00-no-password.conf")
}
