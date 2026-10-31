package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initServer — то, что делает k3sctl init для ещё не созданного сервера.
func initServer(name, k3sVersion string) (string, *Config, error) {
	if err := CheckInit(name); err != nil {
		return "", nil, err
	}
	conf, err := Defaults(name)
	if err != nil {
		return "", nil, err
	}
	conf.K3sVersion = k3sVersion

	path, err := Write(name, conf, nil)

	return path, conf, err
}

// inTempDir переходит во временный каталог на время теста.
func inTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)

	return dir
}

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

// Init находит ssh-ключ и записывает файл, где все переменные активны и
// заполнены умолчаниями; файл должен читаться обратно без потерь.
func TestInit_FillsDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".ssh"), 0o700))
	inTempDir(t)

	path, conf, err := initServer("nokey", "")
	require.NoError(t, err)
	assert.Empty(t, conf.SSHKey, "ключей в ~/.ssh нет — пусто")
	assert.Empty(t, conf.SSHPubkey)
	assert.Equal(t, "nokey", conf.ContextName)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, line := range strings.Split(string(content), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		assert.Regexp(t, `^[A-Z0-9_]+=`, line, "каждая переменная активна, без #")
	}
	assert.Contains(t, string(content), "\nSSH_HOST=\n", "адрес заполняет человек")
	assert.Contains(t, string(content), "\nSSH_KEY=\n")
	assert.Contains(t, string(content), "\nCONTEXT_NAME=nokey\n")
	assert.Contains(t, string(content), "\nSERVER_TIMEZONE=Asia/Almaty\n")
	assert.Contains(t, string(content), "\nK3S_VERSION=\n", "версию не передали — пусто")
	assert.Contains(t, string(content), "\nPREPARE_REBOOT_CRON=0 4 * * 0\n")

	require.NoError(t, os.WriteFile(filepath.Join(home, ".ssh", "id_ed25519"), nil, 0o600))
	_, conf, err = initServer("ed", "")
	require.NoError(t, err)
	assert.Equal(t, "~/.ssh/id_ed25519", conf.SSHKey)
	assert.Equal(t, "~/.ssh/id_ed25519.pub", conf.SSHPubkey)

	require.NoError(t, os.WriteFile(filepath.Join(home, ".ssh", "id_rsa"), nil, 0o600))
	_, conf, err = initServer("rsa", "v1.36.5+k3s1")
	require.NoError(t, err)
	assert.Equal(t, "~/.ssh/id_rsa", conf.SSHKey, "id_rsa в приоритете")
	assert.Equal(t, "v1.36.5+k3s1", conf.K3sVersion, "версия k3s, переданная init")
	assert.Equal(t, "0 4 * * 0", conf.PrepareRebootCron, "перезагрузка по воскресеньям в 04:00")

	// то, что записано, читается обратно один в один
	loaded, err := Load("rsa")
	require.NoError(t, err)
	assert.Equal(t, conf, loaded)
	assert.Empty(t, loaded.SSHHost, "единственное, что заполняет человек")
	assert.Equal(t, "user", loaded.SSHUser)
	assert.Equal(t, 22, loaded.SSHPort)
	assert.Equal(t, "~/.ssh/id_rsa", loaded.SSHKey)
	assert.Equal(t, "~/.ssh/id_rsa.pub", loaded.SSHPubkey)
	assert.Equal(t, "v1.36.5+k3s1", loaded.K3sVersion)
	assert.Equal(t, "0 4 * * 0", loaded.PrepareRebootCron)
	assert.Equal(t, "auto", loaded.PrepareReboot)
	assert.Empty(t, loaded.K3sSystemReserved, "пусто = по размеру машины")
	assert.Equal(t, "memory.available<5%,nodefs.available<10%,imagefs.available<10%", loaded.K3sEvictionHard)
	assert.Equal(t, "10Mi", loaded.K3sLogMaxSize)
	assert.Equal(t, 3, loaded.K3sLogMaxFiles)
	assert.Equal(t, "500M", loaded.JournalMaxUse)
	assert.Equal(t, 60, loaded.K3sShutdownGrace)
	assert.Equal(t, 20, loaded.K3sShutdownGraceCritical)
	assert.Equal(t, "root", loaded.BootstrapUser)
	assert.True(t, loaded.PrepareUpgrade)
	assert.True(t, loaded.PrepareDisablePassword)
	assert.Equal(t, "Asia/Almaty", loaded.ServerTimezone)
	assert.Equal(t, "server", loaded.K3sRole)
	assert.Equal(t, "admin", loaded.SAName)
	assert.Equal(t, "kube-system", loaded.SANamespace)
	assert.Equal(t, "cluster-admin", loaded.ClusterRole)
	assert.Equal(t, "rsa", loaded.ContextName, "имя контекста = имя сервера")
	assert.Equal(t, "~/.kube/config", loaded.LocalKubeconfig)
	assert.False(t, loaded.SetCurrentContext)
	for name, value := range map[string]string{
		"SSH_EXTRA_OPTS":      loaded.SSHExtraOpts,
		"K3S_TLS_SAN":         loaded.K3sTLSSan,
		"K3S_EXTRA_ARGS":      loaded.K3sExtraArgs,
		"K3S_INSTALL_ENV":     loaded.K3sInstallEnv,
		"K3S_SERVER_SSH_HOST": loaded.K3sServerSSHHost,
		"K3S_SERVER_SSH_USER": loaded.K3sServerSSHUser,
		"K3S_SERVER_URL":      loaded.K3sServerURL,
		"K3S_TOKEN":           loaded.K3sToken,
		"API_URL":             loaded.APIURL,
		"SA_TOKEN_TTL":        loaded.SATokenTTL,
	} {
		assert.Empty(t, value, name)
	}
}

