package main

import (
	"fmt"
	"os"

	"github.com/openkvlab/boltdb/cmd/boltdb/command"
)

func main() {
	rootCmd := command.NewRootCommand()
	if err := rootCmd.Execute(); err != nil {
		if rootCmd.SilenceErrors {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		} else {
			os.Exit(1)
		}
	}
}
