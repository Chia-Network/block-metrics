package cmd

import (
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/chia-network/block-metrics/internal/metrics"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Starts the metrics server",
	Run: func(cmd *cobra.Command, args []string) {
		mets := newMetsHelper()

		go startWebsocket(mets)

		// Close the websocket when the app is closing
		// @TODO need to actually listen for a signal and call this then, otherwise it doesn't actually get called
		defer func(m *metrics.Metrics) {
			log.Println("App is stopping. Cleaning up...")
			err := m.CloseWebsocket()
			if err != nil {
				log.Errorf("Error closing websocket connection: %s\n", err.Error())
			}
		}(mets)

		ignoreAddresses := viper.GetStringSlice("adjusted-ignore-addresses")
		if len(ignoreAddresses) > 0 {
			log.Println("Ignoring the following addresses when calculating adjusted NC")
			for _, _ignore := range ignoreAddresses {
				log.Printf(" - %s\n", _ignore)
			}
		}

		log.Fatalln(mets.StartServer())
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func startWebsocket(m *metrics.Metrics) {
	// Loop until we get a connection or cancel
	// This enables starting the metrics exporter even if the chia RPC service is not up/responding
	// It just retries every 5 seconds to connect to the RPC server until it succeeds or the app is stopped
	for {
		err := m.OpenWebsocket()
		if err != nil {
			log.Errorln(err.Error())
			time.Sleep(5 * time.Second)
			continue
		}
		break
	}
}
