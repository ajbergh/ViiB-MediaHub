# Beat This Hugging Face evaluation — 2026-10-12

> Archived on 2026-10-12. This is a historical checkpoint, not the current merge checklist. Further local scanning/DSP improvements and quality qualification are deferred to a future branch. See [Spotify behavior and limitations](../../spotify.md) and [deferred work](../../LOCAL_ANALYSIS_FUTURE_BRANCH.md). Original results and unchecked gates below remain historical evidence.

Branch acceptance work is paused at the user's request. This evaluation changes no product code, live credentials, analysis defaults, or acceptance status.

## Recommendation

Beat This is a promising candidate for **musical beat/downbeat timestamps**, especially ViiB's missing bar-one detection. Keep it as a local external reference until independent beat/downbeat, continuity, timing, and deployment checks qualify it. This Space packages the existing upstream model; it does not introduce improved weights or an independent BPM estimator.

## Sources and exact identity

- [Space source](https://huggingface.co/spaces/teamup-tech/Beat-This/tree/74ffbcb6ce5fbbe82412192a57523a886879a843), commit `74ffbcb6ce5fbbe82412192a57523a886879a843`.
- [Weights](https://huggingface.co/teamup-tech/Beat-This-weights/tree/4afbe376535aaab71b76ae2ca2d7c16b08877321), commit `4afbe376535aaab71b76ae2ca2d7c16b08877321`.
- Downloaded `final0.ckpt`: **81,058,141 bytes**, SHA-256 `8c328b45f59d8dd3dff219253ff6a8d6482be57d0133a29140e2febbf8eb8331`. This exactly matches the checkpoint used in ViiB's existing reference benchmark.
- [Paper, arXiv 2407.21658v1](https://arxiv.org/pdf/2407.21658), including methods, experimental results, limitations, and references.
- [Official implementation/model guidance](https://github.com/CPJKU/beat_this#available-models). Code and published weights are MIT licensed; preserve upstream notices if distributing them. Model licensing does not grant rights to its training audio.

## What the Space actually does

The six inference implementation files are byte-identical to installed `beat-this==1.1.0`. The app resolves the weights at a fixed revision, runs `File2Beats(dbn=False)`, and returns the original audio with beat/downbeat markers. It supplies **no BPM, key, meter, section labels, or calibrated confidence**. Downbeats are snapped to detected beats, so the app's exact timestamp membership test is valid.

Audio is decoded through torchaudio with soundfile/madmom fallbacks, averaged to mono, and resampled to 22.05 kHz with soxr. The implementation uses 128 mel bands, a 1024-sample window, hop 441, and 50 frames/second. Its mel upper bound is 11 kHz. Inference uses 1500-frame chunks, discards six border frames, shifts the last chunk to the track end, and keeps earlier predictions in overlaps. Local-max peak picking uses logit > 0, adjacent-peak deduplication, and nearest-beat downbeat snapping. There is no enforced constant tempo or meter.

The Space's decoder path provides no canonical timing/hash provenance. For ViiB comparisons, feed ViiB-decoded canonical WAVs through the existing adapter. Avoid uploading library recordings merely to exercise the hosted demo.

The Space declares Torch/Torchaudio 2.11.0, Python 3.11, pyharp at git tag `v0.4.1`, and `spaces>=0.51.1`; it dynamically selects CPU/CUDA. Our executed environment uses Torch/Torchaudio 2.5.1+cpu. Source/weight equivalence does not establish identical outputs on every runtime/device. The `model.json` checkpoint repository is inconsistent with the working `app.py` repository and should be corrected before using a generic metadata-based loader.

## Paper assessment

The paper targets beat/downbeat tracking, not scalar BPM. Its transformer removes DBN tempo/meter assumptions and uses weighted shift-tolerant loss and augmentation. GTZAN results average three seeds: beat F1 89.1%, downbeat F1 78.3%; continuity CMLt is 79.8% and 67.3%, respectively. Adding DBN improves downbeat CMLt to 73.3% but reduces F1. The authors acknowledge irregular predictions on difficult music and annotation problems. F1 permits ±70 ms and standard evaluation discards the first five seconds; neither establishes DJ sync or intro-cue accuracy. AMLt also accepts alternate metrical levels. The roughly 2M-parameter small model scores near the roughly 20M-parameter full model, making it a useful future deployment experiment; this HF repository contains only full `final0`. Training includes Harmonix and GuitarSet comping tracks; GTZAN is test-only. Those overlaps require recording-level review before qualification. These findings support testing downbeats and continuity separately from global tempo. [Paper](https://arxiv.org/pdf/2407.21658).

## Executed evidence

Downloaded Space inference code plus the downloaded HF checkpoint ran locally on **12 deterministic duration-spread FSLD tuning clips**, from 0.48 to 80 seconds, including multi-chunk audio. All beat and downbeat arrays exactly matched the prior 1.1.0 reference results, and every downbeat was a detected beat. File hashes, runtime versions, selection IDs, and results are in [the validation artifact](evidence/beat-this-hf-validation-2026-10-12.json). This checks equivalence, not accuracy against independent beat/downbeat labels. The hosted UI, its Torch 2.11 runtime, and GPU execution were not tested.

The already completed **308-clip tuning benchmark** therefore remains relevant to these exact weights and inference implementation: 66 strict BPM matches within ±0.5 BPM, 22 half/double errors, and 152 unknown BPM results. Of those unknowns, 142 emit beats but fewer than the 17 required by the fixed 16-beat-span estimator; ten emit no beats. These figures describe **model plus scalar adapter on short loops**, not the paper's beat/downbeat accuracy. A shorter-span estimator would need a separate declared experiment; changing it after seeing results cannot turn this run into qualification evidence. [Full benchmark evidence](evidence/spotify-fsld-beat-this-reference-2026-10-12.json).

That prior Linux CPU process analyzed 3,902.65 audio seconds in 208.59 seconds with four-thread settings and peak RSS 1,380,028 KiB (about 1.32 GiB). It includes imports/model initialization and excludes installation/canonical export. This suggests background analysis is practical on the measured machine, but does not establish Windows/macOS packaging, latency, or real-time suitability. FSLD training overlap remains unreviewed. Reserved FSLD and GuitarSet splits remain untouched.

## Proposed next experiment, when branch work resumes

Use a lawful, recording-overlap-reviewed full-song corpus with independently annotated beats/downbeats. Freeze checkpoint, CPU runtime, canonical timing, and postprocessing before reserved evaluation. Measure beat/downbeat F1, CMLt, bar-one accuracy, intro performance without five-second trimming, local drift, half/double errors, and abstention. Treat the model's logits as scores rather than calibrated trust. Compare full and official small checkpoints if deployment cost matters. Only then decide whether to expose an optional local analysis worker with provenance and native fallback. No integration or promotion is made by this review.
