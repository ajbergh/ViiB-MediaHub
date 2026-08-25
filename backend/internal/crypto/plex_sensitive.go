package crypto

// Plex credentials contain account/server tokens and the private device key used
// by the Plex JWT PIN flow, so they use the same encrypted settings mechanism as
// existing OAuth/API secrets.
func init() {
	sensitiveKeys["plex_credentials"] = true
}
