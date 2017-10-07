package epaxos

import (
	"reflect"
	"testing"

	pb "github.com/mjolk/epx2/epaxos/epaxospb"
)

// newTestingCommand
func newTestingCommand(key string) *pb.Command {
	return &pb.Command{
		Key:     pb.Key(key),
		Writing: true,
	}
}

func newTestingReadCommand(key string) *pb.Command {
	cmd := newTestingCommand(key)
	cmd.Writing = false
	return cmd
}

func newTestingEPaxos() *epaxos {
	c := Config{ID: 0, Nodes: []pb.ReplicaID{0, 1, 2}}
	p := newEPaxos(&c)

	inst01 := p.newInstance(0, 1)
	inst01.is.InstanceData = pb.InstanceData{
		Command: newTestingCommand("a"),
		SeqNum:  1,
		Deps:    []pb.InstanceID{},
	}

	inst11 := p.newInstance(1, 1)
	inst11.is.InstanceData = pb.InstanceData{
		Command: newTestingCommand("a"),
		SeqNum:  2,
		Deps: []pb.InstanceID{
			pb.InstanceID{ReplicaID: 0, InstanceNum: 1},
		},
	}

	inst21 := p.newInstance(2, 1)
	inst21.is.InstanceData = pb.InstanceData{
		Command: newTestingCommand("a"),
		SeqNum:  3,
		Deps: []pb.InstanceID{
			pb.InstanceID{ReplicaID: 0, InstanceNum: 1},
			pb.InstanceID{ReplicaID: 1, InstanceNum: 1},
		},
	}

	inst02 := p.newInstance(0, 2)
	inst02.is.InstanceData = pb.InstanceData{
		Command: newTestingCommand("a"),
		SeqNum:  4,
		Deps: []pb.InstanceID{
			pb.InstanceID{ReplicaID: 0, InstanceNum: 1},
			pb.InstanceID{ReplicaID: 1, InstanceNum: 1},
			pb.InstanceID{ReplicaID: 2, InstanceNum: 1},
		},
	}

	inst12 := p.newInstance(1, 2)
	inst12.is.InstanceData = pb.InstanceData{
		Command: newTestingCommand("a"),
		SeqNum:  5,
		Deps: []pb.InstanceID{
			pb.InstanceID{ReplicaID: 0, InstanceNum: 1},
			pb.InstanceID{ReplicaID: 1, InstanceNum: 1},
		},
	}

	p.commands[0].ReplaceOrInsert(inst01)
	p.commands[1].ReplaceOrInsert(inst11)
	p.commands[2].ReplaceOrInsert(inst21)
	p.commands[0].ReplaceOrInsert(inst02)
	p.commands[1].ReplaceOrInsert(inst12)

	return p
}

// changeID changes the replica's ID. Used for testing to allow an epaxos
// state machine to act as other replicas.
func (p *epaxos) changeID(t *testing.T, newR pb.ReplicaID) {
	if !p.knownReplica(newR) {
		t.Fatalf("unknown replica %v", newR)
	}
	p.id = newR
}

func TestOnRequestIncrementInstanceNumber(t *testing.T) {
	p := newTestingEPaxos()

	// Verify current max instance numbers.
	expMaxInstanceNums := map[pb.ReplicaID]pb.InstanceNum{
		0: 2,
		1: 2,
		2: 1,
	}
	assertMaxInstanceNums := func() {
		for r, expMaxInst := range expMaxInstanceNums {
			if a, e := p.maxInstanceNum(r), expMaxInst; a != e {
				t.Errorf("expected max instance number %v for replica %v, found %v", e, r, a)
			}
		}
	}
	assertMaxInstanceNums()

	// Crete a new command for replica 0 and verify the new max instance number.
	newCmd := newTestingCommand("a")
	p.onRequest(newCmd)
	expMaxInstanceNums[0] = 3
	assertMaxInstanceNums()

	// Crete a new command for replica 1 and verify the new max instance number.
	p.changeID(t, 1)
	p.onRequest(newCmd)
	expMaxInstanceNums[1] = 3
	assertMaxInstanceNums()

	// Crete a new command for replica 2 and verify the new max instance number.
	p.changeID(t, 2)
	p.onRequest(newCmd)
	expMaxInstanceNums[2] = 2
	assertMaxInstanceNums()
}

