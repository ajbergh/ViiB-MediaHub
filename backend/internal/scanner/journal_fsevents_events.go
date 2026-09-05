package scanner

// These flags are stable CoreServices values. Keeping this small check outside
// the Darwin implementation lets the fallback policy be regression-tested on
// every supported build platform.
const (
	fseventsMustScanSubDirs uint32 = 0x00000001
	fseventsUserDropped     uint32 = 0x00000002
	fseventsKernelDropped   uint32 = 0x00000004
	fseventsEventIDsWrapped uint32 = 0x00000008
	fseventsRootChanged     uint32 = 0x00000020
)

func fseventsEventRequiresFallback[T ~uint32](flags T) bool {
	return uint32(flags)&(fseventsMustScanSubDirs|fseventsUserDropped|fseventsKernelDropped|fseventsEventIDsWrapped|fseventsRootChanged) != 0
}
