# SpotifyGo Architecture

## 1. System Overview

SpotifyGo is a modern, high-performance terminal music player and client for Spotify built with Go. It combines an embedded audio engine (backed by `librespot`) with an interactive terminal interface (TUI) powered by Charm's Bubble Tea and Lip Gloss.

```
┌─────────────────────────────────────────────────────────────────┐
│                       SpotifyGo Terminal UI                     │
│    (Bubble Tea v2 Model-Update-View + Lip Gloss + ANSI TrueColor)│
├──────────────────────────────┬──────────────────────────────────┤
│       Now-Playing Card       │      Interactive Views           │
│   - TrueColor Album Cover    │      - Playlists / Liked Tracks  │
│   - Live Progress & Fuse     │      - Track Browser             │
│   - Audio Waveform Meter     │      - Fuzzy Search & Palette    │
│   - Album & Year Metadata    │      - Zen Sanctuary (z)         │
├──────────────────────────────┴──────────────────────────────────┤
│                        Core Subsystems                          │
├──────────────────────────────┬──────────────────────────────────┤
│      Visual & Theme Engine   │      Spotify API Client          │
│   - Cover Palette Extractor  │   - OAuth 2.0 PKCE (Local Server)│
│   - Static Normal Canvas     │   - 2-Tier Metadata Cache        │
│   - Half-block ANSI (▀)      │   - Anti-429 Rate Limiter        │
│   - LRU Memory Eviction      │   - Background Fetch Dispatcher  │
├──────────────────────────────┼──────────────────────────────────┤
│      Audio Engine Daemon     │      Eco-Power & Focus Monitor   │
│   - librespot Process Pipe   │   - Terminal Focus (xterm 1004h) │
│   - Bitrate Selector (96-320)│   - Zero-Idle (0.0% CPU unfocused)│
│   - Gapless Prefetch Arbiter │   - Non-blocking Event Loop      │
└──────────────────────────────┴──────────────────────────────────┘
```

---

## 2. Key Architecture Components

### 2.1 Terminal User Interface (`internal/ui`)
- **Framework:** Charm's Bubble Tea v2 following The Elm Architecture (`Model`, `Update`, `View`).
- **Styling:** Charm Lip Gloss with 24-bit TrueColor support.
- **Views:**
  - `viewPlaylists`: User playlists and followed collections (auto-prepends "Canciones que te gustan").
  - `viewTracks`: Track list within a playlist with live play indicators.
  - `viewSearch`: Real-time fuzzy search across playlists and tracks.
  - `viewDevices`: Spotify Connect device selector and transfer.
  - `viewCoverArt`: Zen Mode with large album art and shimmering starry sky.
  - `viewCommand`: Utility command palette triggered with `/`, matched by command-name and alias prefixes.
- **Independent visual clocks:** Normal mode has no decorative animation loop; playback position refreshes once per second. Zen uses a 120 ms clock for stars, flow, border, and fuse, plus a 33 ms (~30 FPS) clock for its one-second cover/text entrance. Leaving Zen or losing terminal focus invalidates pending ticks.

### 2.2 Audio Engine Daemon (`internal/player`)
- **Process Management:** Launches and manages `librespot` as an isolated subprocess.
- **Event Parsing:** Monitors stdout/stderr for track transitions, volume events, and connection status.
- **Gapless Prefetch Arbiter:** Prevents UI desynchronization when `librespot` pre-loads the next song 20-30s before the current track finishes.
- **Hot Restart:** Restarts the audio engine cleanly when its playback configuration changes, without leaking channels or panicking.

