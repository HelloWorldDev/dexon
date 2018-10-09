// Copyright 2018 The dexon-consensus-core Authors
// This file is part of the dexon-consensus-core library.
//
// The dexon-consensus-core library is free software: you can redistribute it
// and/or modify it under the terms of the GNU Lesser General Public License as
// published by the Free Software Foundation, either version 3 of the License,
// or (at your option) any later version.
//
// The dexon-consensus-core library is distributed in the hope that it will be
// useful, but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU Lesser
// General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the dexon-consensus-core library. If not, see
// <http://www.gnu.org/licenses/>.

package dex

import (
	"bytes"
	"fmt"
	"math/big"
	"sync"
	"time"

	coreCommon "github.com/dexon-foundation/dexon-consensus-core/common"
	coreTypes "github.com/dexon-foundation/dexon-consensus-core/core/types"

	"github.com/dexon-foundation/dexon/common"
	"github.com/dexon-foundation/dexon/core"
	"github.com/dexon-foundation/dexon/core/state"
	"github.com/dexon-foundation/dexon/core/types"
	"github.com/dexon-foundation/dexon/core/vm"
	"github.com/dexon-foundation/dexon/ethdb"
	"github.com/dexon-foundation/dexon/log"
	"github.com/dexon-foundation/dexon/rlp"
)

// DexconApp implementes the DEXON consensus core application interface.
type DexconApp struct {
	txPool     *core.TxPool
	blockchain *core.BlockChain
	gov        *DexconGovernance
	chainDB    ethdb.Database
	config     *Config
	vmConfig   vm.Config

	notifyChan map[uint64]*notify
	mutex      *sync.Mutex

	lastHeight uint64
	insertMu   sync.Mutex
}

type notify struct {
	results []chan uint64
}

type witnessData struct {
	Root        common.Hash
	TxHash      common.Hash
	ReceiptHash common.Hash
}

func NewDexconApp(txPool *core.TxPool, blockchain *core.BlockChain, gov *DexconGovernance, chainDB ethdb.Database, config *Config, vmConfig vm.Config) *DexconApp {
	return &DexconApp{
		txPool:     txPool,
		blockchain: blockchain,
		gov:        gov,
		chainDB:    chainDB,
		config:     config,
		vmConfig:   vmConfig,
		notifyChan: make(map[uint64]*notify),
		mutex:      &sync.Mutex{},
	}
}

func (d *DexconApp) addNotify(height uint64) <-chan uint64 {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	result := make(chan uint64)
	if n, exist := d.notifyChan[height]; exist {
		n.results = append(n.results, result)
	} else {
		d.notifyChan[height] = &notify{}
		d.notifyChan[height].results = append(d.notifyChan[height].results, result)
	}
	return result
}

func (d *DexconApp) notify(height uint64) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	for h, n := range d.notifyChan {
		if height >= h {
			for _, ch := range n.results {
				ch <- height
			}
			delete(d.notifyChan, h)
		}
	}
	d.lastHeight = height
}

func (d *DexconApp) checkChain(address common.Address, chainSize, chainID *big.Int) bool {
	addrModChainSize := new(big.Int)
	return addrModChainSize.Mod(address.Big(), chainSize).Cmp(chainID) == 0
}

// PreparePayload is called when consensus core is preparing payload for block.
func (d *DexconApp) PreparePayload(position coreTypes.Position) (payload []byte, err error) {
	d.insertMu.Lock()
	defer d.insertMu.Unlock()
	txsMap, err := d.txPool.Pending()
	if err != nil {
		return
	}

	chainID := new(big.Int).SetUint64(uint64(position.ChainID))
	chainSize := new(big.Int).SetUint64(uint64(d.gov.GetNumChains(position.Round)))
	var allTxs types.Transactions
	for addr, txs := range txsMap {
		// every address's transactions will appear in fixed chain
		if !d.checkChain(addr, chainSize, chainID) {
			continue
		}

		var stateDB *state.StateDB
		if d.lastHeight > 0 {
			stateDB, err = d.blockchain.StateAt(d.blockchain.GetPendingBlockByHeight(d.lastHeight).Root())
			if err != nil {
				return nil, fmt.Errorf("PreparePayload d.blockchain.StateAt err %v", err)
			}
		} else {
			stateDB, err = d.blockchain.State()
			if err != nil {
				return nil, fmt.Errorf("PreparePayload d.blockchain.State err %v", err)
			}
		}

		for _, tx := range txs {
			if tx.Nonce() != stateDB.GetNonce(addr) {
				log.Debug("break transaction", "tx.hash", tx.Hash(), "nonce", tx.Nonce(), "expect", stateDB.GetNonce(addr))
				break
			}
			log.Debug("receive transaction", "tx.hash", tx.Hash(), "nonce", tx.Nonce(), "amount", tx.Value())
			allTxs = append(allTxs, tx)
		}
	}
	payload, err = rlp.EncodeToBytes(&allTxs)
	if err != nil {
		return
	}

	return
}

