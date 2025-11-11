package command

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/openkvlab/boltdb/version"
)

func newVersionCommand() *cobra.Command {
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
