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
	BasePrices    map[string]float64
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
	done       chan struct{}
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
		done:       make(chan struct{}),
	}
}

func (pm *PriceMonitor) checkPriceChange(coinID string, currentPrice float64, change1h, change24h float64) {
	pm.mu.Lock()
	state, exists := pm.coinStates[coinID]
	if !exists {
		state = &CoinState{
			basePrice: pm.config.BasePrices[coinID],
		}
		pm.coinStates[coinID] = state
	}
	pm.mu.Unlock()

	// If no base price was configured and we haven't set one yet, use current price
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

	// Set read deadline to detect connection issues
	c.SetReadDeadline(time.Now().Add(60 * time.Second))

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

func (pm *PriceMonitor) reconnectWebSocket() (*websocket.Conn, error) {
	maxRetries := 5
	retryDelay := time.Second

	for i := 0; i < maxRetries; i++ {
		log.Printf("Attempting to reconnect (attempt %d/%d)...", i+1, maxRetries)
		conn, err := setupWebSocket(pm.config)
		if err == nil {
			return conn, nil
		}
		log.Printf("Reconnection attempt failed: %v", err)
		time.Sleep(retryDelay)
		retryDelay *= 2 // Exponential backoff
	}
	return nil, fmt.Errorf("failed to reconnect after %d attempts", maxRetries)
}

func (pm *PriceMonitor) runWebSocketLoop() error {
	conn, err := setupWebSocket(pm.config)
	if err != nil {
		// Try to reconnect if initial connection fails
		log.Printf("Initial connection failed, attempting to reconnect...")
		conn, err = pm.reconnectWebSocket()
		if err != nil {
			return fmt.Errorf("failed to establish connection: %v", err)
		}
	}
	defer conn.Close()

	// Create a ping ticker to keep the connection alive
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	// Create a channel to signal when the ping goroutine should stop
	pingDone := make(chan struct{})
	defer close(pingDone)

	go func() {
		for {
			select {
			case <-pingTicker.C:
				if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
					log.Printf("Ping error: %v", err)
					return
				}
			case <-pingDone:
				return
			case <-pm.done:
				log.Printf("Received shutdown signal, stopping ping goroutine...")
				return
			}
		}
	}()

	for {
		// Check for shutdown signal
		select {
		case <-pm.done:
			log.Printf("Received shutdown signal, stopping connection read goroutine...")
			return nil
		default:
		}

		// Reset read deadline for each message
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		// Use a channel to handle websocket read with cancellation
		msgCh := make(chan []byte)
		errCh := make(chan error)

		go func() {
			_, message, err := conn.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}
			msgCh <- message
		}()

		// Wait for either a message, error, or shutdown signal
		select {
		case <-pm.done:
			log.Printf("Received shutdown signal, stopping connection read goroutine...")
			return nil
		case err := <-errCh:
			// Check if this is a normal closure
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				return nil
			}
			log.Printf("WebSocket read error: %v", err)

			// Attempt to reconnect
			log.Printf("Attempting to reconnect...")
			newConn, reconnectErr := pm.reconnectWebSocket()
			if reconnectErr != nil {
				return fmt.Errorf("reconnection failed: %v", reconnectErr)
			}

			// Close old connection and update to new one
			conn.Close()
			conn = newConn
			continue
		case message := <-msgCh:
			var rawMessage map[string]interface{}
			if err := json.Unmarshal(message, &rawMessage); err != nil {
				log.Printf("Parse error: %v", err)
				continue
			}

			// Process the message...
			if d, ok := rawMessage["d"].(map[string]interface{}); ok {
				if id, ok := d["id"].(float64); ok {
					coinID := fmt.Sprintf("%.0f", id)
					if contains(pm.config.CoinIDs, coinID) {
						price, _ := d["p"].(float64)
						change1h, _ := d["p1h"].(float64)
						change24h, _ := d["p24h"].(float64)

						if price > 0 {
							log.Printf("[%s] Price: $%.4f (1h: %.2f%%, 24h: %.2f%%)",
								coinID, price, change1h, change24h)
							pm.checkPriceChange(coinID, price, change1h, change24h)
						}
					}
				}
			}
		}
	}
}

