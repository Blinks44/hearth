# Hearth Design System

Hearth uses a warm neutral palette with a single burnt-orange accent (`#E5582B`), native window chrome per OS, and mygo immediate-mode widgets (`internal/ui/`).

## Color Tokens

The dark theme uses warm charcoal grays rather than cool neutral zinc. Accent and brand logo colors stay consistent across both themes.

| Role | Light | Dark | Usage |
| --- | --- | --- | --- |
| Accent | `#E5582B` | `#E5582B` | Primary actions, active highlights, Hearth mark |
| Accent soft / text | `#FDEEE7` / `#B9441F` | `#3A241B` / `#FF8A62` | Project badge, active version row |
| Surface | `#F4F2EF` | `#252320` | Sidebar fallback (when vibrancy/Mica unavailable), recessed chrome |
| Background | `#FFFFFF` | `#1C1A18` | Main content area and cards |
| Subtle / muted | `#FAF9F7` / `#F1EFEB` | `#211F1C` / `#2C2926` | Secondary panels, input fills, neutral pills |
| Border / strong | `#E8E5E0` / `#D9D5CE` | `#34312C` / `#47433D` | Dividers, card outlines |
| Text / secondary / muted | `#1D1B18` / `#69655E` / `#9D988F` | `#F2EFEA` / `#A8A299` / `#7D776E` | Hierarchy copy |
| Mono text | `#4A4640` | `#CFCAC2` | Versions, paths, commands |
| Success / soft / text | `#2E9A5C` / `#E7F5EC` / `#23794A` | `#3DBA72` / `#1A2E21` / `#5FD08E` | On PATH, running, healthy |
| Warning / soft / text | `#C98208` / `#FBF2DE` / `#9A6300` | `#E0A23A` / `#33291A` / `#E3AE4A` | Duplicates, stopped engine, mismatches |
| Info / soft / text | `#3A73DA` / `#E8EFFC` / `#244A8F` | `#5B8EF0` / `#1C2639` / `#8DB4F5` | Scanning, informational banners |
| Danger / soft / text | `#D2453A` / `#FBEAE8` / `#8C2A22` | `#EF5B4F` / `#3B1F1C` / `#FF8A80` | Failed checks, crashed containers, errors |
| Sidebar item: active / hover | `#FFFFFF` + shadow / `#E8E5E0` | `#36332E` + shadow / `#2F2C28` | Custom `ui.ButtonBase` items |
| Dark action button ("Fix...") | `#1D1B18` (white text) | `#EDE9E3` (`#1C1A18` text) | High-contrast secondary action |

## Typography & Icons

- **UI font:** Inter (SIL OFL 1.1), embedded via `ui.RegisterFont`.
- **Monospace font:** JetBrains Mono (SIL OFL 1.1) for versions, file paths, commands, ports, and diffs.
- **Icons (`resources/icons/`):**
  - `lucide/` - UI icons from Lucide (ISC).
  - `brand/` - monochrome brand marks for the sidebar (Simple Icons, CC0).
  - `logo/` - full-color brand tiles (with `<slug>-dark.svg` variants for dark mode) from theSVG (MIT) and Simple Icons.
  - Full attribution and licenses are maintained in [resources/icons/LICENSES.md](../resources/icons/LICENSES.md) and [resources/NOTICE.txt](../resources/NOTICE.txt).

## Window Chrome & Layout

- Default window size: `1280x820`, minimum supported layout: `1000x680` (verified in UI tests).
- Sidebar width: `~244 px` with right scrollbar clearance so counters, version numbers, and status dots never overlap scrollbars. Content padding: `28 px`.
- **Platform chrome (`c.TitleBar()`):**
  - **macOS:** `TitleBarHidden`, traffic lights above the sidebar (`52 px` top clearance), `VibrancySidebar`.
  - **Windows 11:** Logo starts near top (`14 px`); system caption buttons sit on the top right (`c.TitleBar().Right + 12` header clearance), Mica sidebar on 22H2+ (Surface fallback on Windows 10).
  - **Linux:** `TitleBarHeight: 56`, respects `gtk-decoration-layout`, Surface sidebar fill.

## Recurring UI Patterns

- **Three distinct status pills:** `installed`, `in PATH` / `in terminal`, and `in project` are always separate visual badges.
- **Operation dialog ("What will change"):** Shows exact commands, file diffs (`ui.DiffViewer`), administrator or third-party license notes, and explicit confirmation before running. Long operations show live streaming output and can be minimized to a background progress toast.
- **Table row issue indicators (`internal/ui/row_issue.go`):** Docker and Kubernetes tables keep uniform `44 px` row height. Row-level warnings render as a `20x20` icon badge next to the status pill; hover shows a tooltip, click opens a popover with copyable text and contextual actions.
- **Quick row actions & bulk bar:** Frequent actions (`Play/Stop`, `Logs`, `Shell`, `...`) align in fixed right-hand columns (`24x24` icon buttons). Multi-row selection in Docker and Kubernetes opens a centered floating dark bulk bar (`floatingBulkBar`) at the bottom.
- **Live cluster & engine views:** Open with a back arrow (`24x24`) + breadcrumb selector (`context / namespace / kind` with `Cmd+J` / `Ctrl+J` resource switcher) and freshness indicator. When the selected Docker engine responds or a Kubernetes cluster is active, clicking the sidebar item resumes the last view directly.
- **Motion (`internal/ui/motion.go`):** Short state-driven transitions (tool card reveal over skeletons, hover fills, cleanup row collapse, stepper slides, progress bar transitions) that automatically degrade to opacity/color only when the OS "Reduce Motion" preference is enabled.
