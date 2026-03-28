package main

import "github.com/spf13/cobra"

var ideasCmd = &cobra.Command{
	Use:   "ideas",
	Short: "Browse idea documents",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runIdeasTUI()
	},
}

func init() {
	rootCmd.AddCommand(ideasCmd)
}
