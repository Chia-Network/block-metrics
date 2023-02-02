package cmd

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// backfillBlocksCmd represents the backfillBlocks command
var backfillBlocksCmd = &cobra.Command{
	Use:   "backfill-blocks",
	Short: "Backfills block data from the chia RPC into the metrics database",
	Run: func(cmd *cobra.Command, args []string) {
		mets := newMetsHelper()

		if viper.GetBool("delete-first") {
			log.Println("Deleting block records")
			cobra.CheckErr(mets.DeleteBlockRecords())
		}

		cobra.CheckErr(mets.BackfillBlocks())
	},
}

func init() {
	var (
		deleteFirst bool
	)

	rootCmd.AddCommand(backfillBlocksCmd)

	backfillBlocksCmd.Flags().BoolVar(&deleteFirst, "delete-first", false, "Whether or not to delete the content of the table before importing")
	cobra.CheckErr(viper.BindPFlag("delete-first", backfillBlocksCmd.Flags().Lookup("delete-first")))
}
