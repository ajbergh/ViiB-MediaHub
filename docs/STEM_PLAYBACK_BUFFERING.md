# Stem playback buffering investigation

## Findings

- Stem activation previously waited for only 0.35 seconds or one small chunk, while the nominal 2–4 second buffer filled in the background. Higher sample rates and faster tempos made that startup cushion particularly small.
- A cancelled seek could trigger full-track fallback. Pending mode transitions and old loop requests could also override newer user actions because track-load generation alone did not distinguish operations within one track.
- Seeking outside retained history flushed the worklet without restoring the enabled loop's frames. Wrapping into that discarded history could underrun.
- The worklet advanced through missing samples. Late blocks then arrived behind the playhead, turning a delivery delay into skipped audio.
- Each backend frame response performs package validation and a fresh source identity check. Inspection shows full-file hashing on this path, including package-signature work before validation-cache lookup. This is a potential disk/CPU bottleneck, especially with two decks; production latency has not been measured here.

## Implemented changes

[StemDeckSource](../lib/stemDeckSource.ts) fills its complete startup/seek watermark before enabling or resuming stem playback. The watermark accounts for tempo above unity, preserving the configured wall-clock coverage. Normal settings use four seconds of read-ahead; the configurable range remains two to four seconds before tempo adjustment. The short remainder at track end is accepted.

Requests now use whole chunks up to 131,072 source frames, fitting the endpoint's existing 4 MiB limit for four stereo float32 buses. This raises the previous default chunk size by roughly four times and avoids tiny top-up requests. Frame geometry validation and backend package/source integrity checks remain enabled.

Separate operation revisions invalidate superseded mode/seek work and buffer fills. Expected aborts and stale loop failures cannot select fallback or resume an older operation. A flush refills the enabled loop before playback resumes. Completion of an old request cannot clear the bookkeeping for a newer fill.

The [transport worklet](../lib/stemTransport.worklet.js) holds at a missing source frame and emits one underrun notification until that position becomes playable. Ahead-only blocks do not clear the underrun. A recovery message restarts the source's position estimate, so late data can recover without skipping the missing section. Buffering can still cause an audible gap if delivery cannot keep up; these changes do not guarantee uninterrupted audio on a persistently slow server.

## Verification and hardware retest

Automated tests cover delayed 96/192 kHz startup at 1.5x tempo, bounded request sizes, repeated seeks, stale requests across track loads, Stems-to-Full races, changed-loop failures, loop refills after flush, and worklet clock/recovery behavior. Existing key-lock, scratching, loop, and fallback tests also remain relevant.

After rebuilding, test two prepared tracks simultaneously, then repeat with key lock, pitch changes, hot-cue jumps, and short loops. Compare underrun counts and check for unexpected Full/fallback transitions. Initial loads and long seeks intentionally wait for more audio than before.

If underruns persist, capture frame-request duration in the browser's network tools and compare it with the audio duration returned by each request divided by playback tempo. Measure backend package-signature, source-hash, decode, and packing time before choosing a server cache redesign. Any server optimization must continue detecting replaced, corrupted, or invalidated packages and changed source files; bypassing those checks is not part of this fix.
