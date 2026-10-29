// Package app — разбор аргументов командной строки и запуск команд.
package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rendau/k3s-config/internal/command"
	"github.com/rendau/k3s-config/internal/config"
	"github.com/rendau/k3s-config/internal/constant"
	"github.com/rendau/k3s-config/internal/k3s"
	"github.com/rendau/k3s-config/internal/ui"
)

const usageText = `k3sctl — новый сервер с k3s: подготовить, поставить k3s, получить доступ.

Порядок для нового сервера (здесь он назван zeon):

  k3sctl init zeon          завести сервер: создаёт servers/zeon.env, его нужно заполнить
  k3sctl cloud-init zeon    записать servers/zeon.cloud-init.yaml: user-data для поля
                            cloud-init при создании виртуалки
  k3sctl prepare zeon       ...или подготовить уже созданный сервер по SSH (под root)
  k3sctl install zeon       поставить k3s (K3S_ROLE в файле: server или agent)
  k3sctl access zeon        завести учётку и записать контекст в ~/.kube/config

Ещё:

  k3sctl list               какие серверы заведены (папка servers/)
  k3sctl version

Имя сервера — это файл servers/<имя>.env в текущей папке. Вместо имени
можно дать путь к файлу настроек.
`

// Run выполняет команду по аргументам (без имени программы) и возвращает код выхода.
func Run(args []string) int {
	if len(args) == 0 || isHelp(args[0]) {
		printUsage(os.Stdout)

		return 0
	}
	name, rest := args[0], args[1:]
	if len(rest) > 1 || (len(rest) == 1 && isHelp(rest[0])) {
		printUsage(os.Stderr)

		return 2
	}
	arg := ""
	if len(rest) == 1 {
		arg = rest[0]
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error

	switch name {
	case "init":
		err = runInit(ctx, arg)
	case "list":
		err = runList()
	case "cloud-init":
		err = withConfig(arg, func(c *config.Config) error { return command.CloudInit(c, config.CloudInitPath(arg)) })
	case "prepare":
		err = withConfig(arg, func(c *config.Config) error { return command.Prepare(ctx, c) })
	case "install":
		err = withConfig(arg, func(c *config.Config) error { return command.Install(ctx, c) })
	case "access":
		err = withConfig(arg, func(c *config.Config) error { return command.Access(ctx, c) })
	case "version":
		fmt.Printf("k3sctl %s", constant.Version)
		if constant.Commit != "" {
			fmt.Printf(" (%s, %s)", constant.Commit, constant.BuiltAt)
		}
		fmt.Println()
	default:
		fmt.Fprintf(os.Stderr, "неизвестная команда: %s\n\n", name)
		printUsage(os.Stderr)

		return 2
	}

	if err != nil {
		ui.Error("%v", err)

		return 1
	}

	return 0
}

func isHelp(s string) bool {
	return s == "help" || s == "-h" || s == "--help"
}

func printUsage(w *os.File) {
	fmt.Fprint(w, usageText)
}

func withConfig(arg string, run func(*config.Config) error) error {
	conf, err := config.Load(arg)
	if err != nil {
		return err
	}

	return run(conf)
}

func runInit(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("укажите имя сервера: k3sctl init <имя>")
	}
	if err := config.CheckInit(name); err != nil {
		return err
	}

	version, err := k3s.StableVersion(ctx)
	if err != nil {
		ui.Warn("не узнал стабильную версию k3s (%v): K3S_VERSION оставляю пустым, install возьмёт последнюю", err)
	}

	path, conf, err := config.Init(name, version)
	if err != nil {
		return err
	}

	ui.Info("создан %s", path)
	if conf.SSHKey != "" {
		ui.Info("ssh-ключ: %s (и %s)", conf.SSHKey, conf.SSHPubkey)
	} else {
		ui.Warn("ssh-ключ в ~/.ssh не найден (id_rsa, id_ed25519, id_ecdsa): впишите SSH_KEY и SSH_PUBKEY")
	}
	if version != "" {
		ui.Info("версия k3s: %s (стабильная на сегодня)", version)
	}
	ui.Info("дальше: k3sctl cloud-init %s (или впишите SSH_HOST и k3sctl prepare), install, access", name)

	return nil
}

func runList() error {
	names, err := config.List()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		ui.Info("серверов нет (папка %s/); завести: k3sctl init <имя>", config.Dir)

		return nil
	}
	for _, n := range names {
		fmt.Println(n)
	}

	return nil
}
