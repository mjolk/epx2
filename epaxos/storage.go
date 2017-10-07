package epaxos

import (
	"github.com/petar/GoLLRB/llrb"

	pb "github.com/mjolk/epx2/epaxos/epaxospb"
)

// Storage allows for the persistence of EPaxos state to provide durability.
type Storage interface {
	HardState() (pb.HardState, bool)
	PersistHardState(hs pb.HardState)

	Instances() []*pb.InstanceState
	PersistInstance(is *pb.InstanceState)
}

var _ Storage = &MemoryStorage{}

// MemoryStorage implements the Storage interface backed by an in-memory
// data structure.
type MemoryStorage struct {
	hardState struct {
		set bool
		hs  pb.HardState
	}
	instances map[pb.ReplicaID]*llrb.LLRB // *pb.InstanceState Items
}

// NewMemoryStorage returns a new in-memory implementation of Storage using
// the provided Config.
func NewMemoryStorage(c *Config) Storage {
	s := &MemoryStorage{
		instances: make(map[pb.ReplicaID]*llrb.LLRB, len(c.Nodes)),
	}
	for _, rep := range c.Nodes {
		s.instances[rep] = llrb.New()
	}
	return s
}

// HardState implements the Storage interface.
func (ms *MemoryStorage) HardState() (pb.HardState, bool) {
	if ms.hardState.set {
		return ms.hardState.hs, true
	}
	return pb.HardState{}, false
}

// PersistHardState implements the Storage interface.
func (ms *MemoryStorage) PersistHardState(hs pb.HardState) {
	ms.hardState.hs = hs
	ms.hardState.set = true
}

func instanceStateKey(i pb.InstanceNum) llrb.Item {
	return &pb.InstanceState{InstanceID: pb.InstanceID{InstanceNum: i}}
}

// Instances implements the Storage interface.
func (ms *MemoryStorage) Instances() []*pb.InstanceState {
	var insts []*pb.InstanceState
	for _, replInsts := range ms.instances {
		replInsts.AscendGreaterOrEqual(replInsts.Min(), func(i llrb.Item) bool {
			insts = append(insts, i.(*pb.InstanceState))
			return true
		})
	}
	return insts
}

// PersistInstance implements the Storage interface.
func (ms *MemoryStorage) PersistInstance(is *pb.InstanceState) {
	ms.instances[is.ReplicaID].ReplaceOrInsert(is)
}
