# Documentation review — 2026-10-23

Reviewed all 104 tracked Markdown files against `main` at `818cf94`. Ignored `output/` PR drafts, local notes and generated dependencies are workspace artifacts outside the maintained documentation set. Files that remain accurate are retained without cosmetic rewrites; original validation dates and historical results remain intact.

## Corrections and removals

- Align current Go requirements with 1.26.10 and Wails status with v2.17.0.
- Rebuild unreleased notes from changes after RC8 (#156–#160).
- Correct stem codec/discovery/startup-scan behavior and distinguish implemented scheduling from historical design gaps.
- Reconcile Spotify scalar precedence and remove obsolete active PR #117 handoffs. Preserve archived milestone logs and repair inbound links.
- Describe release inventory, checksum, provenance and channel guards as implemented, retaining signing and consumer-verification gates.
- Add missing DJ/controller/cue/Free-account references and release navigation; label dated reviews as historical.

## Coverage

| Markdown file | Disposition |
|---|---|
| [.github/agents/tasksync-v5.2.md](../.github/agents/tasksync-v5.2.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [.github/copilot-instructions.md](../.github/copilot-instructions.md) | Updated: factual status, current behavior, navigation or successor links |
| [README.md](../README.md) | Updated: factual status, current behavior, navigation or successor links |
| [assets/icons/README.md](../assets/icons/README.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [backend/internal/semantic/testdata/README.md](../backend/internal/semantic/testdata/README.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [backend/internal/spotify/analysis/AUDIO_ATTRIBUTES_V2_CONTRACT.md](../backend/internal/spotify/analysis/AUDIO_ATTRIBUTES_V2_CONTRACT.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [backend/internal/spotify/catalog/SOURCES.md](../backend/internal/spotify/catalog/SOURCES.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/ACCESSIBILITY_QUALIFICATION.md](ACCESSIBILITY_QUALIFICATION.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/AUDIO_FORMAT_SUPPORT_GAPS.md](AUDIO_FORMAT_SUPPORT_GAPS.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md](DJV2_PROFESSIONAL_TRACK_ANALYSIS_ROADMAP.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/DJV2_PRO_STEMS_CUES_MIX_INTELLIGENCE_ROADMAP.md](DJV2_PRO_STEMS_CUES_MIX_INTELLIGENCE_ROADMAP.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/DJV2_TRACK_ANALYSIS_CONCISE_CONTEXT.md](DJV2_TRACK_ANALYSIS_CONCISE_CONTEXT.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/DJ_AUTO_CUE_ACCURACY_RESEARCH.md](DJ_AUTO_CUE_ACCURACY_RESEARCH.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/DJ_AUTO_CUE_IMPLEMENTATION.md](DJ_AUTO_CUE_IMPLEMENTATION.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/DJ_CUE_REAL_AUDIO_VALIDATION.md](DJ_CUE_REAL_AUDIO_VALIDATION.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/DJ_MIDI_PAD_MODES.md](DJ_MIDI_PAD_MODES.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/DJ_MIXER_CONTROLS_REVIEW.md](DJ_MIXER_CONTROLS_REVIEW.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/DJ_PANEL_PERFORMANCE_REVIEW.md](DJ_PANEL_PERFORMANCE_REVIEW.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/DOCUMENTATION_REVIEW_2026-10-12.md](DOCUMENTATION_REVIEW_2026-10-12.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/DOCUMENTATION_REVIEW_2026-10-15.md](DOCUMENTATION_REVIEW_2026-10-15.md) | Retained: dated evidence/history remains relevant |
| [docs/EXPERIMENTAL_DJ_CONTROLLERS.md](EXPERIMENTAL_DJ_CONTROLLERS.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/FOUR_CHANNEL_DJ_AUDIO.md](FOUR_CHANNEL_DJ_AUDIO.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/LOCAL_ANALYSIS_FUTURE_BRANCH.md](LOCAL_ANALYSIS_FUTURE_BRANCH.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/MACOS_BUILD_REVIEW.md](MACOS_BUILD_REVIEW.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/NETWORK_AUDIO_OUTPUT_RESEARCH.md](NETWORK_AUDIO_OUTPUT_RESEARCH.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/REFERENCE_BPM_BENCHMARK.md](REFERENCE_BPM_BENCHMARK.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/REFERENCE_RHYTHM_STRUCTURE_BENCHMARK.md](REFERENCE_RHYTHM_STRUCTURE_BENCHMARK.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/ROADMAP.md](ROADMAP.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/ROADMAP_EASY_PASS_2026-10-20.md](ROADMAP_EASY_PASS_2026-10-20.md) | Retained: dated evidence/history remains relevant |
| [docs/SPOTIFY_BPM_DIAGNOSTIC.md](SPOTIFY_BPM_DIAGNOSTIC.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/SPOTIFY_BPM_KEY_IMPORT.md](SPOTIFY_BPM_KEY_IMPORT.md) | Updated: factual status, current behavior, navigation or successor links |
| `docs/SPOTIFY_BRANCH_CLOSEOUT_NEXT_STEPS.md` | Removed: redundant merged-branch handoff; historical milestone copies remain archived |
| `docs/SPOTIFY_BRANCH_REMAINING_WORK.md` | Removed: redundant merged-branch handoff; historical milestone copies remain archived |
| [docs/SPOTIFY_CONNECTED_RESOURCE_INVENTORY.md](SPOTIFY_CONNECTED_RESOURCE_INVENTORY.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md](SPOTIFY_COOKIE_AUTH_PARITY_AUDIT.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/SPOTIFY_FREE_ACCOUNT_PROBE.md](SPOTIFY_FREE_ACCOUNT_PROBE.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/SPOTIFY_MACOS_AUTH_VALIDATION.md](SPOTIFY_MACOS_AUTH_VALIDATION.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/SPOTIFY_PREPARATION_PERFORMANCE.md](SPOTIFY_PREPARATION_PERFORMANCE.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/SPOTIFY_REFERENCE_BENCHMARK.md](SPOTIFY_REFERENCE_BENCHMARK.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/SPOTIFY_WEBPLAYER_AUTH_AUDIO_ANALYSIS_IMPLEMENTATION_PLAN.md](SPOTIFY_WEBPLAYER_AUTH_AUDIO_ANALYSIS_IMPLEMENTATION_PLAN.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/SPOTIFY_WEBPLAYER_VALIDATION.md](SPOTIFY_WEBPLAYER_VALIDATION.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/STEM_PLAYBACK_BUFFERING.md](STEM_PLAYBACK_BUFFERING.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/VIIB_STEM_PACKAGE_V1.md](VIIB_STEM_PACKAGE_V1.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/WAILS_V2_17_UPGRADE_PLAN.md](WAILS_V2_17_UPGRADE_PLAN.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/albums.md](albums.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/architecture.md](architecture.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/archive/README.md](archive/README.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/archive/completed/AI_DJ_SEMANTIC_RAG_PHASE_1_IMPLEMENTATION_PLAN.md](archive/completed/AI_DJ_SEMANTIC_RAG_PHASE_1_IMPLEMENTATION_PLAN.md) | Retained: dated evidence/history remains relevant |
| [docs/archive/completed/DJV2_UI_REFACTOR_ENGINEERING_PLAN.md](archive/completed/DJV2_UI_REFACTOR_ENGINEERING_PLAN.md) | Retained: dated evidence/history remains relevant |
| [docs/archive/completed/FULL_RELIABILITY_REMEDIATION_STATUS.md](archive/completed/FULL_RELIABILITY_REMEDIATION_STATUS.md) | Retained: dated evidence/history remains relevant |
| [docs/archive/completed/PERFORMANCE_API_REMEDIATION_PLAN.md](archive/completed/PERFORMANCE_API_REMEDIATION_PLAN.md) | Retained: dated evidence/history remains relevant |
| [docs/archive/completed/PERFORMANCE_API_REMEDIATION_STATUS.md](archive/completed/PERFORMANCE_API_REMEDIATION_STATUS.md) | Retained: dated evidence/history remains relevant |
| [docs/archive/completed/REMEDIATION_STATUS.md](archive/completed/REMEDIATION_STATUS.md) | Retained: dated evidence/history remains relevant |
| [docs/archive/completed/SPOTIFY_DOWNLOAD_INTEGRITY_FIX.md](archive/completed/SPOTIFY_DOWNLOAD_INTEGRITY_FIX.md) | Retained: dated evidence/history remains relevant |
| [docs/archive/completed/dj-overlay-validation.md](archive/completed/dj-overlay-validation.md) | Retained: dated evidence/history remains relevant |
| [docs/archive/completed/djv2-visual-polish.md](archive/completed/djv2-visual-polish.md) | Retained: dated evidence/history remains relevant |
| [docs/archive/research/DJ_TEMPO_EVIDENCE.md](archive/research/DJ_TEMPO_EVIDENCE.md) | Retained: dated evidence/history remains relevant |
| [docs/archive/research/DOWNLOAD_RULES.md](archive/research/DOWNLOAD_RULES.md) | Retained: dated evidence/history remains relevant |
| [docs/archive/research/SPOTIFY_WAVEFORM_RESEARCH.md](archive/research/SPOTIFY_WAVEFORM_RESEARCH.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/archive/research/SPOTIFY_WEBPLAYER_AUTH_AUDIO_ANALYSIS_PLAN.md](archive/research/SPOTIFY_WEBPLAYER_AUTH_AUDIO_ANALYSIS_PLAN.md) | Retained: dated evidence/history remains relevant |
| [docs/archive/reviews/ViiB-MediaHub_Project_Review9-28.md](archive/reviews/ViiB-MediaHub_Project_Review9-28.md) | Retained: dated evidence/history remains relevant |
| [docs/archive/reviews/dj-panel-review.md](archive/reviews/dj-panel-review.md) | Retained: dated evidence/history remains relevant |
| [docs/archive/spotify-metadata-foundation/BEAT_THIS_HUGGING_FACE_EVALUATION.md](archive/spotify-metadata-foundation/BEAT_THIS_HUGGING_FACE_EVALUATION.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/archive/spotify-metadata-foundation/DJV2_TRACK_ANALYSIS_CONCISE_CONTEXT.md](archive/spotify-metadata-foundation/DJV2_TRACK_ANALYSIS_CONCISE_CONTEXT.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/archive/spotify-metadata-foundation/README.md](archive/spotify-metadata-foundation/README.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/archive/spotify-metadata-foundation/SPOTIFY_BRANCH_CLOSEOUT_NEXT_STEPS.md](archive/spotify-metadata-foundation/SPOTIFY_BRANCH_CLOSEOUT_NEXT_STEPS.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/archive/spotify-metadata-foundation/SPOTIFY_BRANCH_REMAINING_WORK.md](archive/spotify-metadata-foundation/SPOTIFY_BRANCH_REMAINING_WORK.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/archive/spotify-metadata-foundation/SPOTIFY_METADATA_AND_LOCAL_FALLBACK_IMPLEMENTATION_PLAN.md](archive/spotify-metadata-foundation/SPOTIFY_METADATA_AND_LOCAL_FALLBACK_IMPLEMENTATION_PLAN.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/archive/spotify-metadata-foundation/SPOTIFY_PUBLIC_CORPUS_EVALUATION.md](archive/spotify-metadata-foundation/SPOTIFY_PUBLIC_CORPUS_EVALUATION.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/artists.md](artists.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/development.md](development.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/dj-mode.md](dj-mode.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/downloads.md](downloads.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/duplicates.md](duplicates.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/genres.md](genres.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/home.md](home.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/index.md](index.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/keyboard-shortcuts.md](keyboard-shortcuts.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/library-operations.md](library-operations.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/liked.md](liked.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/player.md](player.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/playlists.md](playlists.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/plex-music.md](plex-music.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/release-asset-verification.md](release-asset-verification.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/release-dependency-inventories.md](release-dependency-inventories.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/release-provenance.md](release-provenance.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/release-signing-policy.md](release-signing-policy.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/releases/UNRELEASED.md](releases/UNRELEASED.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/releases/v1.0.0-rc6.md](releases/v1.0.0-rc6.md) | Retained: dated evidence/history remains relevant |
| [docs/releases/v1.0.0-rc7.md](releases/v1.0.0-rc7.md) | Retained: dated evidence/history remains relevant |
| [docs/releases/v1.0.0-rc8.md](releases/v1.0.0-rc8.md) | Retained: dated evidence/history remains relevant |
| [docs/repository-governance.md](repository-governance.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/scan-stability-and-storage.md](scan-stability-and-storage.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/screenshots.md](screenshots.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/search.md](search.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/settings.md](settings.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/smart-playlists.md](smart-playlists.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/songs.md](songs.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/spotify.md](spotify.md) | Updated: factual status, current behavior, navigation or successor links |
| [docs/stats.md](stats.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/support-bundles.md](support-bundles.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [docs/third-party/model2vec.md](third-party/model2vec.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [fixtures/stems/README.md](../fixtures/stems/README.md) | Reviewed: current guide, contract or scoped reference remains accurate |
| [testdata/browser-metadata/README.md](../testdata/browser-metadata/README.md) | Reviewed: current guide, contract or scoped reference remains accurate |

## Validation scope

`python scripts/check_documentation_links.py` passed for 103 remaining Markdown files, 806 local links and 47 documentation JSON files, with zero failures. `git diff --check` passed. Local link and JSON validation does not verify external URLs, Markdown anchors, published release payloads, provider parity, hardware routing or professional musical accuracy. No new runtime qualification is implied by this review.
