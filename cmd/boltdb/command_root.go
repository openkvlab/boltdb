package main

import (
	"github.com/spf13/cobra"
)

const (
	cliName        = "boltdb"
	cliDescription = "A simple command line tool for inspecting boltdb databases"
)

func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     cliName,
		Short:   cliDescription,
		Version: "dev",
	}

	rootCmd.AddCommand(
		newVersionCobraCommand(),
		newSurgeryCobraCommand(),
	)

	return rootCmd
}
