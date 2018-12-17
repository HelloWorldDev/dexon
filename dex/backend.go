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
	"fmt"
	"math/big"
	"time"

	dexCore "github.com/dexon-foundation/dexon-consensus/core"
	coreEcdsa "github.com/dexon-foundation/dexon-consensus/core/crypto/ecdsa"
	coreTypes "github.com/dexon-foundation/dexon-consensus/core/types"

	"github.com/dexon-foundation/dexon/accounts"
	"github.com/dexon-foundation/dexon/consensus"
	"github.com/dexon-foundation/dexon/consensus/dexcon"
	"github.com/dexon-foundation/dexon/core"
	"github.com/dexon-foundation/dexon/core/bloombits"
	"github.com/dexon-foundation/dexon/core/rawdb"
	"github.com/dexon-foundation/dexon/core/types"
	"github.com/dexon-foundation/dexon/core/vm"
	"github.com/dexon-foundation/dexon/dex/downloader"
	"github.com/dexon-foundation/dexon/eth/filters"
	"github.com/dexon-foundation/dexon/eth/gasprice"
	"github.com/dexon-foundation/dexon/ethdb"
	"github.com/dexon-foundation/dexon/event"
	"github.com/dexon-foundation/dexon/indexer"
	"github.com/dexon-foundation/dexon/internal/ethapi"
	"github.com/dexon-foundation/dexon/log"
	"github.com/dexon-foundation/dexon/node"
	"github.com/dexon-foundation/dexon/p2p"
	"github.com/dexon-foundation/dexon/params"
	"github.com/dexon-foundation/dexon/rlp"
	"github.com/dexon-foundation/dexon/rpc"
)

// Dexon implements the DEXON fullnode service.
type Dexon struct {
	config      *Config
	chainConfig *params.ChainConfig

	// Channel for shutting down the service
	shutdownChan chan bool // Channel for shutting down the Ethereum

	// Handlers
	txPool          *core.TxPool
	blockchain      *core.BlockChain
	protocolManager *ProtocolManager

	// DB interfaces
	chainDb ethdb.Database // Block chain database

	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *accounts.Manager

	bloomRequests chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer  *core.ChainIndexer             // Bloom indexer operating during block imports

	APIBackend *DexAPIBackend

	// Dexon consensus.
	app        *DexconApp
	governance *DexconGovernance
	network    *DexconNetwork

	bp *blockProposer

	networkID     uint64
	netRPCService *ethapi.PublicNetAPI

	indexer indexer.Indexer
}

