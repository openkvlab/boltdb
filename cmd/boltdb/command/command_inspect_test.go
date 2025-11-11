package command_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	bolt "github.com/openkvlab/boltdb"
	"github.com/openkvlab/boltdb/cmd/boltdb/command"
	"github.com/openkvlab/boltdb/internal/btesting"
)

func TestInspect(t *testing.T) {
	pageSize := 4096
	db := btesting.MustCreateDBWithOption(t, &bolt.Options{PageSize: pageSize})
	srcPath := db.Path()
	db.Close()

	defer requireDBNoChange(t, dbData(t, db.Path()), db.Path())

	rootCmd := command.NewRootCommand()
	rootCmd.SetArgs([]string{
		"inspect", srcPath,
	})
	err := rootCmd.Execute()
	require.NoError(t, err)
}
