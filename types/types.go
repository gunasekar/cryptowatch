package types

import (
	"time"
)

type Config struct {
	CoinIDs       []string
	PushTokens    []string
	WebsocketURL  string
	DropThreshold float64
	RiseThreshold float64
	BasePrices    map[string]float64
	Debug         bool
}

type CoinState struct {
	BasePrice     float64
	LastAlertTime time.Time
}

type WSSubscribeCommand struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
}