func New(ctx *node.ServiceContext, config *Config, hardfork bool) (*Dexon, error) {
	// Consensus.
	chainDb, err := CreateDB(ctx, config, "chaindata")
	if err != nil {
		return nil, err
	}
	chainConfig, genesisHash, genesisErr := core.SetupGenesisBlock(chainDb,
		config.Genesis)
	if _, ok := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !ok {
		return nil, genesisErr
	}
	log.Info("Initialised chain configuration", "config", chainConfig)

	if !config.SkipBcVersionCheck {
		bcVersion := rawdb.ReadDatabaseVersion(chainDb)
		if bcVersion != core.BlockChainVersion && bcVersion != 0 {
			return nil, fmt.Errorf("Blockchain DB version mismatch (%d / %d).\n",
				bcVersion, core.BlockChainVersion)
		}
		rawdb.WriteDatabaseVersion(chainDb, core.BlockChainVersion)
	}
	engine := dexcon.New()

	dex := &Dexon{
		config:         config,
		chainDb:        chainDb,
		chainConfig:    chainConfig,
		eventMux:       ctx.EventMux,
		accountManager: ctx.AccountManager,
		shutdownChan:   make(chan bool),
		networkID:      config.NetworkId,
		bloomRequests:  make(chan chan *bloombits.Retrieval),
		bloomIndexer:   NewBloomIndexer(chainDb, params.BloomBitsBlocks, params.BloomConfirms),
		engine:         engine,
	}

	var (
		vmConfig = vm.Config{
			EnablePreimageRecording: config.EnablePreimageRecording,
			EWASMInterpreter:        config.EWASMInterpreter,
			EVMInterpreter:          config.EVMInterpreter,
			IsBlockProposer:         config.BlockProposerEnabled,
		}
		cacheConfig = &core.CacheConfig{Disabled: config.NoPruning, TrieCleanLimit: config.TrieCleanCache, TrieDirtyLimit: config.TrieDirtyCache, TrieTimeLimit: config.TrieTimeout}
	)
	dex.blockchain, err = core.NewBlockChain(chainDb, cacheConfig, dex.chainConfig, dex.engine, vmConfig, nil)

	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*params.ConfigCompatError); ok {
		log.Warn("Rewinding chain to upgrade configuration", "err", compat)
		dex.blockchain.SetHead(compat.RewindTo)
		rawdb.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}
	dex.bloomIndexer.Start(dex.blockchain)

	if config.Indexer.Enable {
		dex.indexer = indexer.NewIndexerFromConfig(
			indexer.NewROBlockChain(dex.blockchain),
			config.Indexer,
		)
		dex.indexer.Start()
	}

	if config.TxPool.Journal != "" {
		config.TxPool.Journal = ctx.ResolvePath(config.TxPool.Journal)
	}
	dex.txPool = core.NewTxPool(config.TxPool, dex.chainConfig, dex.blockchain, config.BlockProposerEnabled)

	dex.APIBackend = &DexAPIBackend{dex, nil}
	gpoParams := config.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = config.DefaultGasPrice
	}
	dex.APIBackend.gpo = gasprice.NewOracle(dex.APIBackend, gpoParams)

	// Dexcon related objects.
	dex.governance = NewDexconGovernance(dex.APIBackend, dex.chainConfig, config.PrivateKey)
	dex.app = NewDexconApp(dex.txPool, dex.blockchain, dex.governance, chainDb, config)

	if hardfork {
		dex.hardfork()
	}

	// Set config fetcher so engine can fetch current system configuration from state.
	engine.SetConfigFetcher(dex.governance)

	dMoment := time.Unix(config.DMoment, int64(0))
	log.Info("DEXON Consensus DMoment", "time", dMoment)

	// Force starting with full sync mode if this node is a bootstrap proposer.
	if config.BlockProposerEnabled && dMoment.After(time.Now()) {
		config.SyncMode = downloader.FullSync
	}

	pm, err := NewProtocolManager(dex.chainConfig, config.SyncMode,
		config.NetworkId, dex.eventMux, dex.txPool, dex.engine, dex.blockchain,
		chainDb, config.BlockProposerEnabled, dex.governance, dex.app)
	if err != nil {
		return nil, err
	}

	dex.protocolManager = pm
	dex.network = NewDexconNetwork(pm)

	dex.bp = NewBlockProposer(dex, dMoment)
	return dex, nil
}

func (s *Dexon) Protocols() []p2p.Protocol {
	return s.protocolManager.SubProtocols
}

func (s *Dexon) APIs() []rpc.API {
	apis := ethapi.GetAPIs(s.APIBackend)

	// Append any APIs exposed explicitly by the consensus engine
	apis = append(apis, s.engine.APIs(s.BlockChain())...)

	// Append all the local APIs and return
	return append(apis, []rpc.API{
		{
			Namespace: "eth",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(s.protocolManager.downloader, s.eventMux),
			Public:    true,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(s.APIBackend, false),
			Public:    true,
		}, {
			Namespace: "admin",
			Version:   "1.0",
			Service:   NewPrivateAdminAPI(s),
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPublicDebugAPI(s),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPrivateDebugAPI(s.chainConfig, s),
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   s.netRPCService,
			Public:    true,
		},
	}...)
}

func (s *Dexon) Start(srvr *p2p.Server) error {
	// Start the bloom bits servicing goroutines
	s.startBloomHandlers(params.BloomBitsBlocks)

	// Start the RPC service
	s.netRPCService = ethapi.NewPublicNetAPI(srvr, s.NetVersion())

	// Figure out a max peers count based on the server limits
	maxPeers := srvr.MaxPeers
	if s.config.LightServ > 0 {
		if s.config.LightPeers >= srvr.MaxPeers {
			return fmt.Errorf("invalid peer config: light peer count (%d) >= total peer count (%d)", s.config.LightPeers, srvr.MaxPeers)
		}
		maxPeers -= s.config.LightPeers
	}
	// Start the networking layer and the light server if requested
	s.protocolManager.Start(srvr, maxPeers)
	s.protocolManager.addSelfMeta()
	return nil
}

func (s *Dexon) Stop() error {
	s.bp.Stop()
	s.app.Stop()
	if s.indexer != nil {
		s.indexer.Stop()
	}
	return nil
}

func (s *Dexon) StartProposing() error {
	return s.bp.Start()
}

