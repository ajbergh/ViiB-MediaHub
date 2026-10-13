# Deferred local scanning and analysis work

**Owner decision:** 2026-10-12. Further improvements are paused on `feature/spotify-metadata-foundation` and belong to a future branch. This supersedes earlier instructions to continue this branch until every local-analysis gate closes. No future branch name or schedule has been assigned.

## Resume scope

- Improve local BPM/key accuracy and confidence calibration, including half/double selection, wide tempo ranges, short clips and honest abstention.
- Optimize scan/backfill and preparation scheduling, cancellation/retry and shared decoding. Measure fresh/warm/offline/connected behavior without duplicate-decode or provider storms.
- Qualify energy/loudness, musical downbeats/meter, structure, cue usefulness and timing. Keep provider scores, local measurements and inferred-from-meter placeholders distinct.
- Evaluate Beat This/full and small models on full songs with independent beat/downbeat labels, canonical timing, recording-level training-overlap review, continuity/drift and intro/bar-one metrics. Decide runtime packaging and fallback only after qualification.
- Expand lawful public corpora, review near duplicates and recording identity, freeze candidates before reserved evaluation, and retain source/license hashes. The owner confirmed noncommercial project/evaluation use; this does not waive each dataset's attribution or other terms.
- Complete real-audio Windows/macOS/Linux, native/browser, long-track accessibility and resource measurements. Existing compiler/synthetic/WSL evidence does not establish all host workflows.

## Preserved checkpoint

The production tempo defaults remain `multifeature-half-bpm`, 90–180 BPM, crest 15. The benchmark-only `phase-support` and wider-range candidates failed qualification and are not promoted. Key candidates and external references likewise retain their measured limitations. No Python/Torch/Beat This dependency is added to the product.

The archived public evaluation reports record 487 acquired FSLD recordings (308 tuning, 179 reserved) and GuitarSet's 238 tuning/119 reserved split after errata. Reserved FSLD/GuitarSet audio remains unanalyzed at this checkpoint. HU33's 24 performances already received evaluation; do not reuse them as unseen qualification data. All prior failures and split boundaries remain binding evidence for future experiments.

Start from [the archived public-corpus report](archive/spotify-metadata-foundation/SPOTIFY_PUBLIC_CORPUS_EVALUATION.md), [Beat This evaluation](archive/spotify-metadata-foundation/BEAT_THIS_HUGGING_FACE_EVALUATION.md), [milestone log through 275](archive/spotify-metadata-foundation/SPOTIFY_BRANCH_CLOSEOUT_NEXT_STEPS.md), and [original acceptance plan](archive/spotify-metadata-foundation/SPOTIFY_METADATA_AND_LOCAL_FALLBACK_IMPLEMENTATION_PLAN.md). The current [BPM](REFERENCE_BPM_BENCHMARK.md) and [rhythm/structure](REFERENCE_RHYTHM_STRUCTURE_BENCHMARK.md) contracts remain reproducibility references.

Separate non-DSP follow-ups include stable-path-independent encrypted credential migration, global cache quotas, and the provider/platform gaps in [the parity audit](SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md). They are not silently included in a future DSP implementation.
