package main

import (
	"os"

	"github.com/rendau/k3s-config/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:]))
}
