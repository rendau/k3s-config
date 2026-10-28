// Package config читает настройки сервера из файла формата .env.
// Файлы живут в ~/.config/k3sctl/<имя>.env, образец — example.env (встроен).
// Значения из файла имеют приоритет над переменными окружения процесса.
package config

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/caarlos0/env/v9"
	"github.com/joho/godotenv"

	"github.com/rendau/k3s_config_merge/internal/util"
)

//go:embed example.env
var example []byte

const (
	dirName = "k3sctl"
	ext     = ".env"
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

// Dir — каталог с файлами серверов: $XDG_CONFIG_HOME/k3sctl, обычно ~/.config/k3sctl
// (и на macOS тоже, а не Library/Application Support).
func Dir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = util.ExpandHome("~/.config")
	}

	return filepath.Join(base, dirName)
}

// Resolve превращает аргумент команды в путь к файлу настроек:
//   - пусто — .env в текущем каталоге;
//   - путь (есть / или \, оканчивается на .env, либо такой файл существует) — он сам;
//   - иначе имя сервера — ~/.config/k3sctl/<имя>.env.
func Resolve(arg string) string {
	switch {
	case arg == "":
		return ".env"
	case strings.ContainsAny(arg, `/\`), strings.HasSuffix(arg, ext), fileExists(arg):
		return util.ExpandHome(arg)
	default:
		return filepath.Join(Dir(), arg+ext)
	}
}

// Load читает файл настроек сервера по аргументу команды (см. Resolve).
// Переменные окружения процесса — основа, значения из файла кладутся поверх них.
func Load(arg string) (*Config, error) {
	path := Resolve(arg)

	fileVars, err := godotenv.Read(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if path == filepath.Join(Dir(), arg+ext) {
				return nil, fmt.Errorf("сервер %q не заведён: нет файла %s (создать: k3sctl init %s)", arg, path, arg)
			}

			return nil, fmt.Errorf("файл настроек не найден: %s", path)
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

// Init создаёт файл настроек нового сервера из образца и возвращает его путь.
// Существующий файл не перезаписывает.
func Init(name string) (string, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasSuffix(name, ext) {
		return "", fmt.Errorf("имя сервера должно быть простым словом, напр. zeon, а не %q", name)
	}

	path := filepath.Join(Dir(), name+ext)
	if fileExists(path) {
		return "", fmt.Errorf("сервер %q уже заведён: %s", name, path)
	}

	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return "", fmt.Errorf("каталог %s: %w", Dir(), err)
	}
	if err := os.WriteFile(path, example, 0o600); err != nil {
		return "", fmt.Errorf("запись %s: %w", path, err)
	}

	return path, nil
}

// List возвращает имена заведённых серверов (файлы *.env в Dir).
func List() ([]string, error) {
	entries, err := os.ReadDir(Dir())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}

		return nil, fmt.Errorf("каталог %s: %w", Dir(), err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ext) {
			names = append(names, strings.TrimSuffix(e.Name(), ext))
		}
	}
	sort.Strings(names)

	return names, nil
}

// Missing — ошибка «в файле настроек не задано ИМЯ».
func Missing(name string) error {
	return fmt.Errorf("в файле настроек не задано %s", name)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)

	return err == nil && !info.IsDir()
}
