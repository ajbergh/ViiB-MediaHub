Findings (outstanding)

High: library drag max still ignores layout mode, so FX mode can regress into overlap.
DJModeV2.tsx (line 370) caps FX library to 180 only on mode-change, but drag uses a global max at DJModeV2.tsx (line 1100) (42vh). In FX mode, user can drag library back up and re-create vertical crowding.

High: new jog side rails can overflow because controls are still horizontal layouts inside a narrow rail.
Rails are fixed w-[120px] at DJModeV2.tsx (line 844) and DJModeV2.tsx (line 1054), but embedded controls are wide rows:
DJBeatJump.tsx (line 58), DJBeatJump.tsx (line 66), DJBeatJump.tsx (line 89), DJLoopSection.tsx (line 57), DJBeatGridEdit.tsx (line 28).
With deck area overflow-hidden (DJModeV2.tsx (line 824), DJModeV2.tsx (line 1051)), this can clip or overlap jog space.

Medium: sampler hide rule is viewport-based, not mixer-space-based.
DJModeV2.tsx (line 994) uses [@media(min-height:800px)], and sampler pads are large (DJSamplerPads.tsx (line 78)). On a 1080 viewport with tall top sections, sampler can still appear when mixer space is actually tight.

Medium: mixer body still hard-clips when space is tight.
DJModeV2.tsx (line 863) uses overflow-hidden for mixer body. This avoids lower controls disappearing, but now upper channel-strip internals can be clipped without fallback.

Low: horizontal pressure still possible at small app widths.
Root keeps min-w-[1024px] (DJModeV2.tsx (line 637)) while each deck now reserves tempo rail + side rail width. On constrained desktop windows, this can still force crowding.