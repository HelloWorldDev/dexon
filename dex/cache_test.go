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

package dex

import (
	"math/rand"
	"sort"
	"strings"
	"testing"

	coreCommon "github.com/dexon-foundation/dexon-consensus/common"
	coreDb "github.com/dexon-foundation/dexon-consensus/core/db"
	coreTypes "github.com/dexon-foundation/dexon-consensus/core/types"
)

type byHash []*coreTypes.Vote

func (v byHash) Len() int {
	return len(v)
}

func (v byHash) Less(i int, j int) bool {
	return strings.Compare(v[i].BlockHash.String(), v[j].BlockHash.String()) < 0
}

func (v byHash) Swap(i int, j int) {
	v[i], v[j] = v[j], v[i]
}

func TestCacheVote(t *testing.T) {
	db, err := coreDb.NewMemBackedDB()
	if err != nil {
		panic(err)
	}
	cache := newCache(3, db)
	pos0 := coreTypes.Position{
		Height: uint64(0),
	}
	pos1 := coreTypes.Position{
		Height: uint64(1),
	}
	vote1 := &coreTypes.Vote{
		VoteHeader: coreTypes.VoteHeader{
			BlockHash: coreCommon.NewRandomHash(),
			Position:  pos0,
		},
	}
	vote2 := &coreTypes.Vote{
		VoteHeader: coreTypes.VoteHeader{
			BlockHash: coreCommon.NewRandomHash(),
			Position:  pos0,
		},
	}
	vote3 := &coreTypes.Vote{
		VoteHeader: coreTypes.VoteHeader{
			BlockHash: coreCommon.NewRandomHash(),
			Position:  pos1,
		},
	}
	vote4 := &coreTypes.Vote{
		VoteHeader: coreTypes.VoteHeader{
			BlockHash: coreCommon.NewRandomHash(),
			Position:  pos1,
		},
	}
	cache.addVote(vote1)
	cache.addVote(vote2)
	cache.addVote(vote3)

	votes := cache.votes(pos0)
	sort.Sort(byHash(votes))

	resultVotes := []*coreTypes.Vote{vote1, vote2}
	sort.Sort(byHash(resultVotes))

	if len(votes) != 2 {
		t.Errorf("fail to get votes: have %d, want 2", len(votes))
	}
	if !votes[0].BlockHash.Equal(resultVotes[0].BlockHash) {
		t.Errorf("get wrong vote: have %s, want %s", votes[0], resultVotes[0])
	}
	if !votes[1].BlockHash.Equal(resultVotes[1].BlockHash) {
		t.Errorf("get wrong vote: have %s, want %s", votes[1], resultVotes[1])
	}
	votes = cache.votes(pos1)
	sort.Sort(byHash(votes))
	if len(votes) != 1 {
		t.Errorf("fail to get votes: have %d, want 1", len(votes))
	}
	if !votes[0].BlockHash.Equal(vote3.BlockHash) {
		t.Errorf("get wrong vote: have %s, want %s", votes[0], vote3)
	}

	cache.addVote(vote4)

	votes = cache.votes(pos0)
	sort.Sort(byHash(votes))

	if len(votes) != 0 {
		t.Errorf("fail to get votes: have %d, want 0", len(votes))
	}
	votes = cache.votes(pos1)
	sort.Sort(byHash(votes))

	resultVotes = []*coreTypes.Vote{vote3, vote4}
	sort.Sort(byHash(resultVotes))

	if len(votes) != 2 {
		t.Errorf("fail to get votes: have %d, want 1", len(votes))
	}
	if !votes[0].BlockHash.Equal(resultVotes[0].BlockHash) {
		t.Errorf("get wrong vote: have %s, want %s", votes[0], resultVotes[0])
	}
	if !votes[1].BlockHash.Equal(resultVotes[1].BlockHash) {
		t.Errorf("get wrong vote: have %s, want %s", votes[1], resultVotes[1])
	}
}

