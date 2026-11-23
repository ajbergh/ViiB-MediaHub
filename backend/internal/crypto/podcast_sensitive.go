// Registers Podcast Index directory credentials as sensitive during package initialization.

package crypto

// Podcast Index directory search uses a user-supplied API key and secret.
// Registering the setting explicitly keeps it inside the existing machine-bound
// enc:v2: encryption mechanism.
func init() {
	sensitiveKeys["podcast_index_credentials"] = true
}