func monitorPrices(config *Config) error {
	var services []notifier.Service
	for _, token := range config.PushTokens {
		services = append(services, notifier.NewPushbulletNotifier(token))
	}
	multiNotifier := notifier.NewMultiNotifier(services)

	monitor := newPriceMonitor(config, multiNotifier)

	// Create channels for shutdown coordination
	interrupt := make(chan os.Signal, 1)
	shutdownComplete := make(chan struct{})
	signal.Notify(interrupt, os.Interrupt)

	log.Printf("Starting price monitor for coins: %v", config.CoinIDs)
	log.Printf("Number of push notification tokens: %d", len(config.PushTokens))
	log.Printf("Monitoring for:")
	log.Printf("- %.1f%% price drops", config.DropThreshold)
	log.Printf("- %.1f%% price increases", config.RiseThreshold)

	// Start monitoring in a separate goroutine
	go func() {
		defer close(shutdownComplete)
		for {
			select {
			case <-monitor.done:
				log.Printf("Received shutdown signal, stopping monitoring...")
				return
			default:
				if err := monitor.runWebSocketLoop(); err != nil {
					log.Printf("WebSocket loop error: %v", err)
					time.Sleep(time.Second * 5)
				}
			}
		}
	}()

	// Wait for interrupt signal
	<-interrupt
	log.Println("\nReceived interrupt signal, shutting down...")

	// Trigger shutdown
	close(monitor.done)

	// Wait for shutdown to complete with timeout
	select {
	case <-shutdownComplete:
		log.Println("Shutdown completed successfully")
	case <-time.After(10 * time.Second):
		log.Println("Shutdown timed out after 10 seconds")
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
		basePrices    string
	)

	flag.StringVar(&coinIDs, "coins", "", "Comma-separated list of coin IDs to monitor (e.g., '12345,67890')")
	flag.StringVar(&pushTokens, "push-tokens", "", "Comma-separated list of Pushbullet API tokens")
	flag.Float64Var(&dropThreshold, "drop", 5.0, "Price drop percentage threshold for alerts")
	flag.Float64Var(&riseThreshold, "rise", 10.0, "Price rise percentage threshold for alerts")
	flag.StringVar(&basePrices, "base-prices", "", "Comma-separated list of base prices in format 'coinID:price' (e.g., '12345:50000,67890:1800')")
	flag.Parse()

	if coinIDs == "" {
		log.Fatal("Please provide coin IDs to monitor using the -coins flag")
	}

	var tokens []string
	if pushTokens != "" {
		tokens = strings.Split(pushTokens, ",")
	}

	// Parse base prices
	basePriceMap := make(map[string]float64)
	if basePrices != "" {
		pairs := strings.Split(basePrices, ",")
		for _, pair := range pairs {
			parts := strings.Split(pair, ":")
			if len(parts) != 2 {
				log.Fatalf("Invalid base price format. Expected 'coinID:price', got '%s'", pair)
			}
			price := 0.0
			if _, err := fmt.Sscanf(parts[1], "%f", &price); err != nil {
				log.Fatalf("Invalid price value for coin %s: %v", parts[0], err)
			}
			basePriceMap[parts[0]] = price
		}
	}

	config := &Config{
		CoinIDs:       strings.Split(coinIDs, ","),
		PushTokens:    tokens,
		WebsocketURL:  "wss://push.coinmarketcap.com/ws?device=web&client_source=coin_detail_page",
		DropThreshold: dropThreshold,
		RiseThreshold: riseThreshold,
		BasePrices:    basePriceMap,
	}

	if err := monitorPrices(config); err != nil {
		log.Fatal(err)
	}
}
