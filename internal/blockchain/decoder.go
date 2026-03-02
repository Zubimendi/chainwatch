package blockchain

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Known ERC-20 event signatures (keccak256 of event signature string)
var (
	// keccak256("Transfer(address,address,uint256)")
	transferEventSig = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)")).Hex()

	// keccak256("Approval(address,address,uint256)")
	approvalEventSig = crypto.Keccak256Hash([]byte("Approval(address,address,uint256)")).Hex()

	// Flash loan signatures (common protocols)
	// keccak256("FlashLoan(address,address,uint256,uint256)")
	aaveFlashLoanSig = crypto.Keccak256Hash([]byte("FlashLoan(address,address,uint256,uint256)")).Hex()
)

// Wad is 10^18 — used for ETH conversion
var wad = new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))

// Decoder processes raw blockchain data into structured, typed representations.
type Decoder struct {
	erc20ABI abi.ABI
}

// NewDecoder initializes the decoder with pre-loaded ABIs.
func NewDecoder(erc20ABI abi.ABI) *Decoder {
	return &Decoder{erc20ABI: erc20ABI}
}

// DecodeTransaction converts a RawTransaction to a DecodedTransaction.
// Hex values are converted, ETH values calculated, and calldata decoded where possible.
func (d *Decoder) DecodeTransaction(raw RawTransaction) (*DecodedTransaction, error) {
	tx := &DecodedTransaction{
		Hash:        raw.Hash,
		From:        strings.ToLower(raw.From),
		ReceivedAt:  time.Now().UTC(),
	}

	if raw.To != nil {
		tx.To = strings.ToLower(*raw.To)
	} else {
		tx.IsContractDeployment = true
	}

	// Decode value (hex wei → big.Int → ETH float)
	if raw.Value != "" && raw.Value != "0x0" {
		weiInt, err := hexToBigInt(raw.Value)
		if err != nil {
			return nil, fmt.Errorf("decoding value: %w", err)
		}
		tx.ValueWei = weiInt
		tx.ValueETH = weiToETH(weiInt)
	} else {
		tx.ValueWei = big.NewInt(0)
	}

	// Gas
	if raw.Gas != "" {
		gasInt, err := hexToBigInt(raw.Gas)
		if err == nil {
			tx.GasLimit = gasInt.Uint64()
		}
	}

	// Gas price (use maxFeePerGas for EIP-1559, fallback to gasPrice)
	gasPriceHex := raw.GasPrice
	if raw.MaxFeePerGas != nil && *raw.MaxFeePerGas != "" {
		gasPriceHex = *raw.MaxFeePerGas
	}
	if gasPriceHex != "" {
		gp, err := hexToBigInt(gasPriceHex)
		if err == nil {
			tx.GasPrice = gp
			tx.GasPriceGwei = weiToGwei(gp)
		}
	}

	// Nonce
	if raw.Nonce != "" {
		nonceInt, err := hexToBigInt(raw.Nonce)
		if err == nil {
			tx.Nonce = nonceInt.Uint64()
		}
	}

	// Block number
	if raw.BlockNumber != nil && *raw.BlockNumber != "" {
		bn, err := hexToBigInt(*raw.BlockNumber)
		if err == nil {
			tx.BlockNumber = bn.Uint64()
		}
	}

	// Decode input data
	if raw.Input != "" && raw.Input != "0x" {
		inputBytes, err := hex.DecodeString(strings.TrimPrefix(raw.Input, "0x"))
		if err == nil {
			tx.InputData = inputBytes
			// Try to decode as ERC-20 method call
			if len(inputBytes) >= 4 {
				method, args, err := d.decodeCalldata(inputBytes)
				if err == nil {
					tx.MethodName = method
					tx.MethodArgs = args
				}
			}
		}
	}

	return tx, nil
}

