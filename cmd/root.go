package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "block-metrics",
	Short: "Stores block data in mysql + exports prometheus style metrics on block data",
	Long: `Stores block data in mysql + exports prometheus style metrics on block data.

Can both connect mysql to grafana and utilize the additional prometheus metrics generated based on the data in the DB.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	var (
		lookbackWindow int
		rpcPerPage     int
		chiaHostname   string

		dbHost string
		dbPort int
		dbUser string
		dbPass string
		dbName string
	)

	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.block-metrics.yaml)")

	rootCmd.PersistentFlags().IntVar(&lookbackWindow, "lookback-window", 32256, "How many blocks to look at when determining metrics such as nakamoto coefficient")
	rootCmd.PersistentFlags().IntVar(&rpcPerPage, "rpc-per-page", 250, "How many results to fetch in each RPC call")
	rootCmd.PersistentFlags().StringVar(&chiaHostname, "chia-hostname", "localhost", "The hostname to use when connecting to chia")
	rootCmd.PersistentFlags().StringVar(&dbHost, "db-host", "127.0.0.1", "Host or IP address of the DB instance to connect to")
	rootCmd.PersistentFlags().IntVar(&dbPort, "db-port", 3306, "Port of the database")
	rootCmd.PersistentFlags().StringVar(&dbUser, "db-user", "root", "The username to use when connecting to the DB")
	rootCmd.PersistentFlags().StringVar(&dbPass, "db-pass", "password", "The password to use when connecting to the DB")
	rootCmd.PersistentFlags().StringVar(&dbName, "db-name", "blocks", "The name of the database to connect to")

	cobra.CheckErr(viper.BindPFlag("lookback-window", rootCmd.PersistentFlags().Lookup("lookback-window")))
	cobra.CheckErr(viper.BindPFlag("rpc-per-page", rootCmd.PersistentFlags().Lookup("rpc-per-page")))
	cobra.CheckErr(viper.BindPFlag("chia-hostname", rootCmd.PersistentFlags().Lookup("chia-hostname")))
	cobra.CheckErr(viper.BindPFlag("db-host", rootCmd.PersistentFlags().Lookup("db-host")))
	cobra.CheckErr(viper.BindPFlag("db-port", rootCmd.PersistentFlags().Lookup("db-port")))
	cobra.CheckErr(viper.BindPFlag("db-user", rootCmd.PersistentFlags().Lookup("db-user")))
	cobra.CheckErr(viper.BindPFlag("db-pass", rootCmd.PersistentFlags().Lookup("db-pass")))
	cobra.CheckErr(viper.BindPFlag("db-name", rootCmd.PersistentFlags().Lookup("db-name")))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".block-metrics" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".block-metrics")
	}

	viper.SetEnvPrefix("BLOCK_METRICS")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
