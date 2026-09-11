package db

import "testing"

func TestDatabaseSetAndGet(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		value     string
		wantValue string
	}{
		{
			name:      "stores value for key",
			key:       "alpha",
			value:     "one",
			wantValue: "one",
		},
		{
			name:      "stores empty value",
			key:       "empty",
			value:     "",
			wantValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := New()

			if err := db.Set(tt.key, tt.value); err != nil {
				t.Fatalf("Set returned error: %v", err)
			}

			value, ok := db.Get(tt.key)
			if !ok {
				t.Fatalf("expected %q to exist", tt.key)
			}
			if value != tt.wantValue {
				t.Fatalf("expected %q to be %q, got %q", tt.key, tt.wantValue, value)
			}
		})
	}
}

func TestDatabaseOverwrite(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		first     string
		second    string
		wantValue string
	}{
		{
			name:      "replaces existing value",
			key:       "alpha",
			first:     "one",
			second:    "two",
			wantValue: "two",
		},
		{
			name:      "replaces value with empty value",
			key:       "alpha",
			first:     "one",
			second:    "",
			wantValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := New()

			if err := db.Set(tt.key, tt.first); err != nil {
				t.Fatalf("first Set returned error: %v", err)
			}
			if err := db.Set(tt.key, tt.second); err != nil {
				t.Fatalf("overwrite Set returned error: %v", err)
			}

			value, ok := db.Get(tt.key)
			if !ok {
				t.Fatalf("expected %q to exist after overwrite", tt.key)
			}
			if value != tt.wantValue {
				t.Fatalf("expected %q to be %q, got %q", tt.key, tt.wantValue, value)
			}
		})
	}
}

func TestDatabaseDelete(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		seed       map[string]string
		wantDelete bool
		wantExists bool
	}{
		{
			name:       "deletes existing key",
			key:        "alpha",
			seed:       map[string]string{"alpha": "one"},
			wantDelete: true,
			wantExists: false,
		},
		{
			name:       "returns false for missing key",
			key:        "missing",
			seed:       map[string]string{"alpha": "one"},
			wantDelete: false,
			wantExists: false,
		},
		{
			name:       "returns false for missing key in empty database",
			key:        "missing",
			seed:       nil,
			wantDelete: false,
			wantExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := New()
			for key, value := range tt.seed {
				if err := db.Set(key, value); err != nil {
					t.Fatalf("Set returned error: %v", err)
				}
			}

			deleted := db.Delete(tt.key)
			if deleted != tt.wantDelete {
				t.Fatalf("expected Delete(%q) to return %v, got %v", tt.key, tt.wantDelete, deleted)
			}

			_, ok := db.Get(tt.key)
			if ok != tt.wantExists {
				t.Fatalf("expected Get(%q) existence to be %v, got %v", tt.key, tt.wantExists, ok)
			}
		})
	}
}

func TestDatabaseGetMissingKey(t *testing.T) {
	db := New()

	if err := db.Set("alpha", "one"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	value, ok := db.Get("missing")
	if ok {
		t.Fatalf("expected missing key to not exist, got value %q", value)
	}
}

func TestDatabaseRange(t *testing.T) {
	db := New()
	for _, pair := range []struct{ key, value string }{
		{"b", "two"},
		{"a", "one"},
		{"c", "three"},
	} {
		if err := db.Set(pair.key, pair.value); err != nil {
			t.Fatalf("Set returned error: %v", err)
		}
	}

	values := db.Range("a", "c")
	if len(values) != 3 {
		t.Fatalf("expected 3 values in range, got %d", len(values))
	}

	if values[0].Key != "a" || values[0].Value != "one" {
		t.Fatalf("first range item mismatch: %#v", values[0])
	}
	if values[1].Key != "b" || values[1].Value != "two" {
		t.Fatalf("second range item mismatch: %#v", values[1])
	}
	if values[2].Key != "c" || values[2].Value != "three" {
		t.Fatalf("third range item mismatch: %#v", values[2])
	}
}