// PrepareWitness will return the witness data no lower than consensusHeight.
func (d *DexconApp) PrepareWitness(consensusHeight uint64) (witness coreTypes.Witness, err error) {
	// TODO(bojie): the witness logic need to correct
	var witnessBlock *types.Block
	if d.lastHeight == 0 && consensusHeight == 0 {
		witnessBlock = d.blockchain.CurrentBlock()
	} else if d.lastHeight >= consensusHeight {
		witnessBlock = d.blockchain.GetPendingBlockByHeight(d.lastHeight)
	} else if h := <-d.addNotify(consensusHeight); h >= consensusHeight {
		witnessBlock = d.blockchain.GetPendingBlockByHeight(h)
	} else {
		log.Error("need pending block")
		return witness, fmt.Errorf("need pending block")
	}

	witnessData, err := rlp.EncodeToBytes(&witnessData{
		Root:        witnessBlock.Root(),
		TxHash:      witnessBlock.TxHash(),
		ReceiptHash: witnessBlock.ReceiptHash(),
	})
	if err != nil {
		return
	}

	return coreTypes.Witness{
		Timestamp: time.Unix(witnessBlock.Time().Int64(), 0),
		Height:    witnessBlock.NumberU64(),
		Data:      witnessData,
	}, nil
}

// VerifyBlock verifies if the payloads are valid.
func (d *DexconApp) VerifyBlock(block *coreTypes.Block) bool {
	// TODO(bojie): implement this
	return true
}

// BlockDelivered is called when a block is add to the compaction chain.
func (d *DexconApp) BlockDelivered(blockHash coreCommon.Hash, result coreTypes.FinalizationResult) {
	d.insertMu.Lock()
	defer d.insertMu.Unlock()

	block := d.blockchain.GetConfirmedBlockByHash(blockHash)
	if block == nil {
		log.Error("can not get confirmed block")
		return
	}

	var transactions types.Transactions
	err := rlp.Decode(bytes.NewReader(block.Payload), &transactions)
	if err != nil {
		log.Error("payload rlp decode", "error", err)
		return
	}

	var witnessData witnessData
	err = rlp.Decode(bytes.NewReader(block.Witness.Data), &witnessData)
	if err != nil {
		log.Error("witness rlp decode", "error", err)
		return
	}

	log.Debug("block proposer id", "hash", block.ProposerID)
	newBlock := types.NewBlock(&types.Header{
		Number:             new(big.Int).SetUint64(result.Height),
		Time:               big.NewInt(result.Timestamp.Unix()),
		Coinbase:           common.BytesToAddress(block.ProposerID.Bytes()),
		WitnessHeight:      block.Witness.Height,
		WitnessRoot:        witnessData.Root,
		WitnessReceiptHash: witnessData.ReceiptHash,
		// TODO(bojie): fix it
		GasLimit:   8000000,
		Difficulty: big.NewInt(1),
	}, transactions, nil, nil)

	_, err = d.blockchain.InsertPendingBlock([]*types.Block{newBlock})
	if err != nil {
		log.Error("insert chain", "error", err)
		return
	}

	log.Debug("insert pending block success", "height", result.Height)

	d.blockchain.RemoveConfirmedBlock(blockHash)
	d.notify(result.Height)
}

// BlockConfirmed is called when a block is confirmed and added to lattice.
func (d *DexconApp) BlockConfirmed(block coreTypes.Block) {
	d.blockchain.AddConfirmedBlock(&block)
}
