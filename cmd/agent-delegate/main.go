package main

import (
	"fmt"
	"os"

	"agent-toolkit/internal/delegatecli"
)

func main() {
	if err := delegatecli.Execute(os.Args[1:]); err != nil {
		fmt.Println(delegatecli.FormatErrorJSON(err))
		os.Exit(1)
	}
}
