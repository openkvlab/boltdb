package command

import (
	"fmt"

	"github.com/spf13/cobra"

	bolt "github.com/openkvlab/boltdb"
)

func newBucketsCommand() *cobra.Command {
	bucketsCmd := &cobra.Command{
		Use:   "buckets <boltdb-file>",
		Short: "print a list of buckets in boltdb database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return bucketsFunc(cmd, args[0])
		},
	}

	return bucketsCmd
}

func bucketsFunc(cmd *cobra.Command, dbPath string) error {
	if _, err := checkSourceDBPath(dbPath); err != nil {
		return err
	}

	// Open database.
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{
		ReadOnly:        true,
		PreLoadFreelist: true,
	})
	if err != nil {
		return err
	}
	defer db.Close()

	// Print buckets.
	return db.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, _ *bolt.Bucket) error {
			fmt.Fprintln(cmd.OutOrStdout(), string(name))
			return nil
		})
	})
}
