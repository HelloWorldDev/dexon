// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// This file contains some shares testing functionality, common to  multiple
// different files and modules being tested.

package dex

import (
	"crypto/ecdsa"
	"math/big"
	"sort"
	"sync"
	"testing"

	"github.com/dexon-foundation/dexon/common"
	"github.com/dexon-foundation/dexon/consensus/ethash"
	"github.com/dexon-foundation/dexon/core"
	"github.com/dexon-foundation/dexon/core/types"
	"github.com/dexon-foundation/dexon/core/vm"
	"github.com/dexon-foundation/dexon/crypto"
	"github.com/dexon-foundation/dexon/eth/downloader"
	"github.com/dexon-foundation/dexon/ethdb"
	"github.com/dexon-foundation/dexon/event"
	"github.com/dexon-foundation/dexon/p2p"
	"github.com/dexon-foundation/dexon/p2p/discover"
	"github.com/dexon-foundation/dexon/params"
)

var (
	testBankKey, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	testBank       = crypto.PubkeyToAddress(testBankKey.PublicKey)
)

// testP2PServer is a fake, helper p2p server for testing purposes.
type testP2PServer struct {
	mu     sync.Mutex
	self   *discover.Node
	key    *ecdsa.PrivateKey
	direct map[discover.NodeID]*discover.Node
	group  map[string][]*discover.Node
}

func newTestP2PServer(self *discover.Node, key *ecdsa.PrivateKey) *testP2PServer {
	return &testP2PServer{
		self:   self,
		key:    key,
		direct: make(map[discover.NodeID]*discover.Node),
		group:  make(map[string][]*discover.Node),
	}
}

func (s *testP2PServer) Self() *discover.Node {
	return s.self
}

func (s *testP2PServer) GetPrivateKey() *ecdsa.PrivateKey {
	return s.key
}

func (s *testP2PServer) AddDirectPeer(node *discover.Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.direct[node.ID] = node
}

func (s *testP2PServer) RemoveDirectPeer(node *discover.Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.direct, node.ID)
}

func (s *testP2PServer) AddGroup(
	name string, nodes []*discover.Node, num uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.group[name] = nodes
}

func (s *testP2PServer) RemoveGroup(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.group, name)
}

// newTestProtocolManager creates a new protocol manager for testing purposes,
// with the given number of blocks already known, and potential notification
// channels for different events.
func newTestProtocolManager(mode downloader.SyncMode, blocks int, generator func(int, *core.BlockGen), newtx chan<- []*types.Transaction) (*ProtocolManager, *ethdb.MemDatabase, error) {
	var (
		evmux  = new(event.TypeMux)
		engine = ethash.NewFaker()
		db     = ethdb.NewMemDatabase()
		gspec  = &core.Genesis{
			Config: params.TestChainConfig,
			Alloc:  core.GenesisAlloc{testBank: {Balance: big.NewInt(1000000), Staked: big.NewInt(0)}},
		}
		genesis       = gspec.MustCommit(db)
		blockchain, _ = core.NewBlockChain(db, nil, gspec.Config, engine, vm.Config{})
	)
	chain, _ := core.GenerateChain(gspec.Config, genesis, ethash.NewFaker(), db, blocks, generator)
	if _, err := blockchain.InsertChain(chain); err != nil {
		panic(err)
	}

	tgov := &testGovernance{
		numChainsFunc: func(uint64) uint32 { return 3 },
		lenCRSFunc:    func() uint64 { return 1 },
		dkgSetFunc:    func(uint64) (map[string]struct{}, error) { return nil, nil },
		notarySetFunc: func(uint64, uint32) (map[string]struct{}, error) { return nil, nil },
	}

	pm, err := NewProtocolManager(gspec.Config, mode, DefaultConfig.NetworkId, evmux, &testTxPool{added: newtx}, engine, blockchain, db, tgov)
	if err != nil {
		return nil, nil, err
	}

	key, err := crypto.GenerateKey()
	if err != nil {
		return nil, nil, err
	}
	pm.Start(newTestP2PServer(&discover.Node{}, key), 1000)
	return pm, db, nil
}

// newTestProtocolManagerMust creates a new protocol manager for testing purposes,
// with the given number of blocks already known, and potential notification
// channels for different events. In case of an error, the constructor force-
// fails the test.
func newTestProtocolManagerMust(t *testing.T, mode downloader.SyncMode, blocks int, generator func(int, *core.BlockGen), newtx chan<- []*types.Transaction) (*ProtocolManager, *ethdb.MemDatabase) {
	pm, db, err := newTestProtocolManager(mode, blocks, generator, newtx)
	if err != nil {
		t.Fatalf("Failed to create protocol manager: %v", err)
	}
	return pm, db
}

