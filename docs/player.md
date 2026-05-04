# Player

![Player — Queue panel](../assets/screenshots/player-queue.png)

![Player — Equalizer panel](../assets/screenshots/player-equalizer.png)

The Player bar is persistent at the bottom of the screen and provides all core playback controls. It also serves as the entry point for the Queue, Equalizer, and Now Playing panels.

---

## Player Bar Layout

```
┌────────────────────────────────────────────────────────────────────────┐
│ [Album art]  Track title          ◀◀  ▶/⏸  ▶▶   ─── progress ───  🔊  │
│              Artist · Album       Shuffle  Repeat  Vol  Queue  EQ  ↑  │
└────────────────────────────────────────────────────────────────────────┘
```

On narrower screens, shuffle, repeat, EQ, and secondary controls are hidden. On mobile, only the essential controls (play/pause, skip) are shown.

---

## Playback Controls

| Control | Description |
|---|---|
| ◀◀ Previous | Jump to the previous track in the queue (or restart current if >3s in) |
| ▶/⏸ Play/Pause | Toggle playback |
| ▶▶ Next | Skip to the next track in the queue |
| Shuffle | Toggle shuffle mode (randomizes queue order) |
| Repeat | Cycle through Off → Repeat All → Repeat One |

---

## Progress Bar

- Click anywhere on the progress bar to seek.
- Hover to see the time at the cursor position.
- Current position and total duration are displayed on either side.

---

## Volume

- Drag the volume slider to adjust.
- Click the speaker icon to mute/unmute.

---

## Album Artwork

- Displays the album cover of the current track (or a color gradient fallback).
- Click the artwork to open the **Now Playing** panel (full-screen expanded view).

---

## Visualizer

When a visualizer mode is active, a visualization renders inside the player bar. Modes: **Waveform**, **Spectrum**, **Milkdrop**. Configure in [Settings → Audio](settings.md#audio).

---

## Sleep Timer

Click the moon icon to set a sleep timer. Available durations: 15, 30, 45, 60, 90, 120 minutes.

When the timer expires, the music fades out and playback stops. The remaining time is shown on the icon.

---

## Queue Panel

Click the queue icon (≡) to toggle the Queue panel. It slides in from the right side.

### Queue Features

| Feature | Description |
|---|---|
| Virtualized list | Smooth scrolling even with 2,000+ tracks queued |
| Drag to reorder | Grab the drag handle ( ⠿ ) to move tracks |
| Remove | Click × on any row to remove it from the queue |
| Clear All | Button at the top to empty the queue |
| Jump to Current | Button that scrolls the list to the currently playing track |
| Click to play | Click any row to jump to that track |

---

## Equalizer Panel

Click the EQ icon ( sliders ) or press **E** to open the Equalizer panel as a modal overlay.

### EQ Features

- **10 frequency bands**: 32 Hz, 64 Hz, 125 Hz, 250 Hz, 500 Hz, 1 kHz, 2 kHz, 4 kHz, 8 kHz, 16 kHz
- **Gain range**: −12 dB to +12 dB per band
- **Enable/Disable toggle** — bypass the EQ without losing your settings
- **Reset to Flat** — set all bands to 0 dB
- **Presets**: Flat, Rock, Pop, Jazz, Classical, Bass Boost, Treble Boost, and more

Drag each frequency slider vertically to adjust the gain. Changes apply in real time.

---

## Now Playing Panel

Click the album art or the expand icon (↑) to open the Now Playing panel as a full-screen overlay.

### Now Playing Features

- Large album artwork
- Track title, artist, album
- Playback controls (same as the player bar)
- Progress bar with seek
- Visualizer (cycles with **V** key)
- Like button
- Milkdrop preset cycling (when Milkdrop mode is active)

### Closing Now Playing

Press **Escape** or click the ✕ button to close.

---

## Streaming Tracks

When playing a Spotify streamed track (requires librespot setup), the player bar shows a Spotify badge next to the track info. Seek and volume behave the same as local playback.

---

## Media Session / OS Integration

ViiB MediaHub registers with the OS **Media Session API**:

- **Windows**: Media transport controls on the taskbar, lock screen, and compatible Bluetooth headsets
- **macOS**: Control Strip and headset controls
- **Linux**: MPRIS via the browser's media session support

Album art, track title, and artist are sent to the OS for display in the system media overlay.
