package cmd

import (
	"github.com/spf13/cobra"
)

func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Work with the Constellation configuration file",
		Long:  "Generate a configuration file for Constellation.",
		Args:  cobra.ExactArgs(0),
	}

	cmd.AddCommand(newConfigGenerateCmd())
	cmd.AddCommand(newConfigFetchMeasurementsCmd())

	return cmd
}
