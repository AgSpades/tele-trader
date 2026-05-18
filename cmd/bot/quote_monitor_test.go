package main

import (
	"encoding/json"
	"testing"
)

func TestExtractQuoteLTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want float64
		ok   bool
	}{
		{
			name: "nested numeric ltp",
			raw:  `{"status":"success","data":{"ltp":118.75}}`,
			want: 118.75,
			ok:   true,
		},
		{
			name: "nested string ltp",
			raw:  `{"status":"success","data":{"ltp":"83.72"}}`,
			want: 83.72,
			ok:   true,
		},
		{
			name: "missing ltp",
			raw:  `{"status":"success","data":{"open":100}}`,
			ok:   false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := extractQuoteLTP(json.RawMessage(tc.raw))
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("ltp = %v, want %v", got, tc.want)
			}
		})
	}
}
