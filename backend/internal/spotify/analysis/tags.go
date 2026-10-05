package analysis

// InitialKey returns the traditional notation used by Vorbis and ID3 tags.
// Spotify uses mode 1 for major and 0 for minor.
func (o Observation) InitialKey() string {
	if o.Key == nil || o.Mode == nil || *o.Key < 0 || *o.Key > 11 || (*o.Mode != 0 && *o.Mode != 1) {
		return ""
	}
	names := [...]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	value := names[*o.Key]
	if *o.Mode == 0 {
		value += "m"
	}
	return value
}