func TestCacheBlock(t *testing.T) {
	db, err := coreDb.NewMemBackedDB()
	if err != nil {
		panic(err)
	}
	cache := newCache(3, db)
	block1 := &coreTypes.Block{
		Hash: coreCommon.NewRandomHash(),
	}
	block2 := &coreTypes.Block{
		Hash: coreCommon.NewRandomHash(),
	}
	block3 := &coreTypes.Block{
		Hash: coreCommon.NewRandomHash(),
	}
	block4 := &coreTypes.Block{
		Hash: coreCommon.NewRandomHash(),
	}
	cache.addBlock(block1)
	cache.addBlock(block2)
	cache.addBlock(block3)

	hashes := coreCommon.Hashes{block1.Hash, block2.Hash, block3.Hash, block4.Hash}
	hashMap := map[coreCommon.Hash]struct{}{
		block1.Hash: {},
		block2.Hash: {},
		block3.Hash: {},
	}
	blocks := cache.blocks(hashes)
	if len(blocks) != 3 {
		t.Errorf("fail to get blocks: have %d, want 3", len(blocks))
	}
	for _, block := range blocks {
		if _, exist := hashMap[block.Hash]; !exist {
			t.Errorf("get wrong block: have %s, want %v", block, hashMap)
		}
	}

	cache.addBlock(block4)

	blocks = cache.blocks(hashes)
	hashMap[block4.Hash] = struct{}{}
	if len(blocks) != 3 {
		t.Errorf("fail to get blocks: have %d, want 3", len(blocks))
	}
	hasNewBlock := false
	for _, block := range blocks {
		if _, exist := hashMap[block.Hash]; !exist {
			t.Errorf("get wrong block: have %s, want %v", block, hashMap)
		}
		if block.Hash.Equal(block4.Hash) {
			hasNewBlock = true
		}
	}
	if !hasNewBlock {
		t.Errorf("expect block %s in cache, have %v", block4, blocks)
	}

	block5 := &coreTypes.Block{
		Hash: coreCommon.NewRandomHash(),
	}
	if err := db.PutBlock(*block5); err != nil {
		panic(err)
	}
	blocks = cache.blocks(coreCommon.Hashes{block5.Hash})
	if len(blocks) != 1 {
		t.Errorf("fail to get blocks: have %d, want 1", len(blocks))
	} else {
		if !blocks[0].Hash.Equal(block5.Hash) {
			t.Errorf("get wrong block: have %s, want %s", blocks[0], block5)
		}
	}
}

func randomBytes() []byte {
	bytes := make([]byte, 32)
	for i := range bytes {
		bytes[i] = byte(rand.Int() % 256)
	}
	return bytes
}

func TestCacheRandomness(t *testing.T) {
	db, err := coreDb.NewMemBackedDB()
	if err != nil {
		panic(err)
	}
	cache := newCache(3, db)
	rand1 := &coreTypes.BlockRandomnessResult{
		BlockHash:  coreCommon.NewRandomHash(),
		Randomness: randomBytes(),
	}
	rand2 := &coreTypes.BlockRandomnessResult{
		BlockHash:  coreCommon.NewRandomHash(),
		Randomness: randomBytes(),
	}
	rand3 := &coreTypes.BlockRandomnessResult{
		BlockHash:  coreCommon.NewRandomHash(),
		Randomness: randomBytes(),
	}
	rand4 := &coreTypes.BlockRandomnessResult{
		BlockHash:  coreCommon.NewRandomHash(),
		Randomness: randomBytes(),
	}
	cache.addRandomness(rand1)
	cache.addRandomness(rand2)
	cache.addRandomness(rand3)

	hashes := coreCommon.Hashes{rand1.BlockHash, rand2.BlockHash, rand3.BlockHash, rand4.BlockHash}
	hashMap := map[coreCommon.Hash]struct{}{
		rand1.BlockHash: {},
		rand2.BlockHash: {},
		rand3.BlockHash: {},
	}
	rands := cache.randomness(hashes)
	if len(rands) != 3 {
		t.Errorf("fail to get rands: have %d, want 3", len(rands))
	}
	for _, rand := range rands {
		if _, exist := hashMap[rand.BlockHash]; !exist {
			t.Errorf("get wrong rand: have %s, want %v", rand, hashMap)
		}
	}

	cache.addRandomness(rand4)

	rands = cache.randomness(hashes)
	hashMap[rand4.BlockHash] = struct{}{}
	if len(rands) != 3 {
		t.Errorf("fail to get rands: have %d, want 3", len(rands))
	}
	hasNewRandomness := false
	for _, rand := range rands {
		if _, exist := hashMap[rand.BlockHash]; !exist {
			t.Errorf("get wrong rand: have %s, want %v", rand, hashMap)
		}
		if rand.BlockHash.Equal(rand4.BlockHash) {
			hasNewRandomness = true
		}
	}
	if !hasNewRandomness {
		t.Errorf("expect rand %s in cache, have %v", rand4, rands)
	}

	block := &coreTypes.Block{
		Hash: coreCommon.NewRandomHash(),
		Finalization: coreTypes.FinalizationResult{
			Randomness: randomBytes(),
		},
	}
	if err := db.PutBlock(*block); err != nil {
		panic(err)
	}
	rands = cache.randomness(coreCommon.Hashes{block.Hash})
	if len(rands) != 1 {
		t.Errorf("fail to get rands: have %d, want 1", len(rands))
	} else {
		if !rands[0].BlockHash.Equal(block.Hash) {
			t.Errorf("get wrong rand: have %s, want %s", rands[0], block)
		}
	}
}
