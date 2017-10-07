//go:generate protoc -I /usr/local/include -I . -I $GOPATH/src --gogofaster_out=plugins=grpc:. epaxos.proto
package epaxospb

import (
	"bytes"
	"fmt"
)

// Key is an abstract key in a keyspace.
type Key []byte

// Equal returns whether two Keys are identical.
func (k Key) Equal(o Key) bool {
	return bytes.Equal(k, o)
}

// Compare compares the two Keys.
// The result will be 0 if k == k2, -1 if k < k2, and +1 if k > k2.
func (k Key) Compare(k2 Key) int {
	return bytes.Compare(k, k2)
}

// String returns a string-formatted version of the Key.
func (k Key) String() string {
	return fmt.Sprintf("%q", []byte(k))
}

func (c Command) Equal(o Command) bool {
	return c.Key.Equal(o.Key)
}

// Interferes returns whether the two Commands interfere.
func (c Command) Interferes(o Command) bool {
	return (c.Writing || o.Writing) && c.Equal(o)
}

// String returns a string-formatted version of the Command.
func (c Command) String() string {
	prefix := "reading"
	data := ""
	if c.Writing {
		prefix = "writing"
		data = fmt.Sprintf(": %q", c.Data)
	}
	return fmt.Sprintf("{%x %s %s}", c.Key, prefix, data)
}

// InstanceIDs is a slice of InstanceIDs.
type InstanceIDs []InstanceID

// InstanceIDs implements the sort.Interface interface.
func (d InstanceIDs) Len() int      { return len(d) }
func (d InstanceIDs) Swap(i, j int) { d[i], d[j] = d[j], d[i] }
func (d InstanceIDs) Less(i, j int) bool {
	a, b := d[i], d[j]
	if a.ReplicaID != b.ReplicaID {
		return a.ReplicaID < b.ReplicaID
	}
	return a.InstanceNum < b.InstanceNum
}
