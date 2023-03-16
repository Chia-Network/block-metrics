package cmd

import (
	"encoding/csv"
	"fmt"
	"os"

	"github.com/schollz/progressbar/v3"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// historicalOutputCmd represents the historicalOutput command
var historicalOutputCmd = &cobra.Command{
	Use:   "historical-output",
	Short: "Generates a CSV of historical NC data",
	Run: func(cmd *cobra.Command, args []string) {
		mets := newMetsHelper()

		oldest, err := mets.GetOldestBlock()
		if err != nil {
			log.Printf("Error getting oldest block: %s\n", err.Error())
			return
		}
		log.Printf("Oldest block in the DB is %d\n", oldest)

		newest, err := mets.GetNewestBlock()
		if err != nil {
			log.Printf("Error getting newest block: %s\n", err.Error())
			return
		}
		log.Printf("Newest block in the DB is %d\n", newest)

		startBlock := oldest + mets.LookbackWindow()
		log.Printf("Starting historical data at block %d\n", startBlock)

		file, err := os.Create("history.csv")
		if err != nil {
			log.Fatalln(err.Error())
		}

		writer := csv.NewWriter(file)
		defer writer.Flush()

		err = writer.Write([]string{"height", "date", "nc50", "nc51"})
		if err != nil {
			log.Fatalln(err.Error())
		}

		bar := progressbar.Default(int64(newest - startBlock))

		for {
			newestBlock, err := mets.GetNewestBlock()
			if err != nil {
				log.Printf("Error getting newest block: %s\n", err.Error())
				return
			}
			if newestBlock < startBlock {
				break
			}

			nc50, err := mets.CalculateNakamoto(startBlock, 50)
			if err != nil {
				log.Printf("Error calculating 50%% NC for peak %d: %s\n", startBlock, err.Error())
			}
			nc51, err := mets.CalculateNakamoto(startBlock, 51)
			if err != nil {
				log.Printf("Error calculating 51%% NC for peak %d: %s\n", startBlock, err.Error())
			}

			timestamp := mets.GetNonTXBlockTimestamp(startBlock)
			err = writer.Write([]string{
				fmt.Sprintf("%d", startBlock),
				timestamp.String,
				fmt.Sprintf("%d", nc50),
				fmt.Sprintf("%d", nc51),
			})
			if err != nil {
				log.Fatalln(err.Error())
			}
			startBlock += 100
			err = bar.Add(100)
			_ = err
		}

		err = bar.Finish()
		cobra.CheckErr(err)

		log.Println("Complete!")
	},
}

func init() {
	rootCmd.AddCommand(historicalOutputCmd)
}
