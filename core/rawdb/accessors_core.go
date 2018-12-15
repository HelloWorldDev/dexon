package rawdb

import (
	"bytes"

	coreCommon "github.com/dexon-foundation/dexon-consensus/common"
	coreTypes "github.com/dexon-foundation/dexon-consensus/core/types"

	"github.com/dexon-foundation/dexon/common"
	"github.com/dexon-foundation/dexon/log"
	"github.com/dexon-foundation/dexon/rlp"
)

func ReadCoreBlockRLP(db DatabaseReader, hash common.Hash) rlp.RawValue {
	data, _ := db.Get(coreBlockKey(hash))
	return data
}

func WriteCoreBlockRLP(db DatabaseWriter, hash common.Hash, rlp rlp.RawValue) {
	if err := db.Put(coreBlockKey(hash), rlp); err != nil {
		log.Crit("Failed to store core block", "err", err)
	}
}

func HasCoreBlock(db DatabaseReader, hash common.Hash) bool {
	if has, err := db.Has(coreBlockKey(hash)); !has || err != nil {
		return false
	}
	return true
}

func ReadCoreBlock(db DatabaseReader, hash common.Hash) *coreTypes.Block {
	data := ReadCoreBlockRLP(db, hash)
	if len(data) == 0 {
		return nil
	}

	block := new(coreTypes.Block)
	if err := rlp.Decode(bytes.NewReader(data), block); err != nil {
		log.Error("Invalid core block RLP", "hash", hash, "err", err)
		return nil
	}
	return block
}

func WriteCoreBlock(db DatabaseWriter, hash common.Hash, block *coreTypes.Block) {
	data, err := rlp.EncodeToBytes(block)
	if err != nil {
		log.Crit("Failed to RLP encode core block", "err", err)
	}
	WriteCoreBlockRLP(db, hash, data)
}

type coreCompactionChainTipInfo struct {
	Hash   coreCommon.Hash
	Height uint64
}

func ReadCoreCompactionChainTipInfo(db DatabaseReader) (coreCommon.Hash, uint64) {
	data, _ := db.Get(coreCompactionChainTipInfoKey)
	info := new(coreCompactionChainTipInfo)
	if err := rlp.Decode(bytes.NewReader(data), info); err != nil {
		log.Error("Invalid core compaction chaint tip info RLP", "err", err)
		return coreCommon.Hash{}, uint64(0)
	}
	return info.Hash, info.Height
}

func WriteCoreCompactionChainTipInfo(
	db DatabaseWriter, hash coreCommon.Hash, height uint64) {
	info := &coreCompactionChainTipInfo{
		Hash:   hash,
		Height: height,
	}
	rlp, err := rlp.EncodeToBytes(info)
	if err != nil {
		log.Crit("Failed to RLP encode core compaciton chain info", "err", err)
	}
	if err := db.Put(coreCompactionChainTipInfoKey, rlp); err != nil {
		log.Crit("Failed to store core block", "err", err)
	}
}
