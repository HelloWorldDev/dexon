// Copyright 2018 The dexon-consensus Authors
// This file is part of the dexon-consensus library.
//
// The dexon-consensus library is free software: you can redistribute it
// and/or modify it under the terms of the GNU Lesser General Public License as
// published by the Free Software Foundation, either version 3 of the License,
// or (at your option) any later version.
//
// The dexon-consensus library is distributed in the hope that it will be
// useful, but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU Lesser
// General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the dexon-consensus library. If not, see
// <http://www.gnu.org/licenses/>.

package core

import (
	"fmt"
	"sync"
	"time"

	"github.com/dexon-foundation/dexon-consensus/common"
	"github.com/dexon-foundation/dexon-consensus/core/db"
	"github.com/dexon-foundation/dexon-consensus/core/types"
	"github.com/dexon-foundation/dexon-consensus/core/utils"
)

// Errors for sanity check error.
var (
	ErrRetrySanityCheckLater = fmt.Errorf("retry sanity check later")
)

// Lattice represents a unit to produce a global ordering from multiple chains.
type Lattice struct {
	lock     sync.RWMutex
	signer   *utils.Signer
	app      Application
	debug    Debug
	pool     blockPool
	data     *latticeData
	toModule *totalOrdering
	ctModule *consensusTimestamp
	logger   common.Logger
}

// NewLattice constructs an Lattice instance.
func NewLattice(
	dMoment time.Time,
	round uint64,
	cfg *types.Config,
	signer *utils.Signer,
	app Application,
	debug Debug,
	db db.Database,
	logger common.Logger) *Lattice {

	// Create genesis latticeDataConfig.
	return &Lattice{
		signer:   signer,
		app:      app,
		debug:    debug,
		pool:     newBlockPool(cfg.NumChains),
		data:     newLatticeData(db, dMoment, round, cfg),
		toModule: newTotalOrdering(dMoment, round, cfg),
		ctModule: newConsensusTimestamp(dMoment, round, cfg.NumChains),
		logger:   logger,
	}
}

// PrepareBlock setups block's fields based on current status.
func (l *Lattice) PrepareBlock(
	b *types.Block, proposeTime time.Time) (err error) {

	l.lock.RLock()
	defer l.lock.RUnlock()

	b.Timestamp = proposeTime
	if err = l.data.prepareBlock(b); err != nil {
		return
	}
	l.logger.Debug("Calling Application.PreparePayload", "position", &b.Position)
	if b.Payload, err = l.app.PreparePayload(b.Position); err != nil {
		return
	}
	l.logger.Debug("Calling Application.PrepareWitness",
		"height", b.Witness.Height)
	if b.Witness, err = l.app.PrepareWitness(b.Witness.Height); err != nil {
		return
	}
	if err = l.signer.SignBlock(b); err != nil {
		return
	}
	return
}

// PrepareEmptyBlock setups block's fields based on current lattice status.
func (l *Lattice) PrepareEmptyBlock(b *types.Block) (err error) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	if err = l.data.prepareEmptyBlock(b); err != nil {
		return
	}
	if b.Hash, err = utils.HashBlock(b); err != nil {
		return
	}
	return
}

// SanityCheck checks the validity of a block.
//
// If any acking block of this block does not exist, Lattice caches this block
// and retries when Lattice.ProcessBlock is called.
func (l *Lattice) SanityCheck(b *types.Block) (err error) {
	if b.IsEmpty() {
		// Only need to verify block's hash.
		var hash common.Hash
		if hash, err = utils.HashBlock(b); err != nil {
			return
		}
		if b.Hash != hash {
			return ErrInvalidBlock
		}
	} else {
		// Verify block's signature.
		if err = utils.VerifyBlockSignature(b); err != nil {
			return
		}
	}
	// Make sure acks are sorted.
	for i := range b.Acks {
		if i == 0 {
			continue
		}
		if !b.Acks[i-1].Less(b.Acks[i]) {
			err = ErrAcksNotSorted
			return
		}
	}
	if err = func() (err error) {
		l.lock.RLock()
		defer l.lock.RUnlock()
		if err = l.data.sanityCheck(b); err != nil {
			if _, ok := err.(*ErrAckingBlockNotExists); ok {
				err = ErrRetrySanityCheckLater
			}
			return
		}
		return
	}(); err != nil {
		return
	}
	return
}

// Exist checks if the block is known to lattice.
func (l *Lattice) Exist(hash common.Hash) bool {
	l.lock.RLock()
	defer l.lock.RUnlock()
	if _, err := l.data.findBlock(hash); err != nil {
		return false
	}
	return true
}

