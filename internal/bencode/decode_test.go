package bencode

import (
	"reflect"
	"testing"
)

func TestDecode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{"positive int", "i42e", int64(42)},
		{"negative int", "i-3e", int64(-3)},
		{"zero", "i0e", int64(0)},
		{"max int64", "i9223372036854775807e", int64(9223372036854775807)},
		{"string", "8:deadbeef", "deadbeef"},
		{"empty string", "0:", ""},
		{"binary string", "3:\x00\xff\x01", "\x00\xff\x01"},
		{"string containing delimiters", "5:i1e:e", "i1e:e"},
		{"list", "l4:spami42ee", []any{"spam", int64(42)}},
		{"empty list", "le", []any(nil)},
		{"dict", "d3:cow3:moo4:spam4:eggse", map[string]any{"cow": "moo", "spam": "eggs"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := Decode([]byte(tt.input))
			if err != nil {
				t.Fatalf("Decode(%q) error: %v", tt.input, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Decode(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
