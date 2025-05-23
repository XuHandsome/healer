package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/childe/healer"
	"github.com/spf13/cobra"
)

var listGroupsCmd = &cobra.Command{
	Use:   "list-groups",
	Short: "list groups in kafka cluster",

	RunE: func(cmd *cobra.Command, args []string) error {
		bs, err := cmd.Flags().GetString("brokers")
		if err != nil {
			return err
		}
		clientID, err := cmd.Flags().GetString("client")
		if err != nil {
			return err
		}
		client, err := healer.NewClient(bs, clientID)
		if err != nil {
			return err
		}
		groups, err := client.ListGroups()
		if err != nil {
			return err
		}

		b, _ := json.Marshal(groups)
		fmt.Println(string(b))

		return err
	},
}

func init() {
	rootCmd.AddCommand(listGroupsCmd)
}
