package btree

import "testing"

func TestNodeSearch(t *testing.T) {
	tests := []struct {
		name      string
		keys      []string
		key       string
		wantIndex int
		wantFound bool
	}{
		{
			name:      "empty node",
			keys:      nil,
			key:       "a",
			wantIndex: 0,
			wantFound: false,
		},
		{
			name:      "exact match first key",
			keys:      []string{"a", "c", "e"},
			key:       "a",
			wantIndex: 0,
			wantFound: true,
		},
		{
			name:      "exact match middle key",
			keys:      []string{"a", "c", "e"},
			key:       "c",
			wantIndex: 1,
			wantFound: true,
		},
		{
			name:      "exact match last key",
			keys:      []string{"a", "c", "e"},
			key:       "e",
			wantIndex: 2,
			wantFound: true,
		},
		{
			name:      "missing before first key",
			keys:      []string{"b", "d", "f"},
			key:       "a",
			wantIndex: 0,
			wantFound: false,
		},
		{
			name:      "missing between keys",
			keys:      []string{"b", "d", "f"},
			key:       "c",
			wantIndex: 1,
			wantFound: false,
		},
		{
			name:      "missing after last key",
			keys:      []string{"b", "d", "f"},
			key:       "z",
			wantIndex: 3,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &node{keys: tt.keys}

			gotIndex, gotFound := n.search(tt.key)
			if gotIndex != tt.wantIndex || gotFound != tt.wantFound {
				t.Fatalf("search(%q) = (%d, %v), want (%d, %v)", tt.key, gotIndex, gotFound, tt.wantIndex, tt.wantFound)
			}
		})
	}
}

func TestNodeInsertPosition(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		key  string
		want int
	}{
		{
			name: "empty node",
			keys: nil,
			key:  "a",
			want: 0,
		},
		{
			name: "before first key",
			keys: []string{"b", "d", "f"},
			key:  "a",
			want: 0,
		},
		{
			name: "at existing key",
			keys: []string{"b", "d", "f"},
			key:  "d",
			want: 1,
		},
		{
			name: "between keys",
			keys: []string{"b", "d", "f"},
			key:  "e",
			want: 2,
		},
		{
			name: "after last key",
			keys: []string{"b", "d", "f"},
			key:  "z",
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &node{keys: tt.keys}

			got := n.insertPosition(tt.key)
			if got != tt.want {
				t.Fatalf("insertPosition(%q) = %d, want %d", tt.key, got, tt.want)
			}
		})
	}
}

func TestNodeChildIndex(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		key  string
		want int
	}{
		{
			name: "before first separator",
			keys: []string{"c", "f", "k"},
			key:  "a",
			want: 0,
		},
		{
			name: "equal to first separator selects right child",
			keys: []string{"c", "f", "k"},
			key:  "c",
			want: 1,
		},
		{
			name: "between separators",
			keys: []string{"c", "f", "k"},
			key:  "g",
			want: 2,
		},
		{
			name: "equal to last separator selects right child",
			keys: []string{"c", "f", "k"},
			key:  "k",
			want: 3,
		},
		{
			name: "after last separator",
			keys: []string{"c", "f", "k"},
			key:  "z",
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &node{keys: tt.keys}

			got := n.childIndex(tt.key)
			if got != tt.want {
				t.Fatalf("childIndex(%q) = %d, want %d", tt.key, got, tt.want)
			}
		})
	}
}

func TestNodeChild(t *testing.T) {
	left := &node{leaf: true, keys: []string{"a"}}
	middle := &node{leaf: true, keys: []string{"d"}}
	right := &node{leaf: true, keys: []string{"g"}}
	parent := &node{
		keys:     []string{"c", "f"},
		children: []*node{left, middle, right},
	}

	if got := parent.child("b"); got != left {
		t.Fatalf("child before first separator = %p, want %p", got, left)
	}
	if got := parent.child("c"); got != middle {
		t.Fatalf("child at first separator = %p, want %p", got, middle)
	}
	if got := parent.child("z"); got != right {
		t.Fatalf("child after last separator = %p, want %p", got, right)
	}
}

func TestNodeValueAt(t *testing.T) {
	n := &node{
		leaf:   true,
		keys:   []string{"a", "b"},
		values: []string{"one", "two"},
	}

	if got := n.valueAt(1); got != "two" {
		t.Fatalf("valueAt(1) = %q, want %q", got, "two")
	}
}

func TestNodeNextLeaf(t *testing.T) {
	right := &node{leaf: true, keys: []string{"c"}}
	left := &node{
		leaf: true,
		keys: []string{"a"},
		next: right,
	}

	if got := left.nextLeaf(); got != right {
		t.Fatalf("nextLeaf() = %p, want %p", got, right)
	}
}
