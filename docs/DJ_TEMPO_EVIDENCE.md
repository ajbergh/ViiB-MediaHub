# DJ tempo and grid evidence

The browser detector samples up to five non-overlapping windows, each at most
20 seconds, distributed across the audio duration. Positive RMS energy changes
in 5 ms windows supply transients. Silence, sustained audio and sections with
fewer than eight transients cannot vote. It compares 60–200 BPM candidates at
0.05 BPM resolution using circular phase concentration and occupied beat slots.
Each usable section has equal weight. The strongest rhythmic section supplies
the first-beat phase; half/double alternatives are retained for review rather
than silently normalizing the chosen tempo.

Browser tempo evidence is the mean candidate support across usable sections.
Server evidence remains the existing candidate-separation score. Neither is a
calibrated accuracy probability, and they are not numerically interchangeable.
Section agreement counts local winners within 1 BPM of the chosen candidate.
The API now exposes the server's persisted alternate BPM and section stability.

**Edit Grid → Analyze track** measures the current grid without changing it,
including when locked. Alignment reports the fraction of detected transients
within 35 ms of a beat, median signed offset, and the change between earliest and
latest section median offsets. These are onset diagnostics: syncopation can lower
alignment and nearest-beat residuals can wrap, so small section shift cannot
prove there is no accumulated drift. Downbeat/bar alignment still requires review.

**Use detected tempo & grid** applies a scan locally to an unlocked deck.
**Verify & lock grid** saves the grid and its straight-grid BPM. Restart the Go
backend after updating to enable BPM persistence and the new server evidence
fields. Scan diagnostics themselves are session-only and must be refreshed after
timing edits. Phase sync continues to require manually reviewed, locked grids.

Validation includes synthetic PCM pulses, sustained tone/silence refusal, quiet
intros, missing kicks, tempo disagreement, offset versus drift, API persistence,
and a real browser decoder/UI audit. This change has not been calibrated against
the held-out music corpus; do not claim a commercial accuracy percentage until
that evaluation is complete. The browser exporter now identifies its output as
`browser-multisection-v2` to keep it distinct from the original baseline.
