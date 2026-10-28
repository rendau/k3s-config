package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.env")
	require.NoError(t, os.WriteFile(path, []byte(`
# комментарий
SSH_HOST=1.2.3.4                 # адрес сервера
SSH_PORT=2222
SSH_USER=user
SSH_EXTRA_OPTS="-o StrictHostKeyChecking=no -o LogLevel=ERROR"
# пусто = SSH_HOST
K3S_TLS_SAN=
PREPARE_UPGRADE=false
SA_NAME=mdb-admin
`), 0o600))

	conf, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, "1.2.3.4", conf.SSHHost)
	assert.Equal(t, 2222, conf.SSHPort)
	assert.Equal(t, "user", conf.SSHUser)
	assert.Equal(t, "-o StrictHostKeyChecking=no -o LogLevel=ERROR", conf.SSHExtraOpts)
	assert.Empty(t, conf.K3sTLSSan)
	assert.False(t, conf.PrepareUpgrade)
	assert.True(t, conf.PrepareDisablePassword, "значение по умолчанию")
	assert.Equal(t, "root", conf.BootstrapUser, "значение по умолчанию")
	assert.Equal(t, "kube-system", conf.SANamespace, "значение по умолчанию")
	assert.Equal(t, "mdb-admin", conf.SAName)
}

// Образец настроек должен читаться без ошибок, и пустые значения в нём
// должны оставаться пустыми (комментарий после пустого значения godotenv
// читает как значение).
func TestLoad_Example(t *testing.T) {
	conf, err := Load("../../.env.example")
	require.NoError(t, err)

	assert.Equal(t, "1.2.3.4", conf.SSHHost)
	assert.Equal(t, "user", conf.SSHUser)
	assert.Equal(t, "server", conf.K3sRole)
	assert.Equal(t, "Asia/Almaty", conf.ServerTimezone)
	for name, value := range map[string]string{
		"SSH_EXTRA_OPTS":      conf.SSHExtraOpts,
		"K3S_VERSION":         conf.K3sVersion,
		"K3S_TLS_SAN":         conf.K3sTLSSan,
		"K3S_EXTRA_ARGS":      conf.K3sExtraArgs,
		"K3S_INSTALL_ENV":     conf.K3sInstallEnv,
		"K3S_SERVER_SSH_HOST": conf.K3sServerSSHHost,
		"K3S_SERVER_SSH_USER": conf.K3sServerSSHUser,
		"K3S_SERVER_URL":      conf.K3sServerURL,
		"K3S_TOKEN":           conf.K3sToken,
		"API_URL":             conf.APIURL,
		"SA_TOKEN_TTL":        conf.SATokenTTL,
	} {
		assert.Empty(t, value, name)
	}
}

func TestLoad_FileOverridesEnvironment(t *testing.T) {
	t.Setenv("SSH_HOST", "from-env")
	t.Setenv("SSH_USER", "env-user")

	path := filepath.Join(t.TempDir(), "test.env")
	require.NoError(t, os.WriteFile(path, []byte("SSH_HOST=from-file\n"), 0o600))

	conf, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, "from-file", conf.SSHHost, "файл важнее окружения")
	assert.Equal(t, "env-user", conf.SSHUser, "чего нет в файле — берётся из окружения")
}

func TestLoad_NotFound(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.env"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "не найден")
}
