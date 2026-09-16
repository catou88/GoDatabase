package btree

import (
	"strings"
	"testing"
)

func TestValidateTreeReportsStructuralCorruption(t *testing.T) {
	tests := []struct {
		name    string
		tree    *tree
		wantErr string
	}{
		{
			name: "unsorted keys",
			tree: &tree{
				maxKeys: 3,
				root: &node{
					leaf:   true,
					keys:   []string{"b", "a"},
					values: []string{"two", "one"},
				},
			},
			wantErr: "keys are not sorted",
		},
		{
			name: "parent child bound violation",
			tree: &tree{
				maxKeys: 3,
				root: &node{
					keys: []string{"m"},
					children: []*node{
						{leaf: true, keys: []string{"z"}, values: []string{"bad"}},
						{leaf: true, keys: []string{"n"}, values: []string{"ok"}},
					},
				},
			},
			wantErr: "at or above upper bound",
		},
		{
			name: "inconsistent leaf depth",
			tree: &tree{
				maxKeys: 3,
				root: &node{
					keys: []string{"m"},
					children: []*node{
						{leaf: true, keys: []string{"a"}, values: []string{"one"}},
						{
							keys: []string{"s"},
							children: []*node{
								{leaf: true, keys: []string{"n"}, values: []string{"two"}},
								{leaf: true, keys: []string{"t"}, values: []string{"three"}},
							},
						},
					},
				},
			},
			wantErr: "leaf depth",
		},
		{
			name: "node under minimum occupancy",
			tree: &tree{
				maxKeys: 4,
				root: &node{
					keys: []string{"m"},
					children: []*node{
						{leaf: true, keys: []string{}, values: []string{}},
						{leaf: true, keys: []string{"n", "o"}, values: []string{"two", "three"}},
					},
				},
			},
			wantErr: "min is",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTree(tt.tree)
			if err == nil {
				t.Fatal("expected invariant violation")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateTree() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}
