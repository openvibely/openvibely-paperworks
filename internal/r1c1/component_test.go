package r1c1

import "testing"

func TestResolveComponentKindsAndBounds(t *testing.T) {
	for _, tc := range []struct {
		name      string
		component string
		origin    int
		limit     int
		want      int
		absolute  bool
		wantError bool
	}{
		{name: "omitted is relative", origin: 7, limit: 20, want: 7},
		{name: "bracketed offset is relative", component: "[-2]", origin: 7, limit: 20, want: 5},
		{name: "signed positive offset is relative", component: "[+3]", origin: 7, limit: 20, want: 10},
		{name: "unbracketed number is absolute", component: "12", origin: 7, limit: 20, want: 12, absolute: true},
		{name: "relative lower bound", component: "[-7]", origin: 7, limit: 20, wantError: true},
		{name: "relative upper bound", component: "[14]", origin: 7, limit: 20, wantError: true},
		{name: "absolute lower bound", component: "0", origin: 7, limit: 20, wantError: true},
		{name: "absolute upper bound", component: "21", origin: 7, limit: 20, wantError: true},
		{name: "overflowing offset", component: "[9223372036854775807]", origin: 7, limit: 20, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, absolute, err := ResolveComponent(tc.component, tc.origin, tc.limit)
			if tc.wantError {
				if err == nil {
					t.Fatalf("ResolveComponent(%q) = %d, %t; want an error", tc.component, got, absolute)
				}
				return
			}
			if err != nil || got != tc.want || absolute != tc.absolute {
				t.Fatalf("ResolveComponent(%q) = %d, %t, %v; want %d, %t", tc.component, got, absolute, err, tc.want, tc.absolute)
			}
		})
	}
}
