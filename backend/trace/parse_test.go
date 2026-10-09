package trace

import (
	"net/netip"
	"testing"
)

const macOutput = `traceroute to 1.1.1.1 (1.1.1.1), 6 hops max, 40 byte packets
 1  192.168.1.1  3.666 ms  4.334 ms
 2  * *
 3  96.34.20.4  26.460 ms  27.540 ms
    96.34.20.8  30.1 ms
 4  159.111.150.124  18.056 ms *
 5  2606:4700::1111  9.5 ms  10.5 ms
`

const linuxOutput = `traceroute to example.com (93.184.216.34), 30 hops max, 60 byte packets
 1  10.0.0.1  0.412 ms  0.388 ms
 2  10.0.0.2  1.2 ms !H  1.4 ms
 3  93.184.216.34  11.9 ms  12.1 ms
`

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Hop
	}{
		{"macos", macOutput, []Hop{
			{1, netip.MustParseAddr("192.168.1.1"), "", 4},
			{2, netip.Addr{}, "", 0},
			{3, netip.MustParseAddr("96.34.20.4"), "", 27},
			{4, netip.MustParseAddr("159.111.150.124"), "", 18.06},
			{5, netip.MustParseAddr("2606:4700::1111"), "", 10},
		}},
		{"linux", linuxOutput, []Hop{
			{1, netip.MustParseAddr("10.0.0.1"), "", 0.4},
			{2, netip.MustParseAddr("10.0.0.2"), "", 1.3},
			{3, netip.MustParseAddr("93.184.216.34"), "", 12},
		}},
		{"empty", "", nil},
		{"garbage", "traceroute: unknown host\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d hops %+v, want %d", len(got), got, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("hop %d = %+v, want %+v", i+1, got[i], tt.want[i])
				}
			}
		})
	}
}
