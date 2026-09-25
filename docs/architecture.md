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
│   - Album & Year Metadata    │      - Zen Sanctuary (/art)      │
├──────────────────────────────┴──────────────────────────────────┤
│                        Core Subsystems                          │
├──────────────────────────────┬──────────────────────────────────┤
│      Visual & Theme Engine   │      Spotify API Client          │
│   - Cover Palette Extractor  │   - OAuth 2.0 PKCE (Local Server)│
│   - Reactive Ambient Flow    │   - 2-Tier Metadata Cache        │
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
  - `viewCommand`: Fuzzy command palette triggered with `/`.

### 2.2 Audio Engine Daemon (`internal/player`)
- **Process Management:** Launches and manages `librespot` as an isolated subprocess.
- **Event Parsing:** Monitors stdout/stderr for track transitions, volume events, and connection status.
- **Gapless Prefetch Arbiter:** Prevents UI desynchronization when `librespot` pre-loads the next song 20-30s before the current track finishes.
- **Hot Restart:** Supports bitrate changes (`96k`, `160k`, `320k`) on the fly via `/quality` without leaking channels or panicking.

### 2.3 Spotify Web API & Auth (`internal/spotify`)
- **OAuth 2.0 PKCE:** Secure authentication without requiring client secrets. Captures authorization code via a temporary local loopback server (`127.0.0.1:8989`).
- **Two-Tier Metadata Cache:**
  - Tier 1 (RAM): Instant lookup (0.0 ms) for recently played and loaded songs.
  - Tier 2 (Disk): JSON cache stored in `%LOCALAPPDATA%\SpotifyGo\cache\tracks\`.
- **Anti-429 Rate Limiting:** Debounces user actions (volume adjustments, rapid navigation) to prevent HTTP 429 (Too Many Requests).

### 2.4 TrueColor Rendering & Reactive Ambient Gradient (`internal/ui/cover.go`)
- **Half-block ANSI (`▀`):** Packs two vertical pixels per terminal cell (foreground = top pixel, background = bottom pixel), producing crisp album art within compact terminal cells.
- **Palette Extraction:** Analyzes album art to extract dominant and secondary colors while rejecting low-contrast or monochrome washouts.
- **Fast Bitwise Hex Parser (`parseHexRGB`):** Zero-allocation bitwise RGB parser executing in ~1 ns for high-frame-rate gradient blending.
- **LRU Cache:** Bounds in-memory cover bitmaps to a maximum of 25 items (`MaxMemoryCovers = 25`).

### 2.5 Eco-Power & Resource Efficiency
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