func (s *Dexon) StopProposing() {
	s.bp.Stop()
}

func (s *Dexon) IsLatticeSyncing() bool {
	return s.bp.IsLatticeSyncing()
}

func (s *Dexon) IsProposing() bool {
	return s.bp.IsProposing()
}

func (s *Dexon) hardfork() {
	curBlock := s.blockchain.CurrentBlock()
	height := curBlock.NumberU64()
	log.Info("Hard fork", "height", height, "root", curBlock.Root())
	s.app.offset = height + 2
	// purgeGovernance
	state, err := s.blockchain.State()
	if err != nil {
		panic(err)
	}
	vm.PurgeGovernanceContract(state)
	// Insert a block with new Governance state.
	root, err := state.Commit(true)
	if err != nil {
		log.Error("Failed to commit", "error", err)
		panic(err)
	}

	log.Info("New state root", "root", root)

	newBlock := types.NewBlock(&types.Header{
		Number:     new(big.Int).SetUint64(height + 1),
		Time:       new(big.Int).Set(curBlock.Time()),
		GasLimit:   s.governance.DexconConfiguration(0).BlockGasLimit,
		Difficulty: big.NewInt(1),
		Root:       root,
		Extra:      []byte("DEXON hardfork"),
	}, nil, nil, nil)

	data, err := rlp.EncodeToBytes(&witnessData{
		Root:        curBlock.Root(),
		TxHash:      curBlock.TxHash(),
		ReceiptHash: curBlock.ReceiptHash(),
	})
	if err != nil {
		log.Error("Failed to encode rlp", "error", err)
		panic(err)
	}
	dummyWitness := coreTypes.Witness{
		Height: height,
		Data:   data,
	}

	_, err = s.blockchain.ProcessPendingBlock(newBlock, &dummyWitness)
	if err != nil {
		log.Error("Failed to process hard fork block", "error", err)
		panic(err)
	}
	// Insert a block witness new Governance state.
	newBlock = s.blockchain.PendingBlock()
	if height+1 != newBlock.NumberU64() {
		panic("not inserted to pending block")
	}
	if newBlock.Root() != root {
		log.Error("Root not equal", "expected", root, "has", newBlock.Root())
		panic("root not updated")
	}
	witnessBlock := types.NewBlock(&types.Header{
		Number:     new(big.Int).SetUint64(height + 2),
		Time:       new(big.Int).Set(curBlock.Time()),
		GasLimit:   s.governance.DexconConfiguration(0).BlockGasLimit,
		Difficulty: big.NewInt(1),
		Root:       root,
	}, nil, nil, nil)

	data, err = rlp.EncodeToBytes(&witnessData{
		Root:        newBlock.Root(),
		TxHash:      newBlock.TxHash(),
		ReceiptHash: newBlock.ReceiptHash(),
	})
	if err != nil {
		log.Error("Failed to encode rlp", "error", err)
		panic(err)
	}
	witness := coreTypes.Witness{
		Height: height + 1,
		Data:   data,
	}
	_, err = s.blockchain.ProcessPendingBlock(witnessBlock, &witness)
	if err != nil {
		log.Error("Failed to process hard fork block", "error", err)
		panic(err)
	}
}

// CreateDB creates the chain database.
func CreateDB(ctx *node.ServiceContext, config *Config, name string) (ethdb.Database, error) {
	db, err := ctx.OpenDatabase(name, config.DatabaseCache, config.DatabaseHandles)
	if err != nil {
		return nil, err
	}
	if db, ok := db.(*ethdb.LDBDatabase); ok {
		db.Meter("eth/db/chaindata/")
	}
	return db, nil
}

func (d *Dexon) AccountManager() *accounts.Manager { return d.accountManager }
func (d *Dexon) BlockChain() *core.BlockChain      { return d.blockchain }
func (d *Dexon) TxPool() *core.TxPool              { return d.txPool }
func (d *Dexon) DexVersion() int                   { return int(d.protocolManager.SubProtocols[0].Version) }
func (d *Dexon) EventMux() *event.TypeMux          { return d.eventMux }
func (d *Dexon) Engine() consensus.Engine          { return d.engine }
func (d *Dexon) ChainDb() ethdb.Database           { return d.chainDb }
func (d *Dexon) Downloader() ethapi.Downloader     { return d.protocolManager.downloader }
func (d *Dexon) NetVersion() uint64                { return d.networkID }
