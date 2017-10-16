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

// Equal compares Spans for equality.
func (s Span) Equal(o Span) bool {
	return s.Key.Equal(o.Key) && s.EndKey.Equal(o.EndKey)
}

// Overlaps returns whether the two Spans overlap.
func (s Span) Overlaps(o Span) bool {
	if len(s.EndKey) == 0 && len(o.EndKey) == 0 {
		return s.Key.Equal(o.Key)
	} else if len(s.EndKey) == 0 {
		return bytes.Compare(s.Key, o.Key) >= 0 && bytes.Compare(s.Key, o.EndKey) < 0
	} else if len(o.EndKey) == 0 {
		return bytes.Compare(o.Key, s.Key) >= 0 && bytes.Compare(o.Key, s.EndKey) < 0
	}
	return bytes.Compare(s.EndKey, o.Key) > 0 && bytes.Compare(s.Key, o.EndKey) < 0
}

// String returns a string-formatted version of the Span.
func (s Span) String() string {
	if s.EndKey == nil {
		return fmt.Sprintf("[%s]", s.Key)
	}
	return fmt.Sprintf("[%s-%s)", s.Key, s.EndKey)
}

// Interferes returns whether the two Commands interfere.
func (c Command) Interferes(o Command) bool {
	return (c.Writing || o.Writing) && c.Span.Overlaps(o.Span)
}

// String returns a string-formatted version of the Command.
func (c Command) String() string {
	prefix := "reading"
	data := ""
	if c.Writing {
		prefix = "writing"
		data = fmt.Sprintf(": %q", c.Data)
	}
	return fmt.Sprintf("{%d %s %s%s}", c.ID, prefix, c.Span, data)
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
