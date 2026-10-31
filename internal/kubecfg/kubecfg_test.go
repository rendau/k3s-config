package kubecfg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestDecodeNode(t *testing.T) {
	n := decodeNode(corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "data-apps-1",
			Labels: map[string]string{"node-role.kubernetes.io/control-plane": "true"},
		},
		Status: corev1.NodeStatus{
			NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.34.12+k3s1"},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}, 0)
	assert.Equal(t, Node{Name: "data-apps-1", Version: "v1.34.12+k3s1", Ready: true, ControlPlane: true}, n)

	n = decodeNode(corev1.Node{Status: corev1.NodeStatus{
		Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}},
	}}, 0)
	assert.False(t, n.Ready)
	assert.False(t, n.ControlPlane)
}

func TestFindContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	require.NoError(t, Upsert(path, Entry{Context: "b", Cluster: "b", User: "u", Server: "https://10.0.0.1:6443"}, false))
	require.NoError(t, Upsert(path, Entry{Context: "a", Cluster: "a", User: "u", Server: "https://k3s.example.com:16443"}, false))

	assert.Equal(t, "b", FindContext(path, "10.0.0.1"))
	assert.Equal(t, "a", FindContext(path, "k3s.example.com"), "хост без порта")
	assert.Empty(t, FindContext(path, "10.0.0.2"))
	assert.Empty(t, FindContext(filepath.Join(t.TempDir(), "nope"), "10.0.0.1"), "файла нет — контекста нет")
}