// testTxPool is a fake, helper transaction pool for testing purposes
type testTxPool struct {
	txFeed event.Feed
	pool   []*types.Transaction        // Collection of all transactions
	added  chan<- []*types.Transaction // Notification channel for new transactions

	lock sync.RWMutex // Protects the transaction pool
}

// AddRemotes appends a batch of transactions to the pool, and notifies any
// listeners if the addition channel is non nil
func (p *testTxPool) AddRemotes(txs []*types.Transaction) []error {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.pool = append(p.pool, txs...)
	if p.added != nil {
		p.added <- txs
	}
	return make([]error, len(txs))
}

// Pending returns all the transactions known to the pool
func (p *testTxPool) Pending() (map[common.Address]types.Transactions, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	batches := make(map[common.Address]types.Transactions)
	for _, tx := range p.pool {
		from, _ := types.Sender(types.HomesteadSigner{}, tx)
		batches[from] = append(batches[from], tx)
	}
	for _, batch := range batches {
		sort.Sort(types.TxByNonce(batch))
	}
	return batches, nil
}

func (p *testTxPool) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return p.txFeed.Subscribe(ch)
}

// newTestTransaction create a new dummy transaction.
func newTestTransaction(from *ecdsa.PrivateKey, nonce uint64, datasize int) *types.Transaction {
	tx := types.NewTransaction(nonce, common.Address{}, big.NewInt(0), 100000, big.NewInt(0), make([]byte, datasize))
	tx, _ = types.SignTx(tx, types.HomesteadSigner{}, from)
	return tx
}

// testGovernance is a fake, helper governance for testing purposes
type testGovernance struct {
	numChainsFunc func(uint64) uint32
	lenCRSFunc    func() uint64
	notarySetFunc func(uint64, uint32) (map[string]struct{}, error)
	dkgSetFunc    func(uint64) (map[string]struct{}, error)
}

func (g *testGovernance) GetNumChains(round uint64) uint32 {
	return g.numChainsFunc(round)
}

func (g *testGovernance) LenCRS() uint64 {
	return g.lenCRSFunc()
}

func (g *testGovernance) NotarySet(
	round uint64, chainID uint32) (map[string]struct{}, error) {
	return g.notarySetFunc(round, chainID)
}

func (g *testGovernance) DKGSet(round uint64) (map[string]struct{}, error) {
	return g.dkgSetFunc(round)
}

// testPeer is a simulated peer to allow testing direct network calls.
type testPeer struct {
	net p2p.MsgReadWriter // Network layer reader/writer to simulate remote messaging
	app *p2p.MsgPipeRW    // Application layer reader/writer to simulate the local side
	*peer
}

// newTestPeer creates a new peer registered at the given protocol manager.
func newTestPeer(name string, version int, pm *ProtocolManager, shake bool) (*testPeer, <-chan error) {
	// Create a message pipe to communicate through
	app, net := p2p.MsgPipe()

	// Generate a random id and create the peer
	id := randomID()

	peer := pm.newPeer(version, p2p.NewPeer(id, name, nil), net)

	// Start the peer on a new thread
	errc := make(chan error, 1)
	go func() {
		select {
		case pm.newPeerCh <- peer:
			errc <- pm.handle(peer)
		case <-pm.quitSync:
			errc <- p2p.DiscQuitting
		}
	}()
	tp := &testPeer{app: app, net: net, peer: peer}
	// Execute any implicitly requested handshakes and return
	if shake {
		var (
			genesis = pm.blockchain.Genesis()
			head    = pm.blockchain.CurrentHeader()
			td      = pm.blockchain.GetTd(head.Hash(), head.Number.Uint64())
		)
		tp.handshake(nil, td, head.Hash(), genesis.Hash())
	}
	return tp, errc
}

// handshake simulates a trivial handshake that expects the same state from the
// remote side as we are simulating locally.
func (p *testPeer) handshake(t *testing.T, td *big.Int, head common.Hash, genesis common.Hash) {
	msg := &statusData{
		ProtocolVersion: uint32(p.version),
		NetworkId:       DefaultConfig.NetworkId,
		TD:              td,
		CurrentBlock:    head,
		GenesisBlock:    genesis,
	}
	if err := p2p.ExpectMsg(p.app, StatusMsg, msg); err != nil {
		t.Fatalf("status recv: %v", err)
	}
	if err := p2p.Send(p.app, StatusMsg, msg); err != nil {
		t.Fatalf("status send: %v", err)
	}
}

// close terminates the local side of the peer, notifying the remote protocol
// manager of termination.
func (p *testPeer) close() {
	p.app.Close()
}
