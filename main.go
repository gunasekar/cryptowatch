package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gunasekar/cryptowatch/notifier"
)

type Config struct {
	CoinIDs       []string
	PushTokens    []string
	WebsocketURL  string
	DropThreshold float64
	RiseThreshold float64
}

type CoinState struct {
	basePrice     float64
	lastAlertTime time.Time
}

type PriceMonitor struct {
	coinStates map[string]*CoinState
	config     *Config
	notifier   notifier.Service
	mu         sync.RWMutex
}

type WSSubscribeCommand struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
}

func newPriceMonitor(config *Config, notifier notifier.Service) *PriceMonitor {
	return &PriceMonitor{
		config:     config,
		notifier:   notifier,
		coinStates: make(map[string]*CoinState),
	}
}

func (pm *PriceMonitor) checkPriceChange(coinID string, currentPrice float64, change1h, change24h float64) {
	pm.mu.Lock()
	state, exists := pm.coinStates[coinID]
	if !exists {
		state = &CoinState{}
		pm.coinStates[coinID] = state
	}
	pm.mu.Unlock()

	if state.basePrice == 0 {
		state.basePrice = currentPrice
		log.Printf("[%s] Base price set to: $%.4f", coinID, state.basePrice)
		return
	}

	percentageChange := ((currentPrice - state.basePrice) / state.basePrice) * 100

	if !state.lastAlertTime.IsZero() && time.Since(state.lastAlertTime) < time.Minute {
		return
	}

	if percentageChange <= -pm.config.DropThreshold {
		alertMsg := fmt.Sprintf("[%s] Price dropped by %.2f%% (From $%.4f to $%.4f)\n1h Change: %.2f%%\n24h Change: %.2f%%",
			coinID, -percentageChange, state.basePrice, currentPrice, change1h, change24h)
		log.Printf("🔻 ALERT: %s", alertMsg)

		if err := pm.notifier.SendNotification(
			fmt.Sprintf("Price Drop Alert 🔻 [%s]", coinID),
			alertMsg,
		); err != nil {
			log.Printf("Error sending notification: %v", err)
		}

		state.lastAlertTime = time.Now()
		state.basePrice = currentPrice
	} else if percentageChange >= pm.config.RiseThreshold {
		alertMsg := fmt.Sprintf("[%s] Price increased by %.2f%% (From $%.4f to $%.4f)\n1h Change: %.2f%%\n24h Change: %.2f%%",
			coinID, percentageChange, state.basePrice, currentPrice, change1h, change24h)
		log.Printf("🔺 ALERT: %s", alertMsg)

		if err := pm.notifier.SendNotification(
			fmt.Sprintf("Price Increase Alert 🔺 [%s]", coinID),
			alertMsg,
		); err != nil {
			log.Printf("Error sending notification: %v", err)
		}

		state.lastAlertTime = time.Now()
		state.basePrice = currentPrice
	}
}

func setupWebSocket(config *Config) (*websocket.Conn, error) {
	header := http.Header{}
	header.Add("Origin", "https://coinmarketcap.com")
	header.Add("Cache-Control", "no-cache")
	header.Add("Accept-Language", "en-US,en;q=0.9")
	header.Add("Pragma", "no-cache")
	header.Add("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	c, _, err := websocket.DefaultDialer.Dial(config.WebsocketURL, header)
	if err != nil {
		return nil, fmt.Errorf("websocket dial error: %v", err)
	}

	coinList := strings.Join(config.CoinIDs, ",")
	subMsg := WSSubscribeCommand{
		Method: "RSUBSCRIPTION",
		Params: []string{
			"main-site@crypto_price_15s@{}@detail",
			coinList,
		},
	}

	if err := c.WriteJSON(subMsg); err != nil {
		c.Close()
		return nil, fmt.Errorf("subscription error: %v", err)
	}

	return c, nil
}

func monitorPrices(config *Config) error {
	var services []notifier.Service
	for _, token := range config.PushTokens {
		services = append(services, notifier.NewPushbulletNotifier(token))
	}
	multiNotifier := notifier.NewMultiNotifier(services)

	monitor := newPriceMonitor(config, multiNotifier)

	conn, err := setupWebSocket(config)
	if err != nil {
		return err
	}
	defer conn.Close()

	log.Printf("Starting price monitor for coins: %v", config.CoinIDs)
	log.Printf("Number of push notification tokens: %d", len(config.PushTokens))
	log.Printf("Monitoring for:")
	log.Printf("- %.1f%% price drops", config.DropThreshold)
	log.Printf("- %.1f%% price increases", config.RiseThreshold)

	done := make(chan struct{})
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	go func() {
		defer close(done)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("Read error: %v", err)
				return
			}

			var rawMessage map[string]interface{}
			if err := json.Unmarshal(message, &rawMessage); err != nil {
				log.Printf("Parse error: %v", err)
				continue
			}

			if d, ok := rawMessage["d"].(map[string]interface{}); ok {
				if id, ok := d["id"].(float64); ok {
					coinID := fmt.Sprintf("%.0f", id)
					if contains(config.CoinIDs, coinID) {
						price, _ := d["p"].(float64)
						change1h, _ := d["p1h"].(float64)
						change24h, _ := d["p24h"].(float64)

						if price > 0 {
							log.Printf("[%s] Price: $%.4f (1h: %.2f%%, 24h: %.2f%%)",
								coinID, price, change1h, change24h)
							monitor.checkPriceChange(coinID, price, change1h, change24h)
						}
					}
				}
			}
		}
	}()

	<-interrupt
	log.Println("\nReceived interrupt signal, closing connection...")

	err = conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)
	if err != nil {
		return fmt.Errorf("write close error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
	}

	return nil
}

func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

func main() {
	var (
		coinIDs       string
		pushTokens    string
		dropThreshold float64
		riseThreshold float64
	)

	flag.StringVar(&coinIDs, "coins", "", "Comma-separated list of coin IDs to monitor (e.g., '34880,34926')")
	flag.StringVar(&pushTokens, "push-tokens", "", "Comma-separated list of Pushbullet API tokens")
	flag.Float64Var(&dropThreshold, "drop", 5.0, "Price drop percentage threshold for alerts")
	flag.Float64Var(&riseThreshold, "rise", 10.0, "Price rise percentage threshold for alerts")
	flag.Parse()

	if coinIDs == "" {
		log.Fatal("Please provide coin IDs to monitor using the -coins flag")
	}

	var tokens []string
	if pushTokens != "" {
		tokens = strings.Split(pushTokens, ",")
	}

	config := &Config{
		CoinIDs:       strings.Split(coinIDs, ","),
		PushTokens:    tokens,
		WebsocketURL:  "wss://push.coinmarketcap.com/ws?device=web&client_source=coin_detail_page",
		DropThreshold: dropThreshold,
		RiseThreshold: riseThreshold,
	}

	if err := monitorPrices(config); err != nil {
		log.Fatal(err)
	}
}
