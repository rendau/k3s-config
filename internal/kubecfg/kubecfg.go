// Package kubecfg записывает доступ к кластеру в kubeconfig и проверяет его.
package kubecfg

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"time"

	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/rendau/k3s-config/internal/util"
)

// Entry — что записать в kubeconfig: кластер, пользователь и контекст.
type Entry struct {
	Context string
	Cluster string
	User    string
	Server  string
	CA      []byte // PEM
	Token   string
}

// Upsert добавляет или обновляет кластер, пользователя и контекст в файле
// path, остальные записи не трогает. Файл при отсутствии создаётся с правами 0600.
func Upsert(path string, e Entry, setCurrent bool) error {
	path = util.ExpandHome(path)

	cfg, err := clientcmd.LoadFromFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("чтение %s: %w", path, err)
		}
		cfg = clientcmdapi.NewConfig()
	}

	cfg.Clusters[e.Cluster] = &clientcmdapi.Cluster{
		Server:                   e.Server,
		CertificateAuthorityData: e.CA,
	}
	cfg.AuthInfos[e.User] = &clientcmdapi.AuthInfo{Token: e.Token}
	cfg.Contexts[e.Context] = &clientcmdapi.Context{
		Cluster:  e.Cluster,
		AuthInfo: e.User,
	}
	if setCurrent {
		cfg.CurrentContext = e.Context
	}

	if err = clientcmd.WriteToFile(*cfg, path); err != nil {
		return fmt.Errorf("запись %s: %w", path, err)
	}

	return nil
}

// Access — результат проверки доступа.
type Access struct {
	Username      string
	ServerVersion string
}

// Verify подключается к кластеру контекстом context из файла path и
// возвращает, под кем вошли и какая версия у API.
func Verify(ctx context.Context, path, context string) (*Access, error) {
	path = util.ExpandHome(path)

	rc, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: path},
		&clientcmd.ConfigOverrides{CurrentContext: context},
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("контекст %s из %s: %w", context, path, err)
	}
	rc.Timeout = 20 * time.Second

	cs, err := kubernetes.NewForConfig(rc)
	if err != nil {
		return nil, fmt.Errorf("клиент kubernetes: %w", err)
	}

	ver, err := cs.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("запрос версии API %s: %w", rc.Host, err)
	}

	review, err := cs.AuthenticationV1().SelfSubjectReviews().Create(ctx, &authv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("проверка, под кем вошли: %w", err)
	}

	return &Access{
		Username:      review.Status.UserInfo.Username,
		ServerVersion: ver.GitVersion,
	}, nil
}
