package epaxospb

import (
	"github.com/petar/GoLLRB/llrb"
)

// Less implements the btree.Item interface.
func (ihs *InstanceState) Less(than llrb.Item) bool {
	return ihs.InstanceNum < than.(*InstanceState).InstanceNum
}
