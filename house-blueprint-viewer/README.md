# House Blueprint 3D Walkthrough

Interactive 3D visualization of the **18′ × 30′** house from blueprints A03 (Ground Floor) and A05 (First Floor).

## Features

- **3D floor plans** built from blueprint dimensions
- **First-person walkthrough** — click the canvas, then use WASD + mouse
- **Orbit & top-down** viewing modes
- **Floor switcher** — Ground, First, or both floors (ghosted)
- **Room navigation** — jump to any room from the sidebar
- **Easy sharing** — Share button copies a link with your current camera position; includes QR code and quick-view presets

## Controls

| Key | Action |
|-----|--------|
| W A S D | Move |
| Mouse | Look around |
| Shift | Run |
| Space | Switch floor (walk mode) |
| C | Toggle ceiling |
| Esc | Release mouse |

## Run locally

```bash
cd house-blueprint-viewer
npm install
npm run dev
```

Open the URL shown in the terminal (default `http://localhost:5173`).

## Build for deployment

```bash
npm run build
npm run preview
```

Deploy the `dist/` folder to any static host (Netlify, Vercel, GitHub Pages, etc.). Share links work because view state is encoded in the URL hash.

## Blueprint source

- **A03** — Ground Floor Plan (Bathroom, Bedroom, Sitting Room, Kitchen, Front Porch)
- **A05** — First Floor Plan (Bedroom, Washroom, Living Room, Upper Porch, Stairs)
