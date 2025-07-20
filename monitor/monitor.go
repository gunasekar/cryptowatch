package monitor

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/gunasekar/cryptowatch/client"
	"github.com/gunasekar/cryptowatch/notifier"
	"github.com/gunasekar/cryptowatch/types"
)

type PriceMonitor struct {
	coinStates map[string]*types.CoinState
	config     *types.Config
	notifier   notifier.Service
	mu         sync.RWMutex
	done       chan struct{}
}

func NewPriceMonitor(config *types.Config) *PriceMonitor {
	return &PriceMonitor{
		config:     config,
		coinStates: make(map[string]*types.CoinState),
		done:       make(chan struct{}),
	}
}

func (pm *PriceMonitor) CheckPriceChange(coinID string, currentPrice float64, change1h, change24h float64) {
	pm.mu.Lock()
	state, exists := pm.coinStates[coinID]
	if !exists {
		state = &types.CoinState{
			BasePrice: pm.config.BasePrices[coinID],
		}
		pm.coinStates[coinID] = state
	}
	pm.mu.Unlock()

	// If no base price was configured and we haven't set one yet, use current price
	if state.BasePrice == 0 {
		state.BasePrice = currentPrice
		log.Printf("[%s] Base price set to: $%.4f", coinID, state.BasePrice)
		return
	}

	percentageChange := ((currentPrice - state.BasePrice) / state.BasePrice) * 100
	if pm.config.Debug {
		log.Printf("[%s] Price: $%.4f. Base change: %.2f%% (1h: %.2f%%, 24h: %.2f%%)", coinID, currentPrice, percentageChange, change1h, change24h)
	}

	if !state.LastAlertTime.IsZero() && time.Since(state.LastAlertTime) < time.Minute {
		return
	}

	if percentageChange <= -pm.config.DropThreshold {
		alertMsg := fmt.Sprintf("[%s] Price dropped by %.2f%% (From $%.4f to $%.4f)\n1h Change: %.2f%%\n24h Change: %.2f%%",
			coinID, -percentageChange, state.BasePrice, currentPrice, change1h, change24h)
		log.Printf("🔻 ALERT: %s", alertMsg)

		if err := pm.notifier.SendNotification(
			fmt.Sprintf("Price Drop Alert 🔻 [%s]", coinID),
			alertMsg,
		); err != nil {
			log.Printf("Error sending notification: %v", err)
		}

		state.LastAlertTime = time.Now()
		state.BasePrice = currentPrice
	} else if percentageChange >= pm.config.RiseThreshold {
		alertMsg := fmt.Sprintf("[%s] Price increased by %.2f%% (From $%.4f to $%.4f)\n1h Change: %.2f%%\n24h Change: %.2f%%",
			coinID, percentageChange, state.BasePrice, currentPrice, change1h, change24h)
		log.Printf("🔺 ALERT: %s", alertMsg)

		if err := pm.notifier.SendNotification(
			fmt.Sprintf("Price Increase Alert 🔺 [%s]", coinID),
			alertMsg,
		); err != nil {
			log.Printf("Error sending notification: %v", err)
		}

		state.LastAlertTime = time.Now()
		state.BasePrice = currentPrice
	}
}

func (pm *PriceMonitor) Start() error {
	var services []notifier.Service
	for _, token := range pm.config.PushTokens {
		services = append(services, notifier.NewPushbulletNotifier(token))
	}
	multiNotifier := notifier.NewMultiNotifier(services)
	pm.notifier = multiNotifier

	// Create channels for shutdown coordination
	interrupt := make(chan os.Signal, 1)
	shutdownComplete := make(chan struct{})
	signal.Notify(interrupt, os.Interrupt)

	log.Printf("Starting price monitor for coins: %v", pm.config.CoinIDs)
	log.Printf("Number of push notification tokens: %d", len(pm.config.PushTokens))
	log.Printf("Monitoring for:")
	log.Printf("- %.1f%% price drops", pm.config.DropThreshold)
	log.Printf("- %.1f%% price increases", pm.config.RiseThreshold)

	// Start monitoring in a separate goroutine
	go func() {
		defer close(shutdownComplete)
		for {
			select {
			case <-pm.done:
				log.Printf("Received shutdown signal, stopping monitoring...")
				return
			default:
				// Create context that gets cancelled when pm.done is closed
				ctx, cancel := context.WithCancel(context.Background())
				go func() {
					<-pm.done
					cancel()
				}()

				if err := pm.runMonitoringLoop(ctx); err != nil {
					log.Printf("Monitoring loop error: %v", err)
					time.Sleep(time.Second * 5)
				}
				cancel()
			}
		}
	}()

	// Wait for interrupt signal
	<-interrupt
	log.Println("\nReceived interrupt signal, shutting down...")

	// Stop signal notifications to prevent further signals
	signal.Stop(interrupt)

	// Trigger shutdown
	close(pm.done)

	// Wait for shutdown to complete with timeout
	select {
	case <-shutdownComplete:
		log.Println("Shutdown completed successfully")
		return nil
	case <-time.After(10 * time.Second):
		log.Println("Shutdown timed out after 10 seconds")
		return nil // Still return nil for graceful exit even on timeout
	}
}

func (pm *PriceMonitor) runMonitoringLoop(ctx context.Context) error {
	cmcClient := client.NewCoinMarketCapClient(pm.config)
	defer cmcClient.Close()

	if err := cmcClient.Connect(); err != nil {
		return fmt.Errorf("failed to connect to CoinMarketCap: %v", err)
	}

	for {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			log.Printf("Context cancelled, stopping monitoring loop...")
			return nil
		default:
		}

		priceData, err := cmcClient.ReadPriceData(ctx)
		if err != nil {
			return fmt.Errorf("error reading price data: %v", err)
		}

		// Process price data if it's relevant
		if priceData != nil {
			pm.CheckPriceChange(priceData.CoinID, priceData.Price, priceData.Change1h, priceData.Change24h)
		}
	}
}
