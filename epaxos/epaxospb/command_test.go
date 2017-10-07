package epaxospb

import (
	"testing"
)

func TestKeyEqual(t *testing.T) {
	a1 := Key("a1")
	a2 := Key("a2")
	if !a1.Equal(a1) {
		t.Errorf("expected keys equal")
	}
	if a1.Equal(a2) {
		t.Errorf("expected different keys not equal")
	}
}

func TestKeyCompare(t *testing.T) {
	testCases := []struct {
		a, b    Key
		compare int
	}{
		{nil, nil, 0},
		{nil, Key("\x00"), -1},
		{Key("\x00"), Key("\x00"), 0},
		{Key(""), Key("\x00"), -1},
		{Key("a"), Key("b"), -1},
		{Key("a\x00"), Key("a"), 1},
		{Key("a\x00"), Key("a\x01"), -1},
	}
	for i, c := range testCases {
		if c.a.Compare(c.b) != c.compare {
			t.Fatalf("%d: unexpected %q.Compare(%q): %d", i, c.a, c.b, c.compare)
		}
	}
}

func TestCommandInterferes(t *testing.T) {
	rA := Command{Writing: false, Key: []byte("a")}
	wA := Command{Writing: true, Key: []byte("a")}
	rD := Command{Writing: false, Key: []byte("b")}
	wD := Command{Writing: true, Key: []byte("b")}

	testData := []struct {
		c1, c2     Command
		interferes bool
	}{
		{rA, rA, false},
		{rA, wA, true},
		{rA, rD, false},
		{rA, wD, false},
		{wA, rA, true},
		{wA, wA, true},
		{wA, rD, false},
		{wA, wD, false},
	}
	for i, test := range testData {
		for _, swap := range []bool{false, true} {
			c1, c2 := test.c1, test.c2
			if swap {
				c1, c2 = c2, c1
			}
			if inter := c1.Interferes(c2); inter != test.interferes {
				t.Errorf("%d: expected interfere %t; got %t between %s vs. %s", i, test.interferes, inter, c1, c2)
			}
		}
	}
}
