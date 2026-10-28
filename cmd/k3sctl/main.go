package main

import (
	"os"

	"github.com/rendau/k3s_config_merge/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:]))
}
