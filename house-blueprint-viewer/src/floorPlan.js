// Blueprint dimensions in feet (from A03 & A05)
export const FT = 0.3048; // feet → meters
export const WALL_H = 8 * FT;
export const WALL_T = 0.5 * FT;
export const FLOOR_T = 0.33 * FT;

export const HOUSE_W = 18; // feet
export const HOUSE_D = 30;

// ── Ground Floor (A03) ──────────────────────────────────────────
// Origin: back-left corner. +X = right, +Z = toward front (porch).
export const GROUND_ROOMS = [
  { id: 'bathroom',   name: 'Bathroom',    floor: 'ground', x: 0,  z: 0,     w: 5,    d: 6.75, color: 0xc8e6f0 },
  { id: 'bedroom-g',  name: 'Bedroom',     floor: 'ground', x: 0,  z: 6.75,  w: 11,   d: 10,   color: 0xe8dcc8 },
  { id: 'sitting',    name: 'Sitting Room', floor: 'ground', x: 0,  z: 16.75, w: 11,   d: 10,   color: 0xd4e8d0 },
  { id: 'kitchen',    name: 'Kitchen',     floor: 'ground', x: 11, z: 0,     w: 7,    d: 26.75, color: 0xf0e0c8 },
  { id: 'porch-g',    name: 'Front Porch', floor: 'ground', x: 0,  z: 26.75, w: 18,   d: 3.25, color: 0xd8d0c0, open: true },
];

// Interior walls on ground floor (startX, startZ, length, axis: 'x'|'z', doorAt?, doorW?)
export const GROUND_WALLS = [
  // Outer walls handled separately
  { x: 11, z: 0,    len: 26.75, axis: 'z', doorAt: 13, doorW: 3 },   // kitchen divider
  { x: 0,  z: 6.75, len: 11,    axis: 'x', doorAt: 5,  doorW: 2.5 }, // bath / bedroom
  { x: 0,  z: 16.75,len: 11,    axis: 'x', doorAt: 5,  doorW: 3 },   // bedroom / sitting
  { x: 5,  z: 0,    len: 6.75,  axis: 'z', doorAt: 3,  doorW: 2 },    // bath side wall
];

// ── First Floor (A05) ─────────────────────────────────────────────
export const FIRST_OPEN_DEPTH = 13.5; // porch open-to-below zone

export const FIRST_ROOMS = [
  { id: 'porch-f',    name: 'Upper Porch',  floor: 'first', x: 0,  z: 0,              w: 18, d: 13.5, color: 0xd0c8b8, open: true, balcony: true },
  { id: 'bedroom-f',  name: 'Bedroom',      floor: 'first', x: 0,  z: FIRST_OPEN_DEPTH, w: 11, d: 10,   color: 0xe0d4c0 },
  { id: 'washroom',   name: 'Washroom',     floor: 'first', x: 5,  z: FIRST_OPEN_DEPTH, w: 5,  d: 6.75, color: 0xc0dce8 },
  { id: 'living',     name: 'Living Room',  floor: 'first', x: 11, z: FIRST_OPEN_DEPTH, w: 7,  d: 10,   color: 0xd8e8d4 },
  { id: 'closet',     name: 'Closet',       floor: 'first', x: 5,  z: FIRST_OPEN_DEPTH + 10, w: 5, d: 4.75, color: 0xe8e0d8 },
  { id: 'stairs',     name: 'Stairwell',    floor: 'first', x: 0,  z: FIRST_OPEN_DEPTH + 10, w: 5,  d: 6.5, color: 0xc8c0b8 },
];

export const FIRST_WALLS = [
  { x: 11, z: FIRST_OPEN_DEPTH, len: 14.5, axis: 'z', doorAt: 5, doorW: 3 },
  { x: 0,  z: FIRST_OPEN_DEPTH + 10, len: 11, axis: 'x', doorAt: 2.5, doorW: 2.5 },
  { x: 5,  z: FIRST_OPEN_DEPTH, len: 6.75, axis: 'z', doorAt: 3, doorW: 2 },
  { x: 5,  z: FIRST_OPEN_DEPTH + 6.75, len: 5, axis: 'x' },
  { x: 10, z: FIRST_OPEN_DEPTH + 10, len: 4.75, axis: 'z', doorAt: 2, doorW: 2 },
];

// Spawn points for walkthrough & share presets
export const PRESETS = {
  'ground-porch':   { floor: 0, x: 9,  z: 28,  ry: Math.PI },
  'ground-kitchen': { floor: 0, x: 14, z: 12,  ry: -Math.PI / 2 },
  'ground-bedroom': { floor: 0, x: 5,  z: 11,  ry: 0 },
  'ground-sitting': { floor: 0, x: 5,  z: 21,  ry: 0 },
  'first-bedroom':  { floor: 1, x: 5,  z: 18,  ry: 0 },
  'first-living':   { floor: 1, x: 14, z: 18,  ry: -Math.PI / 2 },
  'first-porch':    { floor: 1, x: 9,  z: 6,   ry: 0 },
};

export function feetX(x) { return x * FT; }
export function feetZ(z) { return z * FT; }
export function roomCenter(room) {
  return {
    x: feetX(room.x + room.w / 2),
    z: feetZ(room.z + room.d / 2),
  };
}