// Файл, написанный вручную: ключ при запуске не ищется, а публичный ключ и
// имя контекста выводятся из заданного.
func TestLoad_FillDerived(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".ssh"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".ssh", "id_rsa"), nil, 0o600))
	dir := inTempDir(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "zeon.env"), []byte("SSH_HOST=h\n"), 0o600))
	conf, err := Load("zeon.env")
	require.NoError(t, err)
	assert.Empty(t, conf.SSHKey, "при запуске ключ не ищется, только при init")
	assert.Empty(t, conf.SSHPubkey)
	assert.Equal(t, "zeon", conf.ContextName)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "zeon.env"), []byte("SSH_HOST=h\nSSH_KEY=~/.ssh/other\nCONTEXT_NAME=prod\n"), 0o600))
	conf, err = Load("zeon.env")
	require.NoError(t, err)
	assert.Equal(t, "~/.ssh/other", conf.SSHKey)
	assert.Equal(t, "~/.ssh/other.pub", conf.SSHPubkey, "публичный = приватный + .pub")
	assert.Equal(t, "prod", conf.ContextName)
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

// Настройки, снятые с сервера, и заметки попадают в файл и читаются обратно;
// из окружения init берёт только вход на сервер.
func TestWrite_FromServer(t *testing.T) {
	inTempDir(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SSH_PORT", "2222")
	t.Setenv("SSH_EXTRA_OPTS", "-o LogLevel=ERROR -o X=y")
	t.Setenv("K3S_ROLE", "agent")

	conf, err := Defaults("w1")
	require.NoError(t, err)
	assert.Equal(t, 2222, conf.SSHPort)
	assert.Equal(t, "server", conf.K3sRole, "K3S_* из окружения init не берёт")

	conf.SSHHost = "10.0.0.2"
	conf.BootstrapUser = "user"
	conf.BootstrapPassword = "ask"
	conf.PrepareDisablePassword = false
	conf.K3sRole = "agent"
	conf.K3sVersion = "v1.34.12+k3s1"
	conf.K3sExtraArgs = "--disable traefik --cluster-cidr 10.42.0.0/16"
	conf.K3sTLSSan = "1.2.3.4,k3s.example.com"
	conf.K3sInstallEnv = "K3S_NODE_NAME=w1"
	conf.K3sServerSSHHost = "10.0.0.1"
	conf.K3sServerURL = "https://10.0.0.1:6443"

	path, err := Write("w1", conf, []string{"ручные настройки kubelet: --kubelet-arg a=1"})
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "\n# С сервера не перенесено: ручные настройки kubelet: --kubelet-arg a=1\n")

	loaded, err := Load("w1")
	require.NoError(t, err)
	assert.Equal(t, conf, loaded)
}

func TestSetK3sVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.env")
	require.NoError(t, os.WriteFile(path, []byte("# версия\nSSH_HOST=h\nK3S_VERSION=v1.34.12+k3s1\nK3S_ROLE=agent\n"), 0o600))

	require.NoError(t, SetK3sVersion(path, "v1.35.9+k3s1"))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "# версия\nSSH_HOST=h\nK3S_VERSION=v1.35.9+k3s1\nK3S_ROLE=agent\n", string(content), "остальное не тронуто")

	require.NoError(t, os.WriteFile(path, []byte("SSH_HOST=h"), 0o600))
	require.NoError(t, SetK3sVersion(path, "v1.35.9+k3s1"))
	content, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "SSH_HOST=h\nK3S_VERSION=v1.35.9+k3s1\n", string(content), "строки не было — дописана")

	require.Error(t, SetK3sVersion(filepath.Join(t.TempDir(), "nope.env"), "v1"))
}

func TestResolve(t *testing.T) {
	assert.Equal(t, filepath.Join("servers", "zeon.env"), Resolve("zeon"), "имя сервера — файл в servers/")
	assert.Equal(t, "/tmp/x/srv.env", Resolve("/tmp/x/srv.env"), "абсолютный путь")
	assert.Equal(t, "srv.env", Resolve("srv.env"), "имя файла как есть")
	assert.Equal(t, filepath.Join(os.Getenv("HOME"), "a.env"), Resolve("~/a.env"), "~ раскрывается")
}

func TestInitListLoad(t *testing.T) {
	dir := inTempDir(t)

	names, err := List()
	require.NoError(t, err)
	assert.Empty(t, names, "пока ничего не заведено")

	path, _, err := initServer("zeon", "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("servers", "zeon.env"), path)

	info, err := os.Stat(filepath.Join(dir, "servers", "zeon.env"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	_, _, err = initServer("zeon", "")
	require.Error(t, err, "повторный init не перезаписывает")
	_, _, err = initServer("bad/name", "")
	require.Error(t, err)
	_, _, err = initServer("", "")
	require.Error(t, err)

	_, _, err = initServer("mechta", "")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile("stray.env", nil, 0o600))

	names, err = List()
	require.NoError(t, err)
	assert.Equal(t, []string{"mechta", "zeon"}, names, "файлы вне servers/ в список не входят")

	conf, err := Load("zeon")
	require.NoError(t, err)
	assert.Empty(t, conf.SSHHost, "адрес заполняет человек")
	assert.Equal(t, "zeon", conf.ContextName, "имя контекста = имя сервера")

	_, err = Load("nope")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "k3sctl init nope", "подсказка, как завести сервер")

	_, err = Load("")
	require.Error(t, err, "без имени сервера")
}

func TestCloudInitPath(t *testing.T) {
	assert.Equal(t, filepath.Join("servers", "zeon.cloud-init.yaml"), CloudInitPath("zeon"))
	assert.Equal(t, "/tmp/x/srv.cloud-init.yaml", CloudInitPath("/tmp/x/srv.env"))
}
