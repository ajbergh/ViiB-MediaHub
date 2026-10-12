# DJv2 track analysis — current context

**Updated:** 2026-10-12. Further local scanning/DSP improvements are paused and deferred to a future branch by the owner. The Spotify metadata PR retains implemented functionality and correctness safeguards; it does not claim professional BPM/key, downbeat, structure or cue accuracy.

Use [the future-branch backlog](LOCAL_ANALYSIS_FUTURE_BRANCH.md) to resume development, [current merge scope](SPOTIFY_BRANCH_REMAINING_WORK.md) for this PR, and [the detailed roadmap](DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md) for design and quality criteria. The [previous context](archive/spotify-metadata-foundation/DJV2_TRACK_ANALYSIS_CONCISE_CONTEXT.md) is archived as dated evidence.

Existing analysis, manual locks, source-aware provenance, fallback and unknown states remain available. Beat-grid phase sync, cue snapping and grid-quantized loops require a valid locked manual grid; automatic provider or meter-derived grids do not qualify. Benchmark-only estimator candidates remain explicit experiments, production defaults are unchanged, and Beat This remains an external reference rather than an application dependency.

Public evaluation evidence is indexed in [the branch archive](archive/spotify-metadata-foundation/README.md). Measured failures, contaminated/unreviewed training overlap, calibration gaps and reserved split boundaries remain preserved; deferral does not count as passing an accuracy gate.
