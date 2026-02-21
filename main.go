package main

import (
	"fmt"
	"os"

	"agent-toolkit/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Println(cmd.FormatErrorJSON(err))
		os.Exit(1)
	}
}
