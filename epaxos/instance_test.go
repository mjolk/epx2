package epaxos

import (
	"sort"
	"testing"

	"reflect"

	pb "github.com/mjolk/epx2/epaxos/epaxospb"
)

func (p *epaxos) assertOutbox(t *testing.T, outbox ...pb.Message) {
	if a, e := p.msgs, outbox; !reflect.DeepEqual(a, e) {
		t.Errorf("\nexpected outbox %+v\n, found %+v\n", e, a)
	}
}

func (p *epaxos) assertOutboxEmpty(t *testing.T) {
	p.assertOutbox(t)
}

var (
	testingCmd        = newTestingCommand("a")
	testingInstanceID = pb.InstanceID{
		ReplicaID:   0,
		InstanceNum: 3,
	}
	testingInstanceData = pb.InstanceData{
		Command: testingCmd,
		SeqNum:  6,
		Deps: []pb.InstanceID{
			{ReplicaID: 0, InstanceNum: 2},
			{ReplicaID: 1, InstanceNum: 2},
			{ReplicaID: 2, InstanceNum: 1},
		},
	}
)

func TestTransitionToPreAccept(t *testing.T) {
	p := newTestingEPaxos()
	p.assertOutboxEmpty(t)

	// Create a new command and transition to PreAccept.
	newInst := p.onRequest(testingCmd)

	// Assert state.
	newInst.assertState(pb.InstanceState_PreAccepted)

	// Assert outbox.
	msg := pb.Message{
		InstanceID: testingInstanceID,
		Type:       pb.WrapMessageInner(&pb.PreAccept{InstanceData: testingInstanceData}),
	}
	p.assertOutbox(t, msg.WithDestination(1), msg.WithDestination(2))
}

func preAcceptMsg() (pb.InstanceID, pb.InstanceData, pb.Message) {
	instMeta := pb.InstanceID{ReplicaID: 1, InstanceNum: 3}
	instData := testingInstanceData
	msg := pb.Message{
		InstanceID: instMeta,
		Type:       pb.WrapMessageInner(&pb.PreAccept{InstanceData: instData}),
	}
	return instMeta, instData, msg
}

// TestOnPreAcceptWithNoNewInfo tests how a replica behaves when it receives
// a PreAccept message and it has no other information to add. It should not
// matter if the local replica has extra commands if they do not interfere.
// It should return a PreAcceptOK message.
func TestOnPreAcceptWithNoNewInfo(t *testing.T) {
	for _, extraCmd := range []bool{false, true} {
		p := newTestingEPaxos()
		p.assertOutboxEmpty(t)

		if extraCmd {
			// Add a command with a larger sequence number. In this scenerio, Replica 1
			// is not aware of this command, which is why it its proposed sequence
			// number did not take this command into account.
			inst03 := p.newInstance(0, 3)
			inst03.is.InstanceData = pb.InstanceData{
				Command: newTestingCommand("zz"),
				SeqNum:  6,
				Deps:    []pb.InstanceID{},
			}
			p.commands[0].ReplaceOrInsert(inst03)
		}

		instMeta, instData, msg := preAcceptMsg()
		p.Step(msg)

		// Verify internal instance state after receiving message.
		maxInst := p.maxInstance(1)
		if a, e := maxInst.is.InstanceNum, instMeta.InstanceNum; a != e {
			t.Errorf("expected new instance with instance num %v, found %v", e, a)
		}
		if a, e := maxInst.is.SeqNum, pb.SeqNum(6); a != e {
			t.Errorf("expected new instance with seq num %v, found %v", e, a)
		}
		if a, e := maxInst.is.Deps, instData.Deps; !reflect.DeepEqual(a, e) {
			t.Errorf("expected new instance with deps %+v, found %+v", e, a)
		}

		// Verify outbox after receiving message.
		reply := pb.Message{
			To:         1,
			InstanceID: instMeta,
			Type:       pb.WrapMessageInner(&pb.PreAcceptOK{}),
		}
		p.assertOutbox(t, reply)
	}
}

