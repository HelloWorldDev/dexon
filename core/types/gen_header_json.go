// Code generated by github.com/fjl/gencodec. DO NOT EDIT.

package types

import (
	"encoding/json"
	"errors"
	"math/big"

	"github.com/dexon-foundation/dexon-consensus-core/core/types"

	"github.com/dexon-foundation/dexon/common"
	"github.com/dexon-foundation/dexon/common/hexutil"
)

var _ = (*headerMarshaling)(nil)

// MarshalJSON marshals as JSON.
func (h Header) MarshalJSON() ([]byte, error) {
	type Header struct {
		ParentHash         common.Hash    `json:"parentHash"         gencodec:"required"`
		UncleHash          common.Hash    `json:"sha3Uncles"         gencodec:"required"`
		Coinbase           common.Address `json:"miner"              gencodec:"required"`
		Root               common.Hash    `json:"stateRoot"          gencodec:"required"`
		TxHash             common.Hash    `json:"transactionsRoot"   gencodec:"required"`
		ReceiptHash        common.Hash    `json:"receiptsRoot"       gencodec:"required"`
		Bloom              Bloom          `json:"logsBloom"          gencodec:"required"`
		Difficulty         *hexutil.Big   `json:"difficulty"         gencodec:"required"`
		Number             *hexutil.Big   `json:"number"             gencodec:"required"`
		GasLimit           hexutil.Uint64 `json:"gasLimit"           gencodec:"required"`
		GasUsed            hexutil.Uint64 `json:"gasUsed"            gencodec:"required"`
		Time               *hexutil.Big   `json:"timestamp"          gencodec:"required"`
		Extra              hexutil.Bytes  `json:"extraData"          gencodec:"required"`
		MixDigest          common.Hash    `json:"mixHash"            gencodec:"required"`
		Nonce              BlockNonce     `json:"nonce"              gencodec:"required"`
		Randomness         hexutil.Bytes  `json:"randomness"         gencodec:"required"`
		Position           types.Position `json:"position"           gencodec:"required"`
		WitnessHeight      uint64         `json:"witnessHeight"      gencodec:"required"`
		WitnessRoot        common.Hash    `json:"witnessRoot"        gencodec:"required"`
		WitnessReceiptHash common.Hash    `json:"witnessReceiptHash" gencodec:"required"`
		DexconMeta         hexutil.Bytes  `json:"dexconMeta"         gencodec:"required"`
		Hash               common.Hash    `json:"hash"`
	}
	var enc Header
	enc.ParentHash = h.ParentHash
	enc.UncleHash = h.UncleHash
	enc.Coinbase = h.Coinbase
	enc.Root = h.Root
	enc.TxHash = h.TxHash
	enc.ReceiptHash = h.ReceiptHash
	enc.Bloom = h.Bloom
	enc.Difficulty = (*hexutil.Big)(h.Difficulty)
	enc.Number = (*hexutil.Big)(h.Number)
	enc.GasLimit = hexutil.Uint64(h.GasLimit)
	enc.GasUsed = hexutil.Uint64(h.GasUsed)
	enc.Time = (*hexutil.Big)(h.Time)
	enc.Extra = h.Extra
	enc.MixDigest = h.MixDigest
	enc.Nonce = h.Nonce
	enc.Randomness = h.Randomness
	enc.Position = h.Position
	enc.WitnessHeight = h.WitnessHeight
	enc.WitnessRoot = h.WitnessRoot
	enc.WitnessReceiptHash = h.WitnessReceiptHash
	enc.DexconMeta = h.DexconMeta
	enc.Hash = h.Hash()
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (h *Header) UnmarshalJSON(input []byte) error {
	type Header struct {
		ParentHash         *common.Hash    `json:"parentHash"         gencodec:"required"`
		UncleHash          *common.Hash    `json:"sha3Uncles"         gencodec:"required"`
		Coinbase           *common.Address `json:"miner"              gencodec:"required"`
		Root               *common.Hash    `json:"stateRoot"          gencodec:"required"`
		TxHash             *common.Hash    `json:"transactionsRoot"   gencodec:"required"`
		ReceiptHash        *common.Hash    `json:"receiptsRoot"       gencodec:"required"`
		Bloom              *Bloom          `json:"logsBloom"          gencodec:"required"`
		Difficulty         *hexutil.Big    `json:"difficulty"         gencodec:"required"`
		Number             *hexutil.Big    `json:"number"             gencodec:"required"`
		GasLimit           *hexutil.Uint64 `json:"gasLimit"           gencodec:"required"`
		GasUsed            *hexutil.Uint64 `json:"gasUsed"            gencodec:"required"`
		Time               *hexutil.Big    `json:"timestamp"          gencodec:"required"`
		Extra              *hexutil.Bytes  `json:"extraData"          gencodec:"required"`
		MixDigest          *common.Hash    `json:"mixHash"            gencodec:"required"`
		Nonce              *BlockNonce     `json:"nonce"              gencodec:"required"`
		Randomness         *hexutil.Bytes  `json:"randomness"         gencodec:"required"`
		Position           *types.Position `json:"position"           gencodec:"required"`
		WitnessHeight      *uint64         `json:"witnessHeight"      gencodec:"required"`
		WitnessRoot        *common.Hash    `json:"witnessRoot"        gencodec:"required"`
		WitnessReceiptHash *common.Hash    `json:"witnessReceiptHash" gencodec:"required"`
		DexconMeta         *hexutil.Bytes  `json:"dexconMeta"         gencodec:"required"`
	}
	var dec Header
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.ParentHash == nil {
		return errors.New("missing required field 'parentHash' for Header")
	}
	h.ParentHash = *dec.ParentHash
	if dec.UncleHash == nil {
		return errors.New("missing required field 'sha3Uncles' for Header")
	}
	h.UncleHash = *dec.UncleHash
	if dec.Coinbase == nil {
		return errors.New("missing required field 'miner' for Header")
	}
	h.Coinbase = *dec.Coinbase
	if dec.Root == nil {
		return errors.New("missing required field 'stateRoot' for Header")
	}
	h.Root = *dec.Root
	if dec.TxHash == nil {
		return errors.New("missing required field 'transactionsRoot' for Header")
	}
	h.TxHash = *dec.TxHash
	if dec.ReceiptHash == nil {
		return errors.New("missing required field 'receiptsRoot' for Header")
	}
	h.ReceiptHash = *dec.ReceiptHash
	if dec.Bloom == nil {
		return errors.New("missing required field 'logsBloom' for Header")
	}
	h.Bloom = *dec.Bloom
	if dec.Difficulty == nil {
		return errors.New("missing required field 'difficulty' for Header")
	}
	h.Difficulty = (*big.Int)(dec.Difficulty)
	if dec.Number == nil {
		return errors.New("missing required field 'number' for Header")
	}
	h.Number = (*big.Int)(dec.Number)
	if dec.GasLimit == nil {
		return errors.New("missing required field 'gasLimit' for Header")
	}
	h.GasLimit = uint64(*dec.GasLimit)
	if dec.GasUsed == nil {
		return errors.New("missing required field 'gasUsed' for Header")
	}
	h.GasUsed = uint64(*dec.GasUsed)
	if dec.Time == nil {
		return errors.New("missing required field 'timestamp' for Header")
	}
	h.Time = (*big.Int)(dec.Time)
	if dec.Extra == nil {
		return errors.New("missing required field 'extraData' for Header")
	}
	h.Extra = *dec.Extra
	if dec.MixDigest == nil {
		return errors.New("missing required field 'mixHash' for Header")
	}
	h.MixDigest = *dec.MixDigest
	if dec.Nonce == nil {
		return errors.New("missing required field 'nonce' for Header")
	}
	h.Nonce = *dec.Nonce
	if dec.Randomness == nil {
		return errors.New("missing required field 'randomness' for Header")
	}
	h.Randomness = *dec.Randomness
	if dec.Position == nil {
		return errors.New("missing required field 'position' for Header")
	}
	h.Position = *dec.Position
	if dec.WitnessHeight == nil {
		return errors.New("missing required field 'witnessHeight' for Header")
	}
	h.WitnessHeight = *dec.WitnessHeight
	if dec.WitnessRoot == nil {
		return errors.New("missing required field 'witnessRoot' for Header")
	}
	h.WitnessRoot = *dec.WitnessRoot
	if dec.WitnessReceiptHash == nil {
		return errors.New("missing required field 'witnessReceiptHash' for Header")
	}
	h.WitnessReceiptHash = *dec.WitnessReceiptHash
	if dec.DexconMeta == nil {
		return errors.New("missing required field 'dexconMeta' for Header")
	}
	h.DexconMeta = *dec.DexconMeta
	return nil
}
