# Documentation archive

Archived on 2026-10-04. These documents preserve completed implementation work, dated reviews, and research checkpoints. Their branch names, proposed behavior, and validation results describe the time they were written. Archiving a record does not close any unresolved quality or native-runtime gate.

Start with the [current documentation index](../index.md) for user guides and active plans/contracts. Active roadmaps, benchmark workflows, and Spotify parity/validation logs remain outside this archive because they still track open work.

## Completed implementation records

| Record | Scope |
|---|---|
| [Semantic retrieval Phase 1](completed/AI_DJ_SEMANTIC_RAG_PHASE_1_IMPLEMENTATION_PLAN.md) | Delivered semantic retrieval and reliability follow-ups; real-library/Plex QA remains a follow-up |
| [Full reliability remediation](completed/FULL_RELIABILITY_REMEDIATION_STATUS.md) | Completed reliability phases |
| [Performance/API plan](completed/PERFORMANCE_API_REMEDIATION_PLAN.md) and [status](completed/PERFORMANCE_API_REMEDIATION_STATUS.md) | Delivered catalog identity, incremental sync, API, jobs, and recovery work |
| [Remediation status](completed/REMEDIATION_STATUS.md) | Earlier completed fixes and checks |
| [DJ workstation refactor](completed/DJV2_UI_REFACTOR_ENGINEERING_PLAN.md) | Completed layout/component refactor phases |
| [DJ overlay validation](completed/dj-overlay-validation.md) | Dated geometry, interaction, and playback evidence |
| [DJ visual polish](completed/djv2-visual-polish.md) | Delivered control and layout changes |
| [Spotify download integrity fix](completed/SPOTIFY_DOWNLOAD_INTEGRITY_FIX.md) | Download/decoder defect evidence and remediation |

## Reviews

| Record | Scope |
|---|---|
| [September 28 project review](reviews/ViiB-MediaHub_Project_Review9-28.md) | Remediated findings with original triggers and evidence |
| [DJ panel review](reviews/dj-panel-review.md) | Dated loop, waveform, and control findings; status labels are historical and minimized Wails behavior still needs runtime proof |

## Research and past evidence

| Record | Scope and current successor |
|---|---|
| [DJ tempo evidence](research/DJ_TEMPO_EVIDENCE.md) | Earlier measurement checkpoint; current gates are in the [analysis roadmap](../DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md) |
| [Spotify Web Player research plan](research/SPOTIFY_WEBPLAYER_AUTH_AUDIO_ANALYSIS_PLAN.md) | Superseded by the [implementation plan](../SPOTIFY_WEBPLAYER_AUTH_AUDIO_ANALYSIS_IMPLEMENTATION_PLAN.md) and [parity audit](../SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md) |
| [Spotify waveform research](research/SPOTIFY_WAVEFORM_RESEARCH.md) | Dated two-recording probe informing the [proposed metadata/fallback plan](../SPOTIFY_METADATA_AND_LOCAL_FALLBACK_IMPLEMENTATION_PLAN.md); production integration remains proposed |
| [Legacy download rules](research/DOWNLOAD_RULES.md) | Supersonic-era design note; current behavior is in [Downloads](../downloads.md) and [Spotify](../spotify.md) |

## Maintenance

Keep operational guides and active specifications in `docs/`. Move completed plans, superseded research, and dated reviews here with a historical notice, an index entry, and repaired relative links. Preserve original evidence dates; a documentation review is not new runtime evidence. Before archiving a plan with unfinished work, identify its active successor or state the remaining limitation explicitly.

Ignored local notes and PR drafts are kept separately in `docs_internal/archive/` and `output/archive/`; they remain ignored workspace artifacts.
