package client

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gunasekar/cryptowatch/types"
)

type WebSocketClient struct {
	conn   *websocket.Conn
	config *types.Config
	done   chan struct{}
}

func NewWebSocketClient(config *types.Config) *WebSocketClient {
	return &WebSocketClient{
		config: config,
		done:   make(chan struct{}),
	}
}

func (ws *WebSocketClient) Connect() error {
	header := http.Header{}
	header.Add("Origin", "https://coinmarketcap.com")
	header.Add("Cache-Control", "no-cache")
	header.Add("Accept-Language", "en-US,en;q=0.9")
	header.Add("Pragma", "no-cache")
	header.Add("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	c, _, err := websocket.DefaultDialer.Dial(ws.config.WebsocketURL, header)
	if err != nil {
		return fmt.Errorf("websocket dial error: %v", err)
	}

	ws.conn = c
	
	// Set read deadline to detect connection issues
	ws.conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	return ws.subscribe()
}

func (ws *WebSocketClient) subscribe() error {
	coinList := ""
	for i, coinID := range ws.config.CoinIDs {
		if i > 0 {
			coinList += ","
		}
		coinList += coinID
	}

	subMsg := types.WSSubscribeCommand{
		Method: "RSUBSCRIPTION",
		Params: []string{
			"main-site@crypto_price_15s@{}@detail",
			coinList,
		},
	}

	if err := ws.conn.WriteJSON(subMsg); err != nil {
		ws.Close()
		return fmt.Errorf("subscription error: %v", err)
	}

	return nil
}

func (ws *WebSocketClient) Reconnect() error {
	maxRetries := 5
	retryDelay := time.Second

	for i := 0; i < maxRetries; i++ {
		log.Printf("Attempting to reconnect (attempt %d/%d)...", i+1, maxRetries)
		
		if err := ws.Connect(); err == nil {
			return nil
		} else {
			log.Printf("Reconnection attempt failed: %v", err)
		}
		
		time.Sleep(retryDelay)
		retryDelay *= 2 // Exponential backoff
	}
	
	return fmt.Errorf("failed to reconnect after %d attempts", maxRetries)
}

func (ws *WebSocketClient) ReadMessage() ([]byte, error) {
	// Reset read deadline for each message
	ws.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	
	_, message, err := ws.conn.ReadMessage()
	return message, err
}

func (ws *WebSocketClient) StartPingHandler() {
	// Create a ping ticker to keep the connection alive
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	go func() {
		for {
			select {
			case <-pingTicker.C:
				if err := ws.conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
					log.Printf("Ping error: %v", err)
					return
				}
			case <-ws.done:
				log.Printf("Received shutdown signal, stopping ping goroutine...")
				return
			}
		}
	}()
}

func (ws *WebSocketClient) Close() error {
	close(ws.done)
	if ws.conn != nil {
		return ws.conn.Close()
	}
	return nil
}

func (ws *WebSocketClient) IsCloseError(err error) bool {
	return websocket.IsCloseError(err, websocket.CloseNormalClosure)
}