package epaxos

import (
	"math/rand"
	"reflect"
	"time"

	"github.com/cockroachdb/cockroach/pkg/util/interval"
	"github.com/google/btree"
	"github.com/pkg/errors"

	pb "github.com/mjolk/epx2/epaxos/epaxospb"
)

// Config contains the parameters to start epaxos.
type Config struct {
	// ID is the identity of the local epaxos.
	ID pb.ReplicaID
	// Nodes is the set of all nodes in the epaxos network.
	Nodes []pb.ReplicaID
	// Storage is the persistent storage for epaxos. epaxos reads out
	// the previous instance state and configuration from storage when
	// restarting.
	Storage Storage
	// Logger is the logger that the epaxos state machine will use
	// to log events. If not set, a default logger will be used.
	Logger Logger
	// RandSeed allows the seed used by epaxos's rand.Source to be
	// injected, to allow for fully deterministic execution.
	RandSeed int64
}

func (c *Config) validate() error {
	if !inReplicaSlice(c.ID, c.Nodes) {
		return errors.Errorf("ID not in Nodes slice")
	}
	if c.Storage == nil {
		c.Storage = NewMemoryStorage(c)
	} else if hs, ok := c.Storage.HardState(); ok {
		if hs.ReplicaID != c.ID {
			return errors.Errorf("ID different than in HardState")
		}
		if !reflect.DeepEqual(hs.Nodes, c.Nodes) {
			return errors.Errorf("Node set different than in HardState")
		}
	}
	if c.Logger == nil {
		c.Logger = NewDefaultLogger()
	}
	if c.RandSeed == 0 {
		c.RandSeed = time.Now().UnixNano()
	}
	return nil
}

// epaxos is a state machine that implements the EPaxos replication protocol.
//
// epaxos is not safe to use from multiple goroutines.
type epaxos struct {
	// id is a unique identifier for this node.
	id pb.ReplicaID
	// nodes is the set of all nodes in the EPaxos network.
	nodes []pb.ReplicaID
	// storage is a handle to the node's persistent storage.
	storage Storage

	// commands is a map from replica to an ordered tree of instance, indexed by
	// sequence number. BTree contains *instance elements.
	commands map[pb.ReplicaID]*btree.BTree
	// TODO reintroduce instance space truncation.
	// maxTruncatedInstanceNum is a mapping from replica to the maximum instance
	// number that has been truncated up to in its command space.
	// maxTruncatedInstanceNum map[pb.ReplicaID]pb.InstanceNum
	// maxTruncatedSeqNum is the maximum sequence number that has been truncated.
	// maxTruncatedSeqNum pb.SeqNum
	// rangeGroup is used to minimize dependency lists by tracking transitive
	// dependencies.
	rangeGroup interval.RangeGroup

	// executor holds execution state and handles the execution of committed
	// instances.
	executor executor
	// timers holds all current timers, which are each incremented on every call
	// to Tick.
	timers map[*tickingTimer]struct{}

	// msgs is the outbox for the paxos node, containing all messages that need
	// to be delivered.
	msgs []pb.Message
	// // committedCmds is the outbox for commands that have been committed and
	// // can be acknowledged to clients. The commands have not necessarily been
	// // executed yet, though, so they should not be run on the state machine.
	// committedCmds []pb.Command
	// executedCmds is the outbox for commands that are ready to be executed,
	// in-order.
	executedCmds []pb.Command

	// logger is used by paxos to log event.
	logger Logger
	// rand holds the paxos instance's local Rand object. This allows us to avoid
	// using the synchronized global Rand object.
	rand *rand.Rand
}

func newEPaxos(c *Config) *epaxos {
	if err := c.validate(); err != nil {
		panic(err.Error())
	}
	p := &epaxos{
		id:         c.ID,
		nodes:      c.Nodes,
		logger:     c.Logger,
		commands:   make(map[pb.ReplicaID]*btree.BTree, len(c.Nodes)),
		rangeGroup: interval.NewRangeTree(),
		timers:     make(map[*tickingTimer]struct{}),
		rand:       rand.New(rand.NewSource(c.RandSeed)),
	}
	p.executor = makeExecutor(p)
	for _, rep := range c.Nodes {
		p.commands[rep] = btree.New(32 /* degree */)
	}
	p.initStorage(c)
	p.initTimers()
	return p
}

func (p *epaxos) initStorage(c *Config) {
	s := c.Storage
	p.storage = s

	// Set up the node's HardState.
	if _, ok := s.HardState(); !ok {
		s.PersistHardState(pb.HardState{
			ReplicaID: c.ID,
			Nodes:     c.Nodes,
		})
	}

	// Load all persisted instances.
	insts := s.Instances()
	for _, is := range insts {
		inst := p.newInstanceFromState(is)
		p.commands[is.ReplicaID].ReplaceOrInsert(inst)
		cmdLeader := is.ReplicaID == p.id
		if cmdLeader && !inst.isStates(pb.InstanceState_Executed) {
			inst.restartTransition()
		}
	}
}