// DecodeLog converts a RawLog to a DecodedLog by matching topic[0] against known event signatures.
func (d *Decoder) DecodeLog(raw RawLog) (*DecodedLog, error) {
	if len(raw.Topics) == 0 {
		return nil, fmt.Errorf("log has no topics")
	}

	decoded := &DecodedLog{
		Address:         strings.ToLower(raw.Address),
		TransactionHash: raw.TransactionHash,
		ReceivedAt:      time.Now().UTC(),
	}

	if raw.BlockNumber != "" {
		bn, err := hexToBigInt(raw.BlockNumber)
		if err == nil {
			decoded.BlockNumber = bn.Uint64()
		}
	}

	if raw.LogIndex != "" {
		li, err := hexToBigInt(raw.LogIndex)
		if err == nil {
			decoded.LogIndex = uint(li.Uint64())
		}
	}

	topic0 := raw.Topics[0]
	decoded.EventSignature = topic0

	// Match against known signatures
	switch topic0 {
	case transferEventSig:
		return d.decodeTransferEvent(decoded, raw)
	case approvalEventSig:
		decoded.EventName = "Approval"
		return decoded, nil
	case aaveFlashLoanSig:
		decoded.EventName = "FlashLoan"
		return decoded, nil
	default:
		decoded.EventName = "Unknown"
		return decoded, nil
	}
}

// decodeTransferEvent decodes ERC-20 Transfer(address indexed from, address indexed to, uint256 value)
func (d *Decoder) decodeTransferEvent(decoded *DecodedLog, raw RawLog) (*DecodedLog, error) {
	decoded.EventName = "Transfer"

	args := make(map[string]interface{})

	// Topics: [0] = sig, [1] = from (indexed), [2] = to (indexed)
	if len(raw.Topics) < 3 {
		return decoded, nil
	}

	from := common.HexToAddress(raw.Topics[1])
	to := common.HexToAddress(raw.Topics[2])
	args["from"] = strings.ToLower(from.Hex())
	args["to"] = strings.ToLower(to.Hex())

	// Data contains the non-indexed uint256 value
	if raw.Data != "" && raw.Data != "0x" {
		dataBytes, err := hex.DecodeString(strings.TrimPrefix(raw.Data, "0x"))
		if err == nil && len(dataBytes) == 32 {
			amount := new(big.Int).SetBytes(dataBytes)
			args["value"] = amount.String()
		}
	}

	decoded.Args = args
	return decoded, nil
}

// decodeCalldata attempts to match the 4-byte selector against known ERC-20 methods.
func (d *Decoder) decodeCalldata(input []byte) (string, map[string]interface{}, error) {
	if len(input) < 4 {
		return "", nil, fmt.Errorf("input too short")
	}

	selector := input[:4]
	method, err := d.erc20ABI.MethodById(selector)
	if err != nil {
		return "", nil, fmt.Errorf("unknown method selector: %x", selector)
	}

	if len(input) <= 4 {
		return method.Name, nil, nil
	}

	args := make(map[string]interface{})
	if err := method.Inputs.UnpackIntoMap(args, input[4:]); err != nil {
		return method.Name, nil, fmt.Errorf("unpack args: %w", err)
	}

	return method.Name, args, nil
}

// ─── Hex conversion helpers ───────────────────────────────────────────────────

func hexToBigInt(h string) (*big.Int, error) {
	h = strings.TrimPrefix(h, "0x")
	if h == "" {
		return big.NewInt(0), nil
	}
	n := new(big.Int)
	if _, ok := n.SetString(h, 16); !ok {
		return nil, fmt.Errorf("invalid hex: %s", h)
	}
	return n, nil
}

func weiToETH(wei *big.Int) float64 {
	if wei == nil {
		return 0
	}
	f := new(big.Float).SetInt(wei)
	result, _ := new(big.Float).Quo(f, wad).Float64()
	return result
}

func weiToGwei(wei *big.Int) float64 {
	if wei == nil {
		return 0
	}
	gwei := new(big.Float).SetInt(wei)
	divisor := new(big.Float).SetInt(big.NewInt(1_000_000_000))
	result, _ := new(big.Float).Quo(gwei, divisor).Float64()
	return result
}