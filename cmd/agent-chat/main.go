package main

import (
	"fmt"
	"os"

	"agent-toolkit/internal/chatcli"
)

func main() {
	if err := chatcli.Execute(); err != nil {
		fmt.Println(chatcli.FormatErrorJSON(err))
		os.Exit(1)
	}
}
