package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendau/k3s-config/internal/config"
	"github.com/rendau/k3s-config/internal/kubecfg"
)

func TestParseInspect(t *testing.T) {
	info, err := parseInspect(`PASSWORD_AUTH=yes
ROLE=agent
ACTIVE=true
VERSION=v1.34.12+k3s1
SERVER_URL=https://10.41.41.8:6443
NODE_NAME=
ENV_KEYS=K3S_TOKEN,K3S_URL,HTTP_PROXY,
NODE=data-worker
NODE_ARGS=["agent","--kubelet-arg","maximum-dead-containers=100"]
`)
	require.NoError(t, err)
	assert.Equal(t, &serverInfo{
		Role:         "agent",
		Version:      "v1.34.12+k3s1",
		Node:         "data-worker",
		ServerURL:    "https://10.41.41.8:6443",
		PasswordAuth: "yes",
		EnvKeys:      []string{"K3S_TOKEN", "K3S_URL", "HTTP_PROXY"},
		NodeArgs:     []string{"agent", "--kubelet-arg", "maximum-dead-containers=100"},
		Active:       true,
	}, info)

	info, err = parseInspect("PASSWORD_AUTH=no\nROLE=\n")
	require.NoError(t, err)
	assert.Empty(t, info.Role, "пустой сервер")

	_, err = parseInspect("ROLE=server\nNODE_ARGS=не json\n")
	require.Error(t, err)
}

func TestSplitNodeArgs(t *testing.T) {
	res := splitNodeArgs([]string{
		"server",
		"--cluster-init",
		"--disable", "traefik",
		"--cluster-cidr=10.42.0.0/16",
		"--tls-san", "1.2.3.4",
		"--tls-san", "k3s.example.com",
		"--kubelet-arg", "maximum-dead-containers=50",
		"--kube-apiserver-arg", "audit-log-maxage=3",
		"--write-kubeconfig-mode", "644",
		"--config", "/etc/rancher/k3s/config.yaml",
		"--token", "********",
		"--datastore-endpoint", "postgres://u:p@h/db",
		"--node-label", "a=b c",
		"--node-external-ip", "********",
	})

	assert.Equal(t, []string{
		"--cluster-init", "--disable", "traefik", "--cluster-cidr", "10.42.0.0/16",
		"--kube-apiserver-arg", "audit-log-maxage=3",
	}, res.Extra, "флаги, от которых зависит кластер")
	assert.Equal(t, []string{"1.2.3.4", "k3s.example.com"}, res.TLSSan)
	assert.Equal(t, []string{"--kubelet-arg maximum-dead-containers=50"}, res.Kubelet)
	assert.Equal(t, []string{"--node-label a=b c"}, res.Skipped)
	assert.Equal(t, []string{"--token", "--datastore-endpoint", "--node-external-ip"}, res.Secrets, "значений нет нигде")

	res = splitNodeArgs([]string{"agent", "--server", "https://m:6443"})
	assert.Equal(t, "https://m:6443", res.ServerURL)
	assert.Empty(t, res.Extra)

	assert.Equal(t, nodeArgs{}, splitNodeArgs(nil))
	assert.Equal(t, nodeArgs{}, splitNodeArgs([]string{"agent"}))
}

func TestApplyServer(t *testing.T) {
	conf := &config.Config{K3sRole: "server", PrepareDisablePassword: true, ServerTimezone: "Asia/Almaty", K3sLogMaxSize: "10Mi"}
	notes := applyServer(conf, &serverInfo{
		Role:         "agent",
		Version:      "v1.34.12+k3s1",
		ServerURL:    "https://10.41.41.8:6443",
		NodeName:     "data-worker",
		PasswordAuth: "no",
		EnvKeys:      []string{"K3S_URL", "K3S_TOKEN", "HTTP_PROXY"},
		NodeArgs:     []string{"agent", "--kubelet-arg", "maximum-dead-containers=100", "--flannel-iface", "eth1"},
	})

	assert.Equal(t, "agent", conf.K3sRole)
	assert.Equal(t, "v1.34.12+k3s1", conf.K3sVersion)
	assert.Equal(t, "https://10.41.41.8:6443", conf.K3sServerURL)
	assert.Equal(t, "10.41.41.8", conf.K3sServerSSHHost)
	assert.Equal(t, "--flannel-iface eth1", conf.K3sExtraArgs)
	assert.Equal(t, "K3S_NODE_NAME=data-worker", conf.K3sInstallEnv)
	assert.True(t, conf.PrepareDisablePassword, "пароль выключен — флаг true")
	assert.Equal(t, "Asia/Almaty", conf.ServerTimezone, "стандарт не трогаем")
	assert.Equal(t, "10Mi", conf.K3sLogMaxSize, "стандарт не трогаем")
	require.Len(t, notes, 2)
	assert.Contains(t, notes[0], "maximum-dead-containers=100")
	assert.Contains(t, notes[1], "HTTP_PROXY")
	assert.NotContains(t, notes[1], "K3S_TOKEN")

	conf = &config.Config{PrepareDisablePassword: true}
	notes = applyServer(conf, &serverInfo{Role: "server", PasswordAuth: "yes", NodeArgs: []string{"server", "--cluster-init"}})
	assert.False(t, conf.PrepareDisablePassword, "пароль включён — случайно не закрываем")
	assert.Empty(t, conf.K3sServerSSHHost)
	assert.Empty(t, notes)

	conf = &config.Config{PrepareDisablePassword: true}
	notes = applyServer(conf, &serverInfo{Role: "server"})
	assert.False(t, conf.PrepareDisablePassword, "не узнали — не закрываем")
	assert.Len(t, notes, 2)
}

func TestSplitHostPort(t *testing.T) {
	host, port := splitHostPort("10.0.0.1", 22)
	assert.Equal(t, "10.0.0.1", host)
	assert.Equal(t, 22, port)

	host, port = splitHostPort("k3s.example.com:2222", 22)
	assert.Equal(t, "k3s.example.com", host)
	assert.Equal(t, 2222, port)
}

func TestAutoVersion_WorkerFollowsMaster(t *testing.T) {
	info := &serverInfo{Role: "agent", Version: "v1.34.12+k3s1"}
	nodes := []kubecfg.Node{
		{Name: "w2", Version: "v1.34.12+k3s1"},
		{Name: "m", Version: "v1.35.9+k3s1", ControlPlane: true},
	}

	assert.Equal(t, "v1.35.9+k3s1", autoVersion(t.Context(), info, nodes), "версия master, а не из канала")
}

func TestCheckCluster(t *testing.T) {
	const wanted = "v1.35.9+k3s1"
	master := func(version string) kubecfg.Node {
		return kubecfg.Node{Name: "m", Version: version, Ready: true, ControlPlane: true}
	}
	worker := kubecfg.Node{Name: "w", Version: "v1.34.12+k3s1", Ready: true}

	require.NoError(t, checkCluster(nil, "server", wanted), "кластер не виден")
	require.NoError(t, checkCluster([]kubecfg.Node{master("v1.34.12+k3s1"), worker}, "server", wanted), "master идёт первым")
	require.NoError(t, checkCluster([]kubecfg.Node{master(wanted), worker}, "agent", wanted))
	require.Error(t, checkCluster([]kubecfg.Node{master("v1.34.12+k3s1"), worker}, "agent", wanted), "worker вперёд master")

	worker.Ready = false
	require.Error(t, checkCluster([]kubecfg.Node{master(wanted), worker}, "server", wanted), "нода не в Ready")
}