// addBlockToLattice adds a block into lattice, and delivers blocks with the
// acks already delivered.
//
// NOTE: input block should pass sanity check.
func (l *Lattice) addBlockToLattice(
	input *types.Block) (outputBlocks []*types.Block, err error) {

	if tip := l.data.chains[input.Position.ChainID].tip; tip != nil {
		if !input.Position.Newer(&tip.Position) {
			l.logger.Warn("Dropping block: older than tip",
				"block", input, "tip", tip)
			return
		}
	}
	l.pool.addBlock(input)
	// Check tips in pool to check their validity for moving blocks from pool
	// to lattice.
	for {
		hasOutput := false
		for i := uint32(0); i < uint32(len(l.pool)); i++ {
			var tip *types.Block
			if tip = l.pool.tip(i); tip == nil {
				continue
			}
			err = l.data.sanityCheck(tip)
			if err == nil {
				var output []*types.Block
				if output, err = l.data.addBlock(tip); err != nil {
					// We should be able to add this block once sanity check
					// passed.
					l.logger.Error("Failed to add sanity-checked block",
						"block", tip, "error", err)
					panic(err)
				}
				hasOutput = true
				outputBlocks = append(outputBlocks, output...)
				l.pool.removeTip(i)
				continue
			}
			if _, ok := err.(*ErrAckingBlockNotExists); ok {
				l.logger.Debug("Pending block for lattice",
					"pending", tip,
					"err", err,
					"last", l.data.chains[tip.Position.ChainID].tip)
				err = nil
				continue
			} else {
				l.logger.Error("Unexpected sanity check error",
					"block", tip, "error", err)
				panic(err)
			}
		}
		if !hasOutput {
			break
		}
	}

	for _, b := range outputBlocks {
		l.logger.Debug("Calling Application.BlockConfirmed", "block", b)
		l.app.BlockConfirmed(*b.Clone())
		// Purge blocks in pool with the same chainID and lower height.
		l.pool.purgeBlocks(b.Position.ChainID, b.Position.Height)
	}

	return
}

// ProcessBlock adds a block into lattice, and deliver ordered blocks.
// If any block pass sanity check after this block add into lattice, they
// would be returned, too.
//
// NOTE: assume the block passed sanity check.
func (l *Lattice) ProcessBlock(
	input *types.Block) (delivered []*types.Block, err error) {
	var (
		b             *types.Block
		inLattice     []*types.Block
		toDelivered   []*types.Block
		deliveredMode uint32
	)
	l.lock.Lock()
	defer l.lock.Unlock()
	if inLattice, err = l.addBlockToLattice(input); err != nil {
		return
	}
	if len(inLattice) == 0 {
		return
	}
	for _, b = range inLattice {
		if err = l.toModule.addBlock(b); err != nil {
			// All errors from total ordering is serious, should panic.
			panic(err)
		}
	}
	for {
		toDelivered, deliveredMode, err = l.toModule.extractBlocks()
		if err != nil {
			panic(err)
		}
		if len(toDelivered) == 0 {
			break
		}
		hashes := make(common.Hashes, len(toDelivered))
		for idx := range toDelivered {
			hashes[idx] = toDelivered[idx].Hash
		}
		if l.debug != nil {
			l.debug.TotalOrderingDelivered(hashes, deliveredMode)
		}
		// Perform consensus timestamp module.
		if err = l.ctModule.processBlocks(toDelivered); err != nil {
			break
		}
		delivered = append(delivered, toDelivered...)
	}
	return
}

// NextHeight returns expected height of incoming block for specified chain and
// given round.
func (l *Lattice) NextHeight(round uint64, chainID uint32) (uint64, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	return l.data.nextHeight(round, chainID)
}

// PurgeBlocks purges blocks' cache in memory, this is called when the caller
// makes sure those blocks are already saved in db.
func (l *Lattice) PurgeBlocks(blocks []*types.Block) error {
	l.lock.Lock()
	defer l.lock.Unlock()
	return l.data.purgeBlocks(blocks)
}

// AppendConfig adds a new config for upcoming rounds. If a config of round r is
// added, only config in round r + 1 is allowed next.
func (l *Lattice) AppendConfig(round uint64, config *types.Config) (err error) {
	l.lock.Lock()
	defer l.lock.Unlock()
	l.pool.resize(config.NumChains)
	if err = l.data.appendConfig(round, config); err != nil {
		return
	}
	if err = l.toModule.appendConfig(round, config); err != nil {
		return
	}
	if err = l.ctModule.appendConfig(round, config); err != nil {
		return
	}
	return
}

// ProcessFinalizedBlock is used for syncing lattice data.
func (l *Lattice) ProcessFinalizedBlock(
	b *types.Block) (delivered []*types.Block, err error) {
	var (
		toDelivered   []*types.Block
		deliveredMode uint32
	)
	l.lock.Lock()
	defer l.lock.Unlock()
	// Syncing state for core.latticeData module.
	if err = l.data.addFinalizedBlock(b); err != nil {
		return
	}
	l.pool.purgeBlocks(b.Position.ChainID, b.Position.Height)
	// Syncing state for core.totalOrdering module.
	if err = l.toModule.addBlock(b); err != nil {
		return
	}
	for {
		toDelivered, deliveredMode, err = l.toModule.extractBlocks()
		if err != nil || len(toDelivered) == 0 {
			break
		}
		hashes := make(common.Hashes, len(toDelivered))
		for idx := range toDelivered {
			hashes[idx] = toDelivered[idx].Hash
		}
		if l.debug != nil {
			l.debug.TotalOrderingDelivered(hashes, deliveredMode)
		}
		// Sync core.consensusTimestamp module.
		if err = l.ctModule.processBlocks(toDelivered); err != nil {
			break
		}
		delivered = append(delivered, toDelivered...)
	}
	return
}