// TestOnPreAcceptWithExtraInterferingCommand tests how a replica behaves when it
// receives a PreAccept message and it find that the command should be given additional
// dependencies and a larger sequence number. It should return a PreAcceptReply message
// with the extra dependencies.
func TestOnPreAcceptWithExtraInterferingCommand(t *testing.T) {
	p := newTestingEPaxos()
	p.assertOutboxEmpty(t)

	// Add a command with a larger sequence number. In this scenerio, Replica 1
	// is not aware of this command, which is why it its proposed sequence
	// number did not take this command into account.
	inst03 := p.newInstance(0, 3)
	inst03.is.InstanceData = pb.InstanceData{
		Command: newTestingCommand("a"),
		SeqNum:  6,
		Deps:    []pb.InstanceID{},
	}
	p.commands[0].ReplaceOrInsert(inst03)

	instMeta, instData, msg := preAcceptMsg()
	p.Step(msg)

	// Verify internal instance state after receiving message.
	maxInst := p.maxInstance(1)
	if a, e := maxInst.is.InstanceNum, instMeta.InstanceNum; a != e {
		t.Errorf("expected new instance with instance num %v, found %v", e, a)
	}
	if a, e := maxInst.is.SeqNum, pb.SeqNum(7); a != e {
		t.Errorf("expected new instance with seq num %v, found %v", e, a)
	}

	// The extra command should be part of the deps.
	expDeps := append(instData.Deps, pb.InstanceID{
		ReplicaID:   0,
		InstanceNum: 3,
	})
	sort.Sort(pb.InstanceIDs(expDeps))
	if a, e := maxInst.is.Deps, expDeps; !reflect.DeepEqual(a, e) {
		t.Errorf("expected new instance with deps %+v, found %+v", e, a)
	}

	// Verify outbox after receiving message.
	reply := pb.Message{
		To:         1,
		InstanceID: instMeta,
		Type: pb.WrapMessageInner(&pb.PreAcceptReply{
			UpdatedSeqNum: 7,
			UpdatedDeps:   expDeps,
		}),
	}
	p.assertOutbox(t, reply)
}

func TestOnPreAcceptOK(t *testing.T) {
	p := newTestingEPaxos()

	newInst := p.onRequest(testingCmd)
	p.clearMsgs()

	assertPreAcceptReplies := func(e int) {
		if a := newInst.preAcceptReplies; a != e {
			t.Errorf("expected %d preAcceptReplies, found %d", e, a)
		}
	}
	assertDeps := func(e int) {
		if a := len(newInst.is.Deps); a != e {
			t.Errorf("expected %d dependencies, found %d", e, a)
		}
	}

	// Assert instance state.
	newInst.assertState(pb.InstanceState_PreAccepted)
	assertPreAcceptReplies(0)
	assertDeps(3)

	// Send PreAcceptOK.
	p.Step(pb.Message{
		To:         0,
		InstanceID: testingInstanceID,
		Type:       pb.WrapMessageInner(&pb.PreAcceptOK{}),
	})

	// Assert instance state.
	newInst.assertState(pb.InstanceState_Committed, pb.InstanceState_Executed)
	assertPreAcceptReplies(1)
	assertDeps(3)

	// Assert outbox.
	msg := pb.Message{
		InstanceID: testingInstanceID,
		Type:       pb.WrapMessageInner(&pb.Commit{InstanceData: testingInstanceData}),
	}
	p.assertOutbox(t, msg.WithDestination(1), msg.WithDestination(2))
}

func TestOnPreAcceptReply(t *testing.T) {
	p := newTestingEPaxos()

	newInst := p.onRequest(testingCmd)
	p.clearMsgs()

	assertPreAcceptReplies := func(e int) {
		if a := newInst.preAcceptReplies; a != e {
			t.Errorf("expected %d preAcceptReplies, found %d", e, a)
		}
	}
	assertDeps := func(e int) {
		if a := len(newInst.is.Deps); a != e {
			t.Errorf("expected %d dependencies, found %d", e, a)
		}
	}

	// Assert instance state.
	newInst.assertState(pb.InstanceState_PreAccepted)
	assertPreAcceptReplies(0)
	assertDeps(3)

	// Send PreAcceptOK.
	updatedDeps := append([]pb.InstanceID(nil), testingInstanceData.Deps...)
	updatedDeps = append(updatedDeps, pb.InstanceID{
		ReplicaID:   2,
		InstanceNum: 2,
	})
	p.Step(pb.Message{
		To:         0,
		InstanceID: testingInstanceID,
		Type: pb.WrapMessageInner(&pb.PreAcceptReply{
			UpdatedSeqNum: 7,
			UpdatedDeps:   updatedDeps,
		}),
	})

	// Assert instance state.
	newInst.assertState(pb.InstanceState_Accepted)
	assertPreAcceptReplies(1)
	assertDeps(4)

	// Assert outbox.
	instanceState := testingInstanceData
	instanceState.Command = nil
	instanceState.SeqNum = 7
	instanceState.Deps = updatedDeps
	msg := pb.Message{
		InstanceID: testingInstanceID,
		Type:       pb.WrapMessageInner(&pb.Accept{InstanceData: instanceState}),
	}
	p.assertOutbox(t, msg.WithDestination(1), msg.WithDestination(2))
}
