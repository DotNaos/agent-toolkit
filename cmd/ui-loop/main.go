package main

import (
	"fmt"
	"os"

	"agent-toolkit/internal/uilloopcli"
)

func main() {
	if err := uilloopcli.Execute(); err != nil {
		fmt.Println(uilloopcli.FormatErrorJSON(err))
		os.Exit(1)
	}
}
