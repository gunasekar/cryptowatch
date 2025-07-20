package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/gunasekar/cryptowatch/types"
)

type CoinMarketCapClient struct {
	ws     *WebSocketClient
	config *types.Config
}

type PriceData struct {
	CoinID   string
	Price    float64
	Change1h float64
	Change24h float64
}

func NewCoinMarketCapClient(config *types.Config) *CoinMarketCapClient {
	return &CoinMarketCapClient{
		ws:     NewWebSocketClient(config),
		config: config,
	}
}

func (cmc *CoinMarketCapClient) Connect() error {
	if err := cmc.ws.Connect(); err != nil {
		// Try to reconnect if initial connection fails
		log.Printf("Initial connection failed, attempting to reconnect...")
		if reconnectErr := cmc.ws.Reconnect(); reconnectErr != nil {
			return fmt.Errorf("failed to establish connection: %v", reconnectErr)
		}
	}

	// Start ping handler to keep connection alive
	cmc.ws.StartPingHandler()
	
	return nil
}

func (cmc *CoinMarketCapClient) ReadPriceData(ctx context.Context) (*PriceData, error) {
	// Check for context cancellation before proceeding
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	message, err := cmc.ws.ReadMessage()
	if err != nil {
		// Check if this is a normal closure
		if cmc.ws.IsCloseError(err) {
			return nil, err
		}
		
		log.Printf("WebSocket read error: %v", err)
		
		// Check for context cancellation before reconnecting
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		
		// Attempt to reconnect
		log.Printf("Attempting to reconnect...")
		if reconnectErr := cmc.ws.Reconnect(); reconnectErr != nil {
			return nil, fmt.Errorf("reconnection failed: %v", reconnectErr)
		}
		
		// Restart ping handler after reconnection
		cmc.ws.StartPingHandler()
		
		// Try reading again after reconnection with context
		return cmc.ReadPriceData(ctx)
	}

	return cmc.parseMessage(message)
}

func (cmc *CoinMarketCapClient) parseMessage(message []byte) (*PriceData, error) {
	var rawMessage map[string]interface{}
	if err := json.Unmarshal(message, &rawMessage); err != nil {
		return nil, fmt.Errorf("parse error: %v", err)
	}

	// Process the message...
	if d, ok := rawMessage["d"].(map[string]interface{}); ok {
		if id, ok := d["id"].(float64); ok {
			coinID := fmt.Sprintf("%.0f", id)
			
			// Check if this coin is being monitored
			if cmc.contains(cmc.config.CoinIDs, coinID) {
				price, _ := d["p"].(float64)
				change1h, _ := d["p1h"].(float64)
				change24h, _ := d["p24h"].(float64)

				if price > 0 {
					return &PriceData{
						CoinID:    coinID,
						Price:     price,
						Change1h:  change1h,
						Change24h: change24h,
					}, nil
				}
			}
		}
	}

	return nil, nil // Not a relevant price update
}

func (cmc *CoinMarketCapClient) contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

func (cmc *CoinMarketCapClient) Close() error {
	return cmc.ws.Close()
}