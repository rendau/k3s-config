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
	s, err := CloudInit(CloudInitParams{SSHUser: "user", SSHPubkey: "ssh-ed25519 AAAA test", Timezone: "Asia/Almaty"})
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(s, "#cloud-config\n"))
	assert.Contains(t, s, "\ntimezone: Asia/Almaty\n")
	assert.Contains(t, s, "\n  - name: user\n")
	assert.Contains(t, s, "\n      - ssh-ed25519 AAAA test\n")

	s, err = CloudInit(CloudInitParams{SSHUser: "user", SSHPubkey: "ssh-ed25519 AAAA"})
	require.NoError(t, err)
	assert.NotContains(t, s, "timezone:", "без часового пояса строки нет")
}
