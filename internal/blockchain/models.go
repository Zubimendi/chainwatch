// internal/blockchain/models.go
package blockchain

import (
	"math/big"
	"time"
)

// RawTransaction is what comes directly off the WebSocket subscription.
// Field names match Ethereum JSON-RPC response fields exactly.
type RawTransaction struct {
	Hash             string  `json:"hash"`
	From             string  `json:"from"`
	To               *string `json:"to"` // nil for contract deployments
	Value            string  `json:"value"`    // hex-encoded wei
	Gas              string  `json:"gas"`      // hex-encoded
	GasPrice         string  `json:"gasPrice"` // hex-encoded
	MaxFeePerGas     *string `json:"maxFeePerGas"`
	MaxPriorityFee   *string `json:"maxPriorityFeePerGas"`
	Input            string  `json:"input"`    // hex-encoded calldata
	Nonce            string  `json:"nonce"`    // hex-encoded
	BlockNumber      *string `json:"blockNumber"`
	TransactionIndex *string `json:"transactionIndex"`
	ChainID          string  `json:"chainId"`
}

// DecodedTransaction is what the decoder produces after processing a RawTransaction.
// All hex values are converted to their native Go types here.
type DecodedTransaction struct {
	Hash        string
	From        string
	To          string   // empty string for contract deployments
	ValueWei    *big.Int // original value in wei
	ValueETH    float64  // converted to ETH for human readability
	GasLimit    uint64
	GasPrice    *big.Int
	GasPriceGwei float64
	InputData   []byte
	Nonce       uint64
	BlockNumber uint64
	IsContractDeployment bool
	// Decoded method call — populated if the input matches a known ABI
	MethodName string
	MethodArgs map[string]interface{}
	// ERC-20 specifics (populated if this looks like a token transfer)
	TokenTransfer *TokenTransfer
	ReceivedAt    time.Time
}

// TokenTransfer holds decoded ERC-20 Transfer event data.
type TokenTransfer struct {
	ContractAddress string
	From            string
	To              string
	Amount          *big.Int
	TokenSymbol     string // if known
	TokenDecimals   uint8  // if known
}

// RawLog is an Ethereum event log from the WebSocket subscription.
type RawLog struct {
	Address          string   `json:"address"`
	Topics           []string `json:"topics"`
	Data             string   `json:"data"`
	BlockNumber      string   `json:"blockNumber"`
	TransactionHash  string   `json:"transactionHash"`
	TransactionIndex string   `json:"transactionIndex"`
	LogIndex         string   `json:"logIndex"`
	Removed          bool     `json:"removed"`
}

// DecodedLog is a processed event log with decoded topic and data fields.
type DecodedLog struct {
	Address         string
	EventName       string
	EventSignature  string
	Args            map[string]interface{}
	BlockNumber     uint64
	TransactionHash string
	LogIndex        uint
	ReceivedAt      time.Time
}

// WSMessage is the envelope for every message received over the WebSocket.
type WSMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  WSMessageParams `json:"params"`
}

type WSMessageParams struct {
	Subscription string      `json:"subscription"`
	Result       interface{} `json:"result"`
}

// SubscriptionRequest is sent to the node to subscribe to new transactions or logs.
type SubscriptionRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}