// initTimers initializes all static timers for the epaxos state machine.
// TODO allow injecting timeouts.
func (p *epaxos) initTimers() {
	// // The executorTimer runs the executor, attempting to execute any
	// // committed instances.
	// const executorTimeout = 10
	// executorTimer := makeTickingTimer(executorTimeout, func() {
	// 	p.executor.run()
	// })
	// p.registerInfiniteTimer(&executorTimer)
}

func (p *epaxos) Tick() {
	for t := range p.timers {
		t.tick()
	}
}

func (p *epaxos) registerInfiniteTimer(t *tickingTimer) {
	p.timers[t] = struct{}{}
	t.instrument(func() {
		t.reset()
	})
}

func (p *epaxos) registerOneTimeTimer(t *tickingTimer) {
	p.timers[t] = struct{}{}
	t.instrument(func() {
		p.unregisterTimer(t)
	})
	t.reset()
}

func (p *epaxos) unregisterTimer(t *tickingTimer) {
	t.stop()
	delete(p.timers, t)
}

func (p *epaxos) Request(cmd *pb.Command) {
	p.onRequest(cmd)
}

func (p *epaxos) Step(m pb.Message) {
	if ok := p.validateMessage(m); !ok {
		p.logger.Warningf("found invalid Message: %+v", m)
		return
	}

	r := m.InstanceID.ReplicaID
	i := m.InstanceID.InstanceNum
	inst := p.getInstance(r, i)
	if inst == nil {
		// if p.hasTruncated(r, i) {
		// 	// We've already truncated this instance, which means that it was
		// 	// already committed. Ignore the messsage.
		// 	p.logger.Debugf("ignoring message to truncated instance: %+v", m)
		// 	return
		// }
		if r == p.id {
			// We should always know about our own instances.
			p.logger.Warningf("unknown local instance number: %+v", m)
			return
		}

		// Create a new instance if one does not already exist.
		inst = p.newInstance(r, i)
		p.commands[r].ReplaceOrInsert(inst)
	}

	switch t := m.Type.(type) {
	case *pb.Message_PreAccept:
		inst.onPreAccept(t.PreAccept)
	case *pb.Message_PreAcceptOk:
		inst.onPreAcceptOK(t.PreAcceptOk)
	case *pb.Message_PreAcceptReply:
		inst.onPreAcceptReply(t.PreAcceptReply)
	case *pb.Message_Accept:
		inst.onAccept(t.Accept)
	case *pb.Message_AcceptOk:
		inst.onAcceptOK(t.AcceptOk)
	case *pb.Message_Commit:
		inst.onCommit(t.Commit)
	default:
		p.logger.Panicf("unexpected Message type: %T", t)
	}
}

func (p *epaxos) validateMessage(m pb.Message) bool {
	// The message should have us as its destination.
	if m.To != p.id {
		return false
	}

	if pb.IsReply(m.Type) {
		// The instance's replica should be us.
		if m.InstanceID.ReplicaID != p.id {
			return false
		}
	} else {
		// The instance's replica should be a node that we're aware of, but not us.
		if m.InstanceID.ReplicaID == p.id {
			return false
		}
		if !p.knownReplica(m.InstanceID.ReplicaID) {
			return false
		}
	}

	// TODO ballot stuff
	return true
}

// func (p *epaxos) deliverCommittedCommand(cmd pb.Command) {
// 	p.committedCmds = append(p.committedCmds, cmd)
// }

// func (p *epaxos) clearCommittedCommands() {
// 	p.committedCmds = nil
// }

func (p *epaxos) deliverExecutedCommand(cmd pb.Command) {
	p.executedCmds = append(p.executedCmds, cmd)
}

func (p *epaxos) clearExecutedCommands() {
	p.executedCmds = nil
}

func (p *epaxos) knownReplica(r pb.ReplicaID) bool {
	return inReplicaSlice(r, p.nodes)
}

func inReplicaSlice(r pb.ReplicaID, s []pb.ReplicaID) bool {
	for _, n := range s {
		if n == r {
			return true
		}
	}
	return false
}

func (p *epaxos) F() int {
	// N = 2F+1
	return (len(p.nodes)+1)/2 - 1
}

func (p *epaxos) quorum(val int) bool {
	// floor(N/2)+1
	return val > len(p.nodes)/2
}

func (p *epaxos) fastQuorum(val int) bool {
	return val >= len(p.nodes)-1
	// // (x+y-1)/y == ceil(x/y)
	// return val > (3*len(p.nodes)+(4-1))/4
}
