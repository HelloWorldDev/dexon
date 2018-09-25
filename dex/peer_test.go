package dex

import "testing"

func TestPeerSetAddNotaryPeer(t *testing.T) {
	ps := newPeerSet()
	ps.SetRound(10)

	p := &peer{id: "register peer"}

	if err := ps.Register(p); err != nil {
		t.Fatalf("register peer fail: %v", err)
	}

	tests := []struct {
		round uint64
		p     *peer
		err   error
		ok    bool
		pp    *peer
	}{
		{10, p, nil, true, p},
		{9, p, errInvalidRound, false, nil},
		{13, p, errInvalidRound, false, nil},
		{10, &peer{id: "not register peer"}, errNotRegistered, false, nil},
	}

	for _, tt := range tests {
		err := ps.AddNotaryPeer(tt.round, tt.p)
		if err != tt.err {
			t.Errorf("err mismatch: got %v, want %v", err, tt.err)
		}

		p, ok := ps.notaryPeers[tt.round][tt.p.id]

		if ok != tt.ok {
			t.Errorf("lookup ok mismatched, got %v, want %v", ok, tt.ok)
		}

		if p != tt.pp {
			t.Errorf("lookup peer mismatched, got %v, want %v", p, tt.pp)
		}
	}
}

func TestPeerSetNotaryPeer(t *testing.T) {
	ps := newPeerSet()
	notaryPeers := map[uint64]map[string]*peer{
		10: map[string]*peer{
			"r10p1": &peer{id: "r10p1"},
			"r10p2": &peer{id: "r10p2"},
		},
		11: map[string]*peer{
			"r11p1": &peer{id: "r11p1"},
			"r11p2": &peer{id: "r11p2"},
		},
	}

	for _, peers := range notaryPeers {
		for _, p := range peers {
			if err := ps.Register(p); err != nil {
				t.Errorf("register peer fail: %v", err)
			}
		}
	}

	ps.notaryPeers = notaryPeers

	tests := []struct {
		round uint64
		peers map[string]struct{}
	}{
		{10, map[string]struct{}{
			"r10p1": struct{}{},
			"r10p2": struct{}{},
		}},
		{11, map[string]struct{}{
			"r11p1": struct{}{},
			"r11p2": struct{}{},
		}},
		{12, map[string]struct{}{}},
	}

	for _, tt := range tests {
		peers := ps.NotaryPeers(tt.round)

		if len(peers) != len(tt.peers) {
			t.Errorf("notary peers num mismatched, got %d, want %d",
				len(peers), len(tt.peers))
		}

		for _, p := range peers {
			if _, ok := tt.peers[p.id]; !ok {
				t.Errorf("peer %s not in notary peers", p.id)
			}
		}
	}
}

func TestPeerSetSetRound(t *testing.T) {
	ps := newPeerSet()
	notaryPeers := map[uint64]map[string]*peer{
		10: map[string]*peer{
			"r10p1": &peer{id: "r10p1"},
			"r10p2": &peer{id: "r10p2"},
		},
		11: map[string]*peer{
			"r11p1": &peer{id: "r11p1"},
			"r11p2": &peer{id: "r11p2"},
		},
		12: map[string]*peer{
			"r12p1": &peer{id: "r12p1"},
			"r12p2": &peer{id: "r12p2"},
		},
	}

	for _, peers := range notaryPeers {
		for _, p := range peers {
			if err := ps.Register(p); err != nil {
				t.Errorf("register peer fail: %v", err)
			}
		}
	}

	ps.notaryPeers = notaryPeers
	ps.round = 10

	if err := ps.SetRound(9); err != errInvalidRound {
		t.Errorf("got %v, want %v", err, errInvalidRound)
	}

	if err := ps.SetRound(12); err != nil {
		t.Errorf("set round fail: %v", err)
	}

	if len(ps.notaryPeers) != 1 {
		t.Errorf("notary peers not clear correctly, round num got %d, want %d",
			len(ps.notaryPeers), 1)
	}

	if ps.round != 12 {
		t.Errorf("round mismatched: got %d, want %d", ps.round, 12)
	}
}
