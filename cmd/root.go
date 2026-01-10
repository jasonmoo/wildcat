package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "wildcat",
	Short: "Wildcat CLI tool",
	Long:  `Wildcat is a CLI tool built for hackathon excellence.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringP("config", "c", "", "config file path")
}
