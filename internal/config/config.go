// Package config читает настройки из файла формата .env (образец .env.example).
// Значения из файла имеют приоритет над переменными окружения процесса.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/caarlos0/env/v9"
	"github.com/joho/godotenv"
)

type Config struct {
	// SSH до сервера
	SSHHost      string `env:"SSH_HOST"`
	SSHPort      int    `env:"SSH_PORT" envDefault:"22"`
	SSHUser      string `env:"SSH_USER"`
	SSHKey       string `env:"SSH_KEY"`
	SSHPubkey    string `env:"SSH_PUBKEY"`
	SSHExtraOpts string `env:"SSH_EXTRA_OPTS"`

	// prepare: подготовка свежего сервера
	BootstrapUser          string `env:"BOOTSTRAP_USER" envDefault:"root"`
	PrepareUpgrade         bool   `env:"PREPARE_UPGRADE" envDefault:"true"`
	PrepareDisablePassword bool   `env:"PREPARE_DISABLE_PASSWORD" envDefault:"true"`
	ServerTimezone         string `env:"SERVER_TIMEZONE"`

	// install: установка k3s
	K3sRole          string `env:"K3S_ROLE" envDefault:"server"`
	K3sVersion       string `env:"K3S_VERSION"`
	K3sTLSSan        string `env:"K3S_TLS_SAN"`
	K3sExtraArgs     string `env:"K3S_EXTRA_ARGS"`
	K3sInstallEnv    string `env:"K3S_INSTALL_ENV"`
	K3sServerSSHHost string `env:"K3S_SERVER_SSH_HOST"`
	K3sServerSSHPort int    `env:"K3S_SERVER_SSH_PORT" envDefault:"22"`
	K3sServerSSHUser string `env:"K3S_SERVER_SSH_USER"`
	K3sServerURL     string `env:"K3S_SERVER_URL"`
	K3sToken         string `env:"K3S_TOKEN"`

	// access: доступ через ServiceAccount
	APIURL            string `env:"API_URL"`
	SAName            string `env:"SA_NAME"`
	SANamespace       string `env:"SA_NAMESPACE" envDefault:"kube-system"`
	ClusterRole       string `env:"CLUSTER_ROLE" envDefault:"cluster-admin"`
	SATokenTTL        string `env:"SA_TOKEN_TTL"`
	ContextName       string `env:"CONTEXT_NAME"`
	LocalKubeconfig   string `env:"LOCAL_KUBECONFIG" envDefault:"~/.kube/config"`
	SetCurrentContext bool   `env:"SET_CURRENT_CONTEXT" envDefault:"false"`
}

// Load читает файл настроек. Переменные окружения процесса — основа,
// значения из файла кладутся поверх них.
func Load(path string) (*Config, error) {
	fileVars, err := godotenv.Read(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("файл настроек не найден: %s (образец: .env.example)", path)
		}

		return nil, fmt.Errorf("чтение %s: %w", path, err)
	}

	vars := make(map[string]string, len(fileVars))
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			vars[k] = v
		}
	}
	for k, v := range fileVars {
		vars[k] = v
	}

	conf := &Config{}
	if err = env.ParseWithOptions(conf, env.Options{Environment: vars}); err != nil {
		return nil, fmt.Errorf("разбор %s: %w", path, err)
	}

	return conf, nil
}

// Missing — ошибка «в файле настроек не задано ИМЯ».
func Missing(name string) error {
	return fmt.Errorf("в файле настроек не задано %s", name)
}
