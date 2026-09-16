package btree

import "testing"

func TestTreeGetEmptyTree(t *testing.T) {
	tree := &tree{}

	value, ok := tree.get("alpha")
	if ok {
		t.Fatalf("expected missing key in empty tree, got %q", value)
	}
}

func TestTreeGetSingleLeaf(t *testing.T) {
	tree := &tree{
		root: &node{
			leaf:   true,
			keys:   []string{"alpha", "beta", "gamma"},
			values: []string{"one", "two", "three"},
		},
	}

	tests := []struct {
		name      string
		key       string
		wantValue string
		wantOK    bool
	}{
		{
			name:      "first key",
			key:       "alpha",
			wantValue: "one",
			wantOK:    true,
		},
		{
			name:      "middle key",
			key:       "beta",
			wantValue: "two",
			wantOK:    true,
		},
		{
			name:      "last key",
			key:       "gamma",
			wantValue: "three",
			wantOK:    true,
		},
		{
			name:      "missing key",
			key:       "delta",
			wantValue: "",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValue, gotOK := tree.get(tt.key)
			if gotValue != tt.wantValue || gotOK != tt.wantOK {
				t.Fatalf("get(%q) = (%q, %v), want (%q, %v)", tt.key, gotValue, gotOK, tt.wantValue, tt.wantOK)
			}
		})
	}
}

func TestTreeGetMultiLevelTree(t *testing.T) {
	left := &node{
		leaf:   true,
		keys:   []string{"alpha", "bravo"},
		values: []string{"one", "two"},
	}
	middle := &node{
		leaf:   true,
		keys:   []string{"charlie", "delta"},
		values: []string{"three", "four"},
	}
	right := &node{
		leaf:   true,
		keys:   []string{"echo", "foxtrot"},
		values: []string{"five", "six"},
	}
	tree := &tree{
		root: &node{
			keys:     []string{"charlie", "echo"},
			children: []*node{left, middle, right},
		},
	}

	tests := []struct {
		name      string
		key       string
		wantValue string
		wantOK    bool
	}{
		{
			name:      "left child",
			key:       "alpha",
			wantValue: "one",
			wantOK:    true,
		},
		{
			name:      "middle child at separator",
			key:       "charlie",
			wantValue: "three",
			wantOK:    true,
		},
		{
			name:      "right child at separator",
			key:       "echo",
			wantValue: "five",
			wantOK:    true,
		},
		{
			name:      "missing key reaches leaf",
			key:       "golf",
			wantValue: "",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValue, gotOK := tree.get(tt.key)
			if gotValue != tt.wantValue || gotOK != tt.wantOK {
				t.Fatalf("get(%q) = (%q, %v), want (%q, %v)", tt.key, gotValue, gotOK, tt.wantValue, tt.wantOK)
			}
		})
	}
}
