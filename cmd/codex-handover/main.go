package main

import (
	"fmt"
	"os"

	"agent-toolkit/internal/handovercli"
	"agent-toolkit/internal/shared/cliio"
)

func main() {
	if err := handovercli.Execute(); err != nil {
		fmt.Println(cliio.FormatErrorJSON(err))
		os.Exit(1)
	}
}
