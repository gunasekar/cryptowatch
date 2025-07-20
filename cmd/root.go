package cmd

import (
	"fmt"
	"log"

	"github.com/gunasekar/cryptowatch/config"
	"github.com/gunasekar/cryptowatch/monitor"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "cryptowatch",
	Short: "A cryptocurrency price monitoring tool",
	Long: `Cryptowatch monitors cryptocurrency prices and sends notifications
when prices drop or rise beyond configured thresholds.`,
	RunE: runCryptoWatchE,
}

func init() {
	config.InitConfig(rootCmd)
}

func runCryptoWatchE(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("configuration error: %v", err)
	}

	priceMonitor := monitor.NewPriceMonitor(cfg)
	if err := priceMonitor.Start(); err != nil {
		return fmt.Errorf("monitor error: %v", err)
	}

	log.Println("Cryptowatch stopped gracefully")
	return nil
}

func Execute() error {
	return rootCmd.Execute()
}
