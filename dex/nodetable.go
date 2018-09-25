package dex

import (
	"net"
	"sync"

	"github.com/dexon-foundation/dexon/common"
	"github.com/dexon-foundation/dexon/crypto/sha3"
	"github.com/dexon-foundation/dexon/event"
	"github.com/dexon-foundation/dexon/p2p/discover"
	"github.com/dexon-foundation/dexon/rlp"
)

type NodeMeta struct {
	ID        discover.NodeID
	IP        net.IP
	UDP       uint16
	TCP       uint16
	Timestamp uint64
	Sig       []byte
}

func (n *NodeMeta) Hash() (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, n)
	hw.Sum(h[:0])
	return h
}

type newMetasEvent struct{ Metas []*NodeMeta }

type nodeTable struct {
	mu    sync.RWMutex
	entry map[discover.NodeID]*NodeMeta
	feed  event.Feed
}

func newNodeTable() *nodeTable {
	return &nodeTable{
		entry: make(map[discover.NodeID]*NodeMeta),
	}
}

func (t *nodeTable) Get(id discover.NodeID) *NodeMeta {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.entry[id]
}

func (t *nodeTable) Add(metas []*NodeMeta) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var newMetas []*NodeMeta
	for _, meta := range metas {
		// TODO: validate the meta
		if e, ok := t.entry[meta.ID]; ok && e.Timestamp > meta.Timestamp {
			continue
		}
		t.entry[meta.ID] = meta
		newMetas = append(newMetas, meta)
	}
	t.feed.Send(newMetasEvent{newMetas})
}

func (t *nodeTable) Metas() []*NodeMeta {
	t.mu.RLock()
	defer t.mu.RUnlock()
	metas := make([]*NodeMeta, 0, len(t.entry))
	for _, meta := range t.entry {
		metas = append(metas, meta)
	}
	return metas
}

func (t *nodeTable) SubscribeNewMetasEvent(
	ch chan<- newMetasEvent) event.Subscription {
	return t.feed.Subscribe(ch)
}
