package hashring

import (
	"fmt"
	"math"
	"testing"
)

const numKeys = 100_000

func keys() []string {
	k := make([]string, numKeys)
	for i := range k {
		k[i] = fmt.Sprintf("user:%d", i)
	}
	return k
}

func TestAddingNodeMovesFewKeys(t *testing.T) {
	ring := New(200)
	mod := Modulo{}
	for i := 1; i <= 4; i++ {
		ring.Add(fmt.Sprintf("node-%d", i))
		mod.Nodes = append(mod.Nodes, fmt.Sprintf("node-%d", i))
	}
	ks := keys()
	beforeRing := make([]string, numKeys)
	beforeMod := make([]string, numKeys)
	for i, k := range ks {
		beforeRing[i], beforeMod[i] = ring.Get(k), mod.Get(k)
	}

	ring.Add("node-5")
	mod.Nodes = append(mod.Nodes, "node-5")

	movedRing, movedMod := 0, 0
	for i, k := range ks {
		if ring.Get(k) != beforeRing[i] {
			movedRing++
		}
		if mod.Get(k) != beforeMod[i] {
			movedMod++
		}
	}
	ringPct := 100 * float64(movedRing) / numKeys
	modPct := 100 * float64(movedMod) / numKeys
	t.Logf("4 → 5 nodes: consistent hashing moved %.1f%% of keys, modulo moved %.1f%%", ringPct, modPct)

	if ringPct > 30 {
		t.Errorf("consistent hashing moved too many keys: %.1f%% (ideal ≈ 20%%)", ringPct)
	}
	if modPct < 60 {
		t.Errorf("expected modulo to move most keys, got %.1f%%", modPct)
	}
}

func TestVirtualNodesBalanceLoad(t *testing.T) {
	for _, vnodes := range []int{1, 10, 100, 500} {
		ring := New(vnodes)
		for i := 1; i <= 5; i++ {
			ring.Add(fmt.Sprintf("node-%d", i))
		}
		counts := map[string]int{}
		for _, k := range keys() {
			counts[ring.Get(k)]++
		}
		ideal := float64(numKeys) / 5
		worst := 0.0
		for _, c := range counts {
			worst = math.Max(worst, math.Abs(float64(c)-ideal)/ideal)
		}
		t.Logf("vnodes=%3d → worst node deviates %.1f%% from ideal", vnodes, worst*100)
		if vnodes == 500 && worst > 0.12 {
			t.Errorf("with 500 vnodes expected <12%% imbalance, got %.1f%%", worst*100)
		}
	}
}

func TestGetNReturnsDistinctReplicas(t *testing.T) {
	ring := New(50)
	for i := 1; i <= 5; i++ {
		ring.Add(fmt.Sprintf("node-%d", i))
	}
	reps := ring.GetN("order:42", 3)
	if len(reps) != 3 {
		t.Fatalf("want 3 replicas, got %v", reps)
	}
	if reps[0] != ring.Get("order:42") {
		t.Fatal("first replica must be the key's owner")
	}
	seen := map[string]bool{}
	for _, r := range reps {
		if seen[r] {
			t.Fatalf("duplicate replica in %v", reps)
		}
		seen[r] = true
	}
}

func TestRemoveNode(t *testing.T) {
	ring := New(100)
	ring.Add("a")
	ring.Add("b")
	ring.Remove("a")
	for _, k := range keys()[:1000] {
		if ring.Get(k) != "b" {
			t.Fatal("all keys must map to the only remaining node")
		}
	}
}
