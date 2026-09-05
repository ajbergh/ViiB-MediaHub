package scanner

import "testing"

func TestFSEventsEventRequiresFallback(t *testing.T) {
	tests := []struct {
		name  string
		flags uint32
		want  bool
	}{
		{name: "ordinary file event", flags: 0x00010000, want: false},
		{name: "history sentinel", flags: 0x00000010, want: false},
		{name: "coalesced hierarchy", flags: fseventsMustScanSubDirs, want: true},
		{name: "kernel drop", flags: fseventsKernelDropped, want: true},
		{name: "root renamed", flags: fseventsRootChanged, want: true},
		{name: "wrapped cursor", flags: fseventsEventIDsWrapped, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fseventsEventRequiresFallback(tt.flags); got != tt.want {
				t.Fatalf("fseventsEventRequiresFallback(%#x) = %v, want %v", tt.flags, got, tt.want)
			}
		})
	}
}
