# Keyboard Shortcuts

ViiB MediaHub supports keyboard shortcuts for hands-free control of playback, panels, and navigation.

> Shortcuts are **disabled** when focus is in an input field or text area. Press **Escape** to exit an input and re-enable shortcuts.

---

## Playback

| Key | Action |
|---|---|
| `Space` | Play / Pause |
| `→` (Arrow Right) | Seek forward 5 seconds |
| `←` (Arrow Left) | Seek backward 5 seconds |
| `Shift + →` | Next track |
| `Shift + ←` | Previous track |
| `↑` (Arrow Up) | Volume up 5% |
| `↓` (Arrow Down) | Volume down 5% |
| `M` | Toggle mute |
| `Ctrl/Cmd + I` | Open Song Info for the current track (also works with `I` while Now Playing is open) |

---

## Panels

| Key | Action |
|---|---|
| `Q` | Toggle Queue panel |
| `E` | Toggle Equalizer panel |
| `N` | Toggle Now Playing view |
| `Escape` | Close any open panel (Queue, EQ, Now Playing) |

---

## Visualizer (Now Playing open)

| Key | Action |
|---|---|
| `V` | Cycle the 12 visualizer modes while Now Playing is open |

---

## Milkdrop Presets (when Milkdrop visualizer is active)

| Key | Action |
|---|---|
| `]` | Next Milkdrop preset |
| `[` | Previous Milkdrop preset |
| `F` | Toggle favorite for current preset |

---

## DJ Mode

DJ Mode has its own dedicated set of shortcuts active only while on the DJ Mode page.

| Key | Action |
|---|---|
| `Space` | Play/Pause the active deck (Shift does not change the target) |
| `W` | Play/Pause Deck A |
| `P` | Play/Pause Deck B |
| `Q` | Return Deck A to cue |
| `O` | Return Deck B to cue |
| `Tab` | Toggle active deck |
| `Z` / `X` / `C` | Crossfader left / center / right |
| `←` / `→` | Adjust crossfader by 2% toward A / B |
| `E` / `[` | Sync Deck A / Deck B |
| `Shift + ←` / `Shift + →` | Nudge the active deck backward / forward by 20 ms |
| `Alt + Shift + ←` / `Alt + Shift + →` | Fine nudge backward / forward by 5 ms |
| `1`–`8` | Trigger the active deck's hot cue |
| `Shift + 1`–`8` | Set the active deck's hot cue at the current position |
| `/` | Open and focus the DJ library search |
| `?` | Open the DJ keyboard-shortcuts dialog |
| `F11` | Enter/exit fullscreen |

> See the [DJ Mode](dj-mode.md) page for the full list of DJ-specific shortcuts.

---

## Notes

- Global shortcuts work across normal pages except when an input is focused; DJ Mode adds its dedicated mappings while active.
- The `Escape` key blurs a focused input, closes Now Playing/Queue/EQ or context menus, and closes the DJ library/shortcut dialog according to the active surface.
- No modifier key is required for the basic media shortcuts.

- Direct Download and confirmation dialogs keep `Tab` / `Shift + Tab` within the dialog. `Escape` dismisses an idle dialog; it does not interrupt a busy action. Confirmation dialogs initially focus Cancel. Closing a dialog restores its previous focus target when that element is still present.
