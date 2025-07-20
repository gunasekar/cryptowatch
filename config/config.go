package config

import (
	"fmt"
	"strings"

	"github.com/gunasekar/cryptowatch/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func InitConfig(rootCmd *cobra.Command) {
	// Define CLI flags
	rootCmd.PersistentFlags().StringSlice("coins", []string{}, "List of coin IDs to monitor (e.g., 1,1027,825)")
	rootCmd.PersistentFlags().StringSlice("push-tokens", []string{}, "List of Pushbullet API tokens")
	rootCmd.PersistentFlags().Float64("drop-threshold", 5.0, "Price drop percentage threshold for alerts")
	rootCmd.PersistentFlags().Float64("rise-threshold", 10.0, "Price rise percentage threshold for alerts")
	rootCmd.PersistentFlags().String("base-prices", "", "Base prices for coins in format coinID=price,coinID2=price2")
	rootCmd.PersistentFlags().Bool("debug", false, "Enable debug mode")
	rootCmd.PersistentFlags().String("websocket-url", "wss://push.coinmarketcap.com/ws?device=web&client_source=coin_detail_page", "WebSocket URL")

	// Bind CLI flags to viper
	viper.BindPFlag("coins", rootCmd.PersistentFlags().Lookup("coins"))
	viper.BindPFlag("push-tokens", rootCmd.PersistentFlags().Lookup("push-tokens"))
	viper.BindPFlag("drop-threshold", rootCmd.PersistentFlags().Lookup("drop-threshold"))
	viper.BindPFlag("rise-threshold", rootCmd.PersistentFlags().Lookup("rise-threshold"))
	viper.BindPFlag("base-prices", rootCmd.PersistentFlags().Lookup("base-prices"))
	viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))
	viper.BindPFlag("websocket-url", rootCmd.PersistentFlags().Lookup("websocket-url"))

	// Set environment variable prefix
	viper.SetEnvPrefix("CRYPTOWATCH")
	viper.AutomaticEnv()

	// Replace hyphens with underscores in env var names
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	// Set default values
	viper.SetDefault("drop-threshold", 5.0)
	viper.SetDefault("rise-threshold", 10.0)
	viper.SetDefault("websocket-url", "wss://push.coinmarketcap.com/ws?device=web&client_source=coin_detail_page")
	viper.SetDefault("debug", false)
}

func LoadConfig() (*types.Config, error) {
	// Get values from viper
	coinIDs := viper.GetStringSlice("coins")
	pushTokens := viper.GetStringSlice("push-tokens")
	dropThreshold := viper.GetFloat64("drop-threshold")
	riseThreshold := viper.GetFloat64("rise-threshold")
	basePricesStr := viper.GetString("base-prices")
	debug := viper.GetBool("debug")
	websocketURL := viper.GetString("websocket-url")

	// Handle base prices from flags or environment variable
	basePriceMap := make(map[string]float64)
	if basePricesStr != "" {
		pairs := strings.Split(basePricesStr, ",")
		for _, pair := range pairs {
			parts := strings.Split(pair, "=")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid base price format. Expected 'coinID=price', got '%s'", pair)
			}
			var price float64
			if _, err := fmt.Sscanf(parts[1], "%f", &price); err != nil {
				return nil, fmt.Errorf("invalid price value for coin %s: %v", parts[0], err)
			}
			basePriceMap[parts[0]] = price
		}
	}

	// Validate required fields
	if len(coinIDs) == 0 {
		return nil, fmt.Errorf("no coin IDs provided. Use --coins flag or CRYPTOWATCH_COINS environment variable")
	}

	if len(pushTokens) == 0 {
		return nil, fmt.Errorf("no push tokens provided. Use --push-tokens flag or CRYPTOWATCH_PUSH_TOKENS environment variable")
	}

	// Clean up coin IDs (trim whitespace)
	for i, coinID := range coinIDs {
		coinIDs[i] = strings.TrimSpace(coinID)
	}

	// Clean up push tokens (trim whitespace)
	for i, token := range pushTokens {
		pushTokens[i] = strings.TrimSpace(token)
	}

	return &types.Config{
		CoinIDs:       coinIDs,
		PushTokens:    pushTokens,
		WebsocketURL:  websocketURL,
		DropThreshold: dropThreshold,
		RiseThreshold: riseThreshold,
		BasePrices:    basePriceMap,
		Debug:         debug,
	}, nil
}
