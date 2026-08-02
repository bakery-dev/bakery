// Package main is the entrypoint for the generated CLI application.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "{{ if .ProjectName }}{{ .ProjectName }}{{ else }}app{{ end }}",
	Short: "A brief description of your application",
	Long:  `A longer description that spans multiple lines and likely contains examples and usage of using your application.`,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("Hello from {{ if .ProjectName }}{{ .ProjectName }}{{ else }}app{{ end }}!")
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func main() {
	Execute()
}