### 2.3 Spotify Web API & Auth (`internal/spotify`)
- **OAuth 2.0 PKCE:** Secure authentication without requiring client secrets. Captures authorization code via a temporary local loopback server (`127.0.0.1:8989`).
- **Two-Tier Metadata Cache:**
  - Tier 1 (RAM): Instant lookup (0.0 ms) for recently played and loaded songs.
  - Tier 2 (Disk): JSON cache stored in `%LOCALAPPDATA%\SpotifyGo\cache\tracks\`.
- **Anti-429 Rate Limiting:** Debounces user actions (volume adjustments, rapid navigation) to prevent HTTP 429 (Too Many Requests).
- **Adaptive Playback Sync:** Spotify exposes no push channel for remote playback (librespot only reports its own audio), so the UI polls `/me/player` with a state-aware cadence: a boundary probe right after the current track is expected to end (3-10s window), a safety-net interval while the local device plays, lazy polling when paused, and throttling when the window is unfocused. Any keystroke on a state older than 3s refreshes immediately, so a track changed on the phone appears within seconds instead of the previous fixed 25s (100s with the local device active).

### 2.4 TrueColor Rendering & Album Palette (`internal/ui/cover.go`)
- **Half-block ANSI (`▀`):** Packs two vertical pixels per terminal cell (foreground = top pixel, background = bottom pixel), producing crisp album art within compact terminal cells.
- **Palette Extraction:** Analyzes album art to extract dominant and secondary colors while rejecting low-contrast or monochrome washouts.
- **Fast Bitwise Hex Parser (`parseHexRGB`):** Zero-allocation bitwise RGB parser executing in ~1 ns for high-frame-rate gradient blending.
- **LRU Cache:** Bounds in-memory cover bitmaps to a maximum of 25 items (`MaxMemoryCovers = 25`).

### 2.5 Theme-Independent Background Canvas (`internal/ui/background.go`)
- **Owner of the Canvas:** `NewCanvas` + `PaintLines` render static normal backgrounds and Zen's animated `flow` canvas.
- **Mode Isolation:** normal mode never advances a decorative frame; its `flow` preference falls back to the static album gradient. Zen alone uses the 120 ms frame clock for `flow` and its starry sky. Only `dark` consumes `theme.BgBase`.
- **Theme Isolation:** the album-derived `flow` and `gradient` palettes use a bounded blend of 30-42% toward a private floor color, so switching themes restyles text, accents, borders and pills without recoloring Zen's animation.
- **WCAG Contrast Guarantee:** a fixed luminance ceiling (0.10) caps every canvas row, keeping the palette's light text readable (>= 4.5:1 for both text and muted) while `ensureContrast` darkens a hue by bisection when needed, preserving saturation.
- **Band-Free Canvas:** the ceiling is applied identically to every row. An earlier revision scrimmed only the rows carrying UI content, which kept text legible but painted a visible dark band behind the now-playing card and the playlist list; the uniform rule keeps the same contrast guarantee with a single continuous gradient.

### 2.6 Spanish UI Copy, Command Palette & Self-Diagnosis
- **Centralized UI copy (`internal/ui/voice.go`):** Spanish toast and status wording is kept together so playback feedback and background labels stay consistent.
- **Focused command palette (`internal/ui/command.go`):** `/` contains only utility actions without footer shortcuts; their descriptions are in Spanish, and footer actions have no hidden palette resolvers.
- **`spotifygo doctor`:** read-only diagnosis of the Spotify link (profile, granted scopes, current playback, artist genres, audio-features availability and stored preferences), used to decide feature fallbacks with real data instead of assumptions.

### 2.7 Eco-Power & Resource Efficiency
- **Terminal Focus Detection:** Leverages xterm focus reporting (`\x1b[?1004h`) via `view.ReportFocus = true`.
- **Zero-Idle:** Pauses UI tick goroutines when the terminal window loses focus, maintaining 0.0% CPU usage while keeping audio playback alive.

---

## 3. Directory Layout

```
SpotyGo/
├── assets/                  # High-resolution screenshots and animated intro GIF
├── docs/                    # Technical architecture and documentation
├── internal/
│   ├── logger/              # Thread-safe rotating logger (5 MB threshold)
│   ├── player/              # librespot process manager and event streaming
│   ├── spotify/             # OAuth PKCE auth and cached Spotify Web API client
│   ├── theme/               # Theme palettes and persistent JSON configuration
│   └── ui/                  # Bubble Tea model, views, ASCII art, and cover renderer
├── scripts/
│   ├── install.ps1          # One-line installer script for Windows
│   └── generate_intro_gif.py# Asset generation tool for README media
├── main.go                  # CLI entrypoint and lifecycle coordinator
├── go.mod                   # Module definition and dependencies
├── LICENSE                  # MIT Open Source License
└── README.md                # Project documentation and quickstart
```
