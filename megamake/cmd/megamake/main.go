package main

import (
	"os"

	"github.com/megamake/megamake/internal/app/cli"
)

func main() {
	os.Exit(cli.Run(os.Args))
}
