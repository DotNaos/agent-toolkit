package main

import (
	"fmt"
	"os"

	"agent-toolkit/internal/memorycli"
)

func main() {
	if err := memorycli.Execute(); err != nil {
		fmt.Println(memorycli.FormatErrorJSON(err))
		os.Exit(1)
	}
}
