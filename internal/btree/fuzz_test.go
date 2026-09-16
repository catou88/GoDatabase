package btree

import "testing"

func FuzzTreeOperations(f *testing.F) {
	f.Add([]byte{
		0, 1, 'a', 3, 'o', 'n', 'e',
		0, 1, 'b', 3, 't', 'w', 'o',
		1, 1, 'a',
		2, 1, 'b',
	})
	f.Add([]byte{
		0, 1, 'c', 5, 't', 'h', 'r', 'e', 'e',
		0, 1, 'c', 7, 'u', 'p', 'd', 'a', 't', 'e', 'd',
		1, 1, 'c',
	})
	f.Add([]byte{
		0, 1, 'a', 1, 'a',
		0, 1, 'b', 1, 'b',
		0, 1, 'c', 1, 'c',
		0, 1, 'd', 1, 'd',
		2, 1, 'a',
		2, 1, 'd',
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		tree := &tree{maxKeys: 3}
		want := make(map[string]string)

		for cursor := 0; cursor < len(data); {
			op := data[cursor] % 3
			cursor++

			key, next, ok := fuzzString(data, cursor)
			if !ok {
				return
			}
			cursor = next

			switch op {
			case 0:
				value, next, ok := fuzzString(data, cursor)
				if !ok {
					return
				}
				cursor = next

				tree.set(key, value)
				want[key] = value
			case 1:
				gotValue, gotOK := tree.get(key)
				wantValue, wantOK := want[key]
				if gotValue != wantValue || gotOK != wantOK {
					t.Fatalf("get(%q) = (%q, %v), want (%q, %v)", key, gotValue, gotOK, wantValue, wantOK)
				}
			case 2:
				got := tree.delete(key)
				_, existed := want[key]
				if got != existed {
					t.Fatalf("delete(%q) = %v, want %v", key, got, existed)
				}
				delete(want, key)
			}

			if err := validateTree(tree); err != nil {
				t.Fatalf("invariant violation after op %d on key %q: %v", op, key, err)
			}
			assertTreeMatchesMap(t, tree, want)
		}
	})
}

func fuzzString(data []byte, cursor int) (string, int, bool) {
	if cursor >= len(data) {
		return "", cursor, false
	}

	length := int(data[cursor]%8) + 1
	cursor++
	if cursor+length > len(data) {
		return "", cursor, false
	}

	buf := make([]byte, length)
	for i := range buf {
		buf[i] = 'a' + data[cursor+i]%26
	}

	return string(buf), cursor + length, true
}

func assertTreeMatchesMap(t *testing.T, tree *tree, want map[string]string) {
	t.Helper()

	got := make(map[string]string)
	collectTreeValues(tree.root, got)

	if len(got) != len(want) {
		t.Fatalf("tree has %d keys, want %d", len(got), len(want))
	}

	for key, wantValue := range want {
		gotValue, ok := got[key]
		if !ok {
			t.Fatalf("expected key %q to exist", key)
		}
		if gotValue != wantValue {
			t.Fatalf("get(%q) = %q, want %q", key, gotValue, wantValue)
		}
	}

	for key := range got {
		if _, ok := want[key]; !ok {
			t.Fatalf("unexpected key %q exists in tree", key)
		}
	}
}

func collectTreeValues(n *node, values map[string]string) {
	if n == nil {
		return
	}

	if n.leaf {
		for i, key := range n.keys {
			values[key] = n.values[i]
		}
		return
	}

	for _, child := range n.children {
		collectTreeValues(child, values)
	}
}