func TestOnRequestIncrementSequenceNumber(t *testing.T) {
	p := newTestingEPaxos()

	// Verify current max sequence numbers.
	expMaxSeqNums := map[pb.ReplicaID]pb.SeqNum{
		0: 4,
		1: 5,
		2: 3,
	}
	assertMaxSeqNums := func() {
		for r, expMaxSeq := range expMaxSeqNums {
			if a, e := p.maxSeqNum(r), expMaxSeq; a != e {
				t.Errorf("expected max seq number %v for replica %v, found %v", e, r, a)
			}
		}
	}
	assertMaxSeqNums()

	// Crete a new command for replica 0 and verify the new max seq number.
	newCmd := newTestingCommand("a")
	p.onRequest(newCmd)
	expMaxSeqNums[0] = 6
	assertMaxSeqNums()

	// Crete a new command for replica 1 and verify the new max seq number.
	p.changeID(t, 1)
	p.onRequest(newCmd)
	expMaxSeqNums[1] = 7
	assertMaxSeqNums()

	// Crete a new command for replica 2 and verify the new max seq number.
	p.changeID(t, 2)
	p.onRequest(newCmd)
	expMaxSeqNums[2] = 8
	assertMaxSeqNums()
}

func TestOnRequestDependencies(t *testing.T) {
	p := newTestingEPaxos()

	// Verify current max dependencies numbers.
	expMaxDeps := map[pb.ReplicaID][]pb.InstanceID{
		0: {
			pb.InstanceID{ReplicaID: 0, InstanceNum: 1},
			pb.InstanceID{ReplicaID: 1, InstanceNum: 1},
			pb.InstanceID{ReplicaID: 2, InstanceNum: 1},
		},
		1: {
			pb.InstanceID{ReplicaID: 0, InstanceNum: 1},
			pb.InstanceID{ReplicaID: 1, InstanceNum: 1},
		},
		2: {
			pb.InstanceID{ReplicaID: 0, InstanceNum: 1},
			pb.InstanceID{ReplicaID: 1, InstanceNum: 1},
		},
	}
	assertMaxDeps := func(run string) {
		for r, expDeps := range expMaxDeps {
			if a, e := p.maxDeps(r), expDeps; !reflect.DeepEqual(a, e) {
				t.Errorf(
					"\n\nrun: %s\n expected max deps %+v for replica %v,\n found %+v",
					run,
					e,
					r,
					a,
				)
			}
		}
	}

	assertMaxDeps("current")

	// Crete a new command for replica 0 and verify the new max deps.
	newCmd := newTestingCommand("a")
	p.onRequest(newCmd)
	expMaxDeps[0] = []pb.InstanceID{
		pb.InstanceID{ReplicaID: 0, InstanceNum: 2},
		pb.InstanceID{ReplicaID: 1, InstanceNum: 2},
		pb.InstanceID{ReplicaID: 2, InstanceNum: 1},
	}
	assertMaxDeps("added a")

	// Crete a new command for replica 1 and verify the new max deps.
	newCmd.Key = pb.Key("c")
	p.changeID(t, 1)
	p.onRequest(newCmd)
	expMaxDeps[1] = []pb.InstanceID{
		pb.InstanceID{ReplicaID: 0, InstanceNum: 3},
	}
	assertMaxDeps("added c")

	// Crete a new command for replica 2 and verify the new max deps.
	newCmd.Key = pb.Key("a")
	p.changeID(t, 2)
	p.onRequest(newCmd)
	expMaxDeps[2] = []pb.InstanceID{
		pb.InstanceID{ReplicaID: 0, InstanceNum: 3},
		pb.InstanceID{ReplicaID: 1, InstanceNum: 3},
		pb.InstanceID{ReplicaID: 2, InstanceNum: 1},
	}
	assertMaxDeps("added a")
}
