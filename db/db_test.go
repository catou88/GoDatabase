package db

import (
	"errors"
	"reflect"
	"testing"
)

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

func TestDatabaseSetRejectsEmptyKey(t *testing.T) {
	db := New()

	if err := db.Set("", "value"); !errors.Is(err, ErrEmptyKey) {
		t.Fatalf("expected ErrEmptyKey, got %v", err)
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
	tests := []struct {
		name  string
		seed  map[string]string
		start string
		end   string
		want  []Item
	}{
		{
			name:  "empty database returns no items",
			seed:  nil,
			start: "a",
			end:   "z",
			want:  []Item{},
		},
		{
			name:  "no matching keys returns no items",
			seed:  map[string]string{"a": "one", "b": "two", "c": "three"},
			start: "x",
			end:   "z",
			want:  []Item{},
		},
		{
			name:  "start equals end returns matching key",
			seed:  map[string]string{"a": "one", "b": "two", "c": "three"},
			start: "b",
			end:   "b",
			want: []Item{
				{Key: "b", Value: "two"},
			},
		},
		{
			name:  "start greater than end returns no items",
			seed:  map[string]string{"a": "one", "b": "two", "c": "three"},
			start: "c",
			end:   "a",
			want:  []Item{},
		},
		{
			name:  "excludes keys outside bounds",
			seed:  map[string]string{"a": "one", "b": "two", "c": "three", "d": "four"},
			start: "b",
			end:   "c",
			want: []Item{
				{Key: "b", Value: "two"},
				{Key: "c", Value: "three"},
			},
		},
		{
			name:  "returns keys sorted ascending",
			seed:  map[string]string{"c": "three", "a": "one", "b": "two"},
			start: "a",
			end:   "c",
			want: []Item{
				{Key: "a", Value: "one"},
				{Key: "b", Value: "two"},
				{Key: "c", Value: "three"},
			},
		},
		{
			name:  "empty start follows string ordering",
			seed:  map[string]string{"a": "one", "b": "two", "c": "three", "d": "four"},
			start: "",
			end:   "c",
			want: []Item{
				{Key: "a", Value: "one"},
				{Key: "b", Value: "two"},
				{Key: "c", Value: "three"},
			},
		},
		{
			name:  "empty end follows start greater than end rule",
			seed:  map[string]string{"a": "one", "b": "two", "c": "three"},
			start: "a",
			end:   "",
			want:  []Item{},
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

			got := db.Range(tt.start, tt.end)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Range(%q, %q) = %#v, want %#v", tt.start, tt.end, got, tt.want)
			}
		})
	}
}
