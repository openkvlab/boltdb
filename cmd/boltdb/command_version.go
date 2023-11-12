package main

import (
	"fmt"
	"runtime"

	"github.com/openkvlab/boltdb/version"
	"github.com/spf13/cobra"
)

func newVersionCobraCommand() *cobra.Command {
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "print the current version of boltdb",
		Long:  "print the current version of boltdb",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("boltdb Version: %s\n", version.Version)
			fmt.Printf("Go Version: %s\n", runtime.Version())
			fmt.Printf("Go OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		},
	}

	return versionCmd
}
