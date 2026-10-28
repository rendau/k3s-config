package kubecfg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

func TestUpsert(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kube", "config")
	ca := []byte("-----BEGIN CERTIFICATE-----\nAAA\n-----END CERTIFICATE-----\n")

	require.NoError(t, Upsert(path, Entry{
		Context: "one", Cluster: "one", User: "sa@one", Server: "https://1.2.3.4:6443", CA: ca, Token: "t1",
	}, false))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "новый файл создаётся с правами 0600")

	// второй контекст рядом, первый не трогаем
	require.NoError(t, Upsert(path, Entry{
		Context: "two", Cluster: "two", User: "sa@two", Server: "https://5.6.7.8:6443", CA: ca, Token: "t2",
	}, true))

	// обновление первого: новый токен
	require.NoError(t, Upsert(path, Entry{
		Context: "one", Cluster: "one", User: "sa@one", Server: "https://1.2.3.4:6443", CA: ca, Token: "t1-new",
	}, false))

	cfg, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)

	assert.Len(t, cfg.Contexts, 2)
	assert.Equal(t, "two", cfg.CurrentContext, "переключили только когда просили")
	assert.Equal(t, "https://1.2.3.4:6443", cfg.Clusters["one"].Server)
	assert.Equal(t, ca, cfg.Clusters["one"].CertificateAuthorityData)
	assert.Equal(t, "t1-new", cfg.AuthInfos["sa@one"].Token)
	assert.Equal(t, "t2", cfg.AuthInfos["sa@two"].Token)
	assert.Equal(t, "sa@one", cfg.Contexts["one"].AuthInfo)
}
