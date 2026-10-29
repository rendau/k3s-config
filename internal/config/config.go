// Package config читает настройки сервера из файла формата .env.
// Файлы серверов лежат в подпапке servers/ текущего каталога: servers/<имя>.env;
// создаёт их Init по шаблону server.env.tpl (встроен), сразу подставляя
// найденный ssh-ключ и значения по умолчанию.
// Значения из файла имеют приоритет над переменными окружения процесса.
package config

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/caarlos0/env/v9"
	"github.com/joho/godotenv"

	"github.com/rendau/k3s_config_merge/internal/util"
)

//go:embed server.env.tpl
var serverTpl string

const (
	ext = ".env"
	// Dir — подпапка текущего каталога с файлами серверов.
	Dir = "servers"
)

type Config struct {
	// SSH до сервера
	SSHHost      string `env:"SSH_HOST"`
	SSHPort      int    `env:"SSH_PORT" envDefault:"22"`
	SSHUser      string `env:"SSH_USER" envDefault:"user"`
	SSHKey       string `env:"SSH_KEY"`
	SSHPubkey    string `env:"SSH_PUBKEY"`
	SSHExtraOpts string `env:"SSH_EXTRA_OPTS"`

	// prepare: подготовка свежего сервера
	BootstrapUser          string `env:"BOOTSTRAP_USER" envDefault:"root"`
	PrepareUpgrade         bool   `env:"PREPARE_UPGRADE" envDefault:"true"`
	PrepareDisablePassword bool   `env:"PREPARE_DISABLE_PASSWORD" envDefault:"true"`
	ServerTimezone         string `env:"SERVER_TIMEZONE" envDefault:"Asia/Almaty"`

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
	SAName            string `env:"SA_NAME" envDefault:"admin"`
	SANamespace       string `env:"SA_NAMESPACE" envDefault:"kube-system"`
	ClusterRole       string `env:"CLUSTER_ROLE" envDefault:"cluster-admin"`
	SATokenTTL        string `env:"SA_TOKEN_TTL"`
	ContextName       string `env:"CONTEXT_NAME"`
	LocalKubeconfig   string `env:"LOCAL_KUBECONFIG" envDefault:"~/.kube/config"`
	SetCurrentContext bool   `env:"SET_CURRENT_CONTEXT" envDefault:"false"`
}

// Resolve превращает аргумент команды в путь к файлу настроек:
//   - имя сервера — servers/<имя>.env в текущем каталоге;
//   - путь или имя файла с .env — как есть (с раскрытием ~).
func Resolve(arg string) string {
	if strings.ContainsAny(arg, `/\`) || strings.HasSuffix(arg, ext) {
		return util.ExpandHome(arg)
	}

	return filepath.Join(Dir, arg+ext)
}

// Load читает файл настроек сервера по аргументу команды (см. Resolve).
// Переменные окружения процесса — основа, значения из файла кладутся поверх них.
func Load(arg string) (*Config, error) {
	if arg == "" {
		return nil, fmt.Errorf("укажите сервер: k3sctl <команда> <имя>; заведённые показывает k3sctl list")
	}
	path := Resolve(arg)

	fileVars, err := godotenv.Read(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if path == filepath.Join(Dir, arg+ext) {
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

	conf, err := parse(vars)
	if err != nil {
		return nil, fmt.Errorf("разбор %s: %w", path, err)
	}
	conf.fillDerived(serverName(path))

	return conf, nil
}

// parse собирает Config из набора переменных; чего нет — берётся из envDefault.
func parse(vars map[string]string) (*Config, error) {
	conf := &Config{}
	if err := env.ParseWithOptions(conf, env.Options{Environment: vars}); err != nil {
		return nil, err
	}

	return conf, nil
}

// serverName — имя сервера по пути к его файлу: servers/zeon.env → zeon.
func serverName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ext)
}

// CloudInitPath — куда писать user-data для сервера: рядом с файлом настроек,
// servers/zeon.env → servers/zeon.cloud-init.yaml.
func CloudInitPath(arg string) string {
	path := Resolve(arg)

	return filepath.Join(filepath.Dir(path), serverName(path)+".cloud-init.yaml")
}

// fillDerived заполняет то, что однозначно следует из уже заданного:
// публичный ключ (SSH_KEY + .pub), имя контекста (имя сервера).
// Это страховка для файлов, написанных вручную; init записывает всё явно.
func (c *Config) fillDerived(name string) {
	if c.SSHPubkey == "" && c.SSHKey != "" {
		c.SSHPubkey = c.SSHKey + ".pub"
	}
	if c.ContextName == "" {
		c.ContextName = name
	}
}

// defaultKeys — где init ищет приватный ssh-ключ, по порядку.
var defaultKeys = []string{"~/.ssh/id_rsa", "~/.ssh/id_ed25519", "~/.ssh/id_ecdsa"}

// findKey возвращает первый существующий ключ из defaultKeys или "".
func findKey() string {
	for _, k := range defaultKeys {
		if _, err := os.Stat(util.ExpandHome(k)); err == nil {
			return k
		}
	}

	return ""
}

// Init создаёт файл servers/<имя>.env (папку заводит при необходимости):
// находит ssh-ключ в ~/.ssh, подставляет умолчания и имя контекста, так что
// человеку остаётся вписать SSH_HOST. Возвращает путь к файлу и собранные
// настройки. Существующий файл не перезаписывает.
func Init(name string) (string, *Config, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasSuffix(name, ext) {
		return "", nil, fmt.Errorf("имя сервера должно быть простым словом, напр. zeon, а не %q", name)
	}

	path := filepath.Join(Dir, name+ext)
	if _, err := os.Stat(path); err == nil {
		return "", nil, fmt.Errorf("сервер %q уже заведён: %s", name, path)
	}

	conf, err := parse(map[string]string{})
	if err != nil {
		return "", nil, fmt.Errorf("умолчания: %w", err)
	}
	conf.SSHKey = findKey()
	conf.fillDerived(name)

	content, err := render(name, conf)
	if err != nil {
		return "", nil, err
	}

	if err = os.MkdirAll(Dir, 0o700); err != nil {
		return "", nil, fmt.Errorf("папка %s: %w", Dir, err)
	}
	if err = os.WriteFile(path, content, 0o600); err != nil {
		return "", nil, fmt.Errorf("запись %s: %w", path, err)
	}

	return path, conf, nil
}

// render заполняет шаблон файла сервера.
func render(name string, conf *Config) ([]byte, error) {
	tpl, err := template.New("server.env").Parse(serverTpl)
	if err != nil {
		return nil, fmt.Errorf("шаблон файла сервера: %w", err)
	}

	var buf bytes.Buffer
	data := struct {
		Name string
		*Config
	}{name, conf}
	if err = tpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("шаблон файла сервера: %w", err)
	}

	return buf.Bytes(), nil
}

// List возвращает имена заведённых серверов (файлы servers/*.env).
func List() ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(Dir, "*"+ext))
	if err != nil {
		return nil, err
	}

	var names []string
	for _, m := range matches {
		names = append(names, strings.TrimSuffix(filepath.Base(m), ext))
	}
	sort.Strings(names)

	return names, nil
}

// Missing — ошибка «в файле настроек не задано ИМЯ».
func Missing(name string) error {
	return fmt.Errorf("в файле настроек не задано %s", name)
}
