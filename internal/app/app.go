// Package app — разбор аргументов командной строки и запуск команд.
package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rendau/k3s_config_merge/internal/command"
	"github.com/rendau/k3s_config_merge/internal/config"
	"github.com/rendau/k3s_config_merge/internal/constant"
	"github.com/rendau/k3s_config_merge/internal/ui"
)

const usageText = `k3sctl — новый сервер с k3s: подготовить, поставить k3s, получить доступ.

Порядок для нового сервера (здесь он назван zeon):

  k3sctl init zeon          завести сервер: создаёт %s/zeon.env, его нужно заполнить
  k3sctl cloud-init zeon    напечатать user-data для поля cloud-init при создании виртуалки
  k3sctl prepare zeon       ...или подготовить уже созданный сервер по SSH (под root)
  k3sctl install zeon       поставить k3s (K3S_ROLE в файле: server или agent)
  k3sctl access zeon        завести учётку и записать контекст в ~/.kube/config

Ещё:

  k3sctl list               какие серверы заведены
  k3sctl version

Вместо имени сервера можно дать путь к файлу настроек; без аргумента
берётся .env из текущего каталога.
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
		err = runInit(arg)
	case "list":
		err = runList()
	case "cloud-init":
		err = withConfig(arg, func(c *config.Config) error { return command.CloudInit(c, os.Stdout) })
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
	fmt.Fprintf(w, usageText, config.Dir())
}

func withConfig(arg string, run func(*config.Config) error) error {
	conf, err := config.Load(arg)
	if err != nil {
		return err
	}

	return run(conf)
}

func runInit(name string) error {
	if name == "" {
		return fmt.Errorf("укажите имя сервера: k3sctl init <имя>")
	}

	path, err := config.Init(name)
	if err != nil {
		return err
	}

	ui.Info("создан %s", path)
	ui.Info("заполните его и дальше: k3sctl cloud-init %s (или prepare), install, access", name)

	return nil
}

func runList() error {
	names, err := config.List()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		ui.Info("серверов пока нет (каталог %s); завести: k3sctl init <имя>", config.Dir())

		return nil
	}
	for _, n := range names {
		fmt.Println(n)
	}

	return nil
}
