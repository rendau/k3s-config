// Package app — разбор аргументов командной строки и запуск команд.
package app

import (
	"context"
	"errors"
	"flag"
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

Использование:
  k3sctl prepare [--env-file FILE] [--print-cloud-init]
  k3sctl install [--env-file FILE]
  k3sctl access  [--env-file FILE]
  k3sctl version

Команды:
  prepare   подготовить свежий сервер: пользователь с sudo и ssh-ключом,
            запрет входа по паролю, обновление пакетов, sysctl, часовой пояс;
            --print-cloud-init печатает то же в виде user-data для cloud-init
  install   поставить k3s официальным установщиком (K3S_ROLE: server или agent)
  access    завести ServiceAccount и записать контекст в kubeconfig

Настройки читаются из файла .env в текущем каталоге (образец .env.example),
другой файл — ключом --env-file.
`

// Run выполняет команду по аргументам (без имени программы) и возвращает код выхода.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usageText)

		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error

	switch name := args[0]; name {
	case "prepare":
		err = runPrepare(ctx, args[1:])
	case "install":
		err = runWithConfig(ctx, name, args[1:], command.Install)
	case "access":
		err = runWithConfig(ctx, name, args[1:], command.Access)
	case "version":
		fmt.Printf("k3sctl %s", constant.Version)
		if constant.Commit != "" {
			fmt.Printf(" (%s, %s)", constant.Commit, constant.BuiltAt)
		}
		fmt.Println()
	case "help", "-h", "--help":
		fmt.Print(usageText)
	default:
		fmt.Fprintf(os.Stderr, "неизвестная команда: %s\n\n%s", name, usageText)

		return 2
	}

	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		ui.Error("%v", err)

		return 1
	}

	return 0
}

// newFlagSet — общие ключи всех команд: --env-file.
func newFlagSet(name string, envFile *string) *flag.FlagSet {
	fs := flag.NewFlagSet("k3sctl "+name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(envFile, "env-file", ".env", "файл настроек")

	return fs
}

func runPrepare(ctx context.Context, args []string) error {
	var envFile string
	var printCloudInit bool

	fs := newFlagSet("prepare", &envFile)
	fs.BoolVar(&printCloudInit, "print-cloud-init", false, "напечатать user-data для cloud-init и выйти")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := noPositional(fs); err != nil {
		return err
	}

	conf, err := config.Load(envFile)
	if err != nil {
		return err
	}

	return command.Prepare(ctx, conf, command.PrepareOpts{
		PrintCloudInit: printCloudInit,
		Out:            os.Stdout,
	})
}

func runWithConfig(ctx context.Context, name string, args []string, run func(context.Context, *config.Config) error) error {
	var envFile string

	fs := newFlagSet(name, &envFile)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := noPositional(fs); err != nil {
		return err
	}

	conf, err := config.Load(envFile)
	if err != nil {
		return err
	}

	return run(ctx, conf)
}

func noPositional(fs *flag.FlagSet) error {
	if fs.NArg() > 0 {
		return fmt.Errorf("лишний аргумент: %s", fs.Arg(0))
	}

	return nil
}
