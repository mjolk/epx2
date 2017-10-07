package epaxos

import (
	"github.com/petar/GoLLRB/llrb"

	pb "github.com/mjolk/epx2/epaxos/epaxospb"
)

func (p *epaxos) maxInstance(r pb.ReplicaID) *instance {
	if maxInstItem := p.commands[r].Max(); maxInstItem != nil {
		return maxInstItem.(*instance)
	}
	return nil
}

func (p *epaxos) maxInstanceNum(r pb.ReplicaID) pb.InstanceNum {
	if maxInst := p.maxInstance(r); maxInst != nil {
		return maxInst.is.InstanceNum
	}
	return 0
}

func (p *epaxos) maxSeqNum(r pb.ReplicaID) pb.SeqNum {
	if maxInst := p.maxInstance(r); maxInst != nil {
		return maxInst.is.SeqNum
	}
	return 0
}

func (p *epaxos) maxDeps(r pb.ReplicaID) []pb.InstanceID {
	if maxInst := p.maxInstance(r); maxInst != nil {
		return maxInst.is.Deps
	}
	return nil
}

func (p *epaxos) getInstance(r pb.ReplicaID, i pb.InstanceNum) *instance {
	if instItem := p.commands[r].Get(instanceKey(i)); instItem != nil {
		return instItem.(*instance)
	}
	return nil
}

func (p *epaxos) hasAccepted(r pb.ReplicaID, i pb.InstanceNum) bool {
	if inst := p.getInstance(r, i); inst != nil {
		return inst.is.Status >= pb.InstanceState_Accepted
	}
	return false
}

func (p *epaxos) hasExecuted(r pb.ReplicaID, i pb.InstanceNum) bool {
	if inst := p.getInstance(r, i); inst != nil {
		return inst.is.Status == pb.InstanceState_Executed
	}
	return false
}

// HasExecuted implements the history interface.
func (p *epaxos) HasExecuted(e executableID) bool {
	d := e.(pb.InstanceID)
	return p.hasExecuted(d.ReplicaID, d.InstanceNum)
}

// seqAndDepsForCommand determines the locally known maximum interfering sequence
// number and dependencies for a given command.
func (p *epaxos) seqAndDepsForCommand(
	cmd *pb.Command, ignoredInstance pb.InstanceID,
) (pb.SeqNum, map[pb.InstanceID]struct{}) {
	var maxSeq pb.SeqNum
	deps := make(map[pb.InstanceID]struct{})

	for rID, cmds := range p.commands {
		cmds.DescendLessOrEqual(cmds.Max(), func(i llrb.Item) bool {
			inst := i.(*instance)
			if inst.is.InstanceID == ignoredInstance {
				return true
			}

			if otherCmd := inst.is.Command; otherCmd.Interferes(*cmd) {
				maxSeq = pb.MaxSeqNum(maxSeq, inst.is.SeqNum)

				dep := pb.InstanceID{
					ReplicaID:   rID,
					InstanceNum: inst.is.InstanceNum,
				}
				deps[dep] = struct{}{}
				return false
			}
			return true
		})
	}
	return maxSeq, deps
}

func (p *epaxos) onRequest(cmd *pb.Command) *instance {
	// Determine the smallest unused instance number.
	i := p.maxInstanceNum(p.id) + 1

	// Add a new instance for the command in the local commands.
	maxLocalSeq, localDeps := p.seqAndDepsForCommand(cmd, pb.InstanceID{})
	newInst := p.newInstance(p.id, i)
	newInst.is.Command = cmd
	newInst.is.SeqNum = maxLocalSeq + 1
	newInst.is.Deps = depSliceFromMap(localDeps)
	p.commands[p.id].ReplaceOrInsert(newInst)

	// Transition the new instance into a preAccepted state.
	newInst.transitionTo(pb.InstanceState_PreAccepted)
	return newInst
}

func (p *epaxos) prepareToExecute(inst *instance) {
	inst.assertState(pb.InstanceState_Committed)
	p.executor.addExec(inst)
	// TODO pull executor into a different goroutine and run asynchronously.
	p.executor.run()
	// p.truncateCommands()
}

// TODO reintroduce instance space truncation.
// func (p *epaxos) truncateCommands() {
// 	for r, cmds := range p.commands {
// 		var executedItems []btree.Item
// 		cmds.Ascend(func(i btree.Item) bool {
// 			if i.(*instance).is.Status == pb.InstanceState_Executed {
// 				executedItems = append(executedItems, i)
// 				return true
// 			}
// 			return false
// 		})
// 		if len(executedItems) > 0 {
// 			curMaxInstNum := p.maxTruncatedInstanceNum[r]
// 			for _, executedItem := range executedItems {
// 				inst := executedItem.(*instance)
// 				p.maxTruncatedSeqNum = pb.MaxSeqNum(p.maxTruncatedSeqNum, inst.is.SeqNum)
// 				curMaxInstNum = pb.MaxInstanceNum(curMaxInstNum, inst.is.InstanceNum)
// 				cmds.Delete(executedItem)
// 			}
// 			p.maxTruncatedInstanceNum[r] = curMaxInstNum
// 		}
// 	}
// }
