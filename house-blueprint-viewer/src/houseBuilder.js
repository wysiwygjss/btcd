import * as THREE from 'three';
import {
  FT, WALL_H, WALL_T, FLOOR_T,
  HOUSE_W, HOUSE_D,
  GROUND_ROOMS, GROUND_WALLS,
  FIRST_ROOMS, FIRST_WALLS, FIRST_OPEN_DEPTH,
  feetX, feetZ,
} from './floorPlan.js';

const WALL_COLOR = 0xf5f0e8;
const FLOOR_COLOR = 0xddd5c8;
const TRIM_COLOR = 0x8a7e6e;

function makeMaterial(color, opts = {}) {
  return new THREE.MeshStandardMaterial({
    color,
    roughness: 0.85,
    metalness: 0.02,
    ...opts,
  });
}

function buildWallSegment(x, z, length, axis, yBase, height, doorAt, doorW) {
  const group = new THREE.Group();
  const mat = makeMaterial(WALL_COLOR);
  const h = height;

  const makeBox = (lx, lz, px, pz) => {
    const geo = new THREE.BoxGeometry(lx, h, lz);
    const mesh = new THREE.Mesh(geo, mat);
    mesh.position.set(px, yBase + h / 2, pz);
    mesh.castShadow = true;
    mesh.receiveShadow = true;
    return mesh;
  };

  const t = WALL_T;
  if (doorAt != null && doorW > 0) {
    const before = doorAt - doorW / 2;
    const after = length - doorAt - doorW / 2;
    if (axis === 'z') {
      if (before > 0.1) group.add(makeBox(t, before * FT, feetX(x) + t / 2, feetZ(z) + before * FT / 2));
      if (after > 0.1) group.add(makeBox(t, after * FT, feetX(x) + t / 2, feetZ(z) + (doorAt + doorW / 2) * FT + after * FT / 2));
      // lintel above door
      const lintel = makeBox(t, doorW * FT, feetX(x) + t / 2, feetZ(z) + doorAt * FT);
      lintel.scale.y = 0.35;
      lintel.position.y = yBase + h * 0.85;
      group.add(lintel);
    } else {
      if (before > 0.1) group.add(makeBox(before * FT, t, feetX(x) + before * FT / 2, feetZ(z) + t / 2));
      if (after > 0.1) group.add(makeBox(after * FT, t, feetX(x) + (doorAt + doorW / 2) * FT + after * FT / 2, feetZ(z) + t / 2));
      const lintel = makeBox(doorW * FT, t, feetX(x) + doorAt * FT, feetZ(z) + t / 2);
      lintel.scale.y = 0.35;
      lintel.position.y = yBase + h * 0.85;
      group.add(lintel);
    }
  } else {
    if (axis === 'z') {
      group.add(makeBox(t, length * FT, feetX(x) + t / 2, feetZ(z) + length * FT / 2));
    } else {
      group.add(makeBox(length * FT, t, feetX(x) + length * FT / 2, feetZ(z) + t / 2));
    }
  }
  return group;
}

function buildOuterWalls(yBase, height, openFrontZ, openBackZ) {
  const group = new THREE.Group();
  const mat = makeMaterial(WALL_COLOR);
  const h = height;
  const w = HOUSE_W * FT;
  const d = HOUSE_D * FT;
  const t = WALL_T;

  const wall = (lx, lz, px, pz) => {
    const geo = new THREE.BoxGeometry(lx, h, lz);
    const mesh = new THREE.Mesh(geo, mat);
    mesh.position.set(px, yBase + h / 2, pz);
    mesh.castShadow = true;
    mesh.receiveShadow = true;
    group.add(mesh);
  };

  // Back wall (or open if porch above)
  if (!openBackZ) {
    wall(w, t, w / 2, t / 2);
  } else {
    // Side portions of back wall around open porch
    wall(t, openBackZ, t / 2, openBackZ / 2);
    wall(t, openBackZ, w - t / 2, openBackZ / 2);
  }

  // Left wall
  wall(t, d, t / 2, d / 2);
  // Right wall
  wall(t, d, w - t / 2, d / 2);

  // Front wall (with porch opening)
  if (openFrontZ) {
    const porchDepth = openFrontZ;
    const upperStart = porchDepth;
    const upperLen = d - porchDepth;
    if (upperLen > 0.1) wall(w, t, w / 2, upperStart + upperLen / 2);
    // porch side rails
    wall(t, porchDepth, t / 2, porchDepth / 2);
    wall(t, porchDepth, w - t / 2, porchDepth / 2);
  } else {
    wall(w, t, w / 2, d - t / 2);
  }

  return group;
}

function buildFloorSlab(rooms, yBase, excludeOpen = true) {
  const group = new THREE.Group();
  for (const room of rooms) {
    if (excludeOpen && room.open && !room.balcony) continue;
    if (room.balcony) continue;
    const geo = new THREE.BoxGeometry(room.w * FT, FLOOR_T, room.d * FT);
    const mat = makeMaterial(room.color);
    const mesh = new THREE.Mesh(geo, mat);
    mesh.position.set(
      feetX(room.x + room.w / 2),
      yBase - FLOOR_T / 2,
      feetZ(room.z + room.d / 2),
    );
    mesh.receiveShadow = true;
    mesh.userData.roomId = room.id;
    mesh.userData.roomName = room.name;
    group.add(mesh);

    // Floor label plane (subtle)
    const labelGeo = new THREE.PlaneGeometry(room.w * FT * 0.6, room.d * FT * 0.25);
    const labelMat = makeMaterial(room.color, { transparent: true, opacity: 0.0 });
    const label = new THREE.Mesh(labelGeo, labelMat);
    label.rotation.x = -Math.PI / 2;
    label.position.set(
      feetX(room.x + room.w / 2),
      yBase + 0.01,
      feetZ(room.z + room.d / 2),
    );
    label.userData.roomId = room.id;
    label.userData.roomName = room.name;
    group.add(label);
  }
  return group;
}

function buildCeiling(width, depth, y) {
  const geo = new THREE.BoxGeometry(width * FT, FLOOR_T * 0.5, depth * FT);
  const mat = makeMaterial(0xfaf8f4, { side: THREE.DoubleSide });
  const mesh = new THREE.Mesh(geo, mat);
  mesh.position.set(feetX(width / 2), y, feetZ(depth / 2));
  return mesh;
}

function buildStairs(yBase) {
  const group = new THREE.Group();
  const mat = makeMaterial(TRIM_COLOR);
  const steps = 14;
  const totalRise = WALL_H;
  const totalRun = 6.5 * FT;
  const stepRise = totalRise / steps;
  const stepRun = totalRun / steps;
  const startX = feetX(2);
  const startZ = feetZ(FIRST_OPEN_DEPTH + 14);

  for (let i = 0; i < steps; i++) {
    const geo = new THREE.BoxGeometry(4 * FT, stepRise, stepRun);
    const mesh = new THREE.Mesh(geo, mat);
    mesh.position.set(
      startX,
      yBase + stepRise * (i + 0.5),
      startZ - stepRun * (i + 0.5),
    );
    mesh.castShadow = true;
    mesh.receiveShadow = true;
    group.add(mesh);
  }
  return group;
}

function buildPorchRailings(yBase, zStart, depth) {
  const group = new THREE.Group();
  const mat = makeMaterial(TRIM_COLOR);
  const h = 3 * FT;
  const postGeo = new THREE.BoxGeometry(0.15, h, 0.15);
  const spacing = 3 * FT;
  const w = HOUSE_W * FT;

  for (let x = 0; x <= w; x += spacing) {
    for (const zOff of [0, depth * FT]) {
      const post = new THREE.Mesh(postGeo, mat);
      post.position.set(x, yBase + h / 2, feetZ(zStart) + zOff);
      group.add(post);
    }
  }

  // Rails
  const railGeo = new THREE.BoxGeometry(w, 0.08, 0.08);
  for (const yOff of [0.4, 0.8]) {
    const frontRail = new THREE.Mesh(railGeo, mat);
    frontRail.position.set(w / 2, yBase + h * yOff, feetZ(zStart) + depth * FT);
    group.add(frontRail);
  }
  return group;
}

function buildFurniture(yBase) {
  const group = new THREE.Group();
  const wood = makeMaterial(0xa08060);
  const fabric = makeMaterial(0x6a8caf);

  // Kitchen counter
  const counter = new THREE.Mesh(
    new THREE.BoxGeometry(5 * FT, 3 * FT, 1.5 * FT),
    wood,
  );
  counter.position.set(feetX(14), yBase + 1.5 * FT, feetZ(5));
  group.add(counter);

  // Ground bedroom bed
  const bed = new THREE.Mesh(new THREE.BoxGeometry(5 * FT, 1.5 * FT, 6.5 * FT), fabric);
  bed.position.set(feetX(5), yBase + 0.75 * FT, feetZ(11));
  group.add(bed);

  // Sitting room sofa
  const sofa = new THREE.Mesh(new THREE.BoxGeometry(6 * FT, 2 * FT, 2.5 * FT), fabric);
  sofa.position.set(feetX(5), yBase + 1 * FT, feetZ(20));
  group.add(sofa);

  // First floor living sofa
  const sofa2 = sofa.clone();
  sofa2.position.set(feetX(14), yBase + WALL_H + 1 * FT, feetZ(18));
  group.add(sofa2);

  return group;
}

export function buildHouse() {
  const house = new THREE.Group();
  house.name = 'house';

  const groundY = FLOOR_T;
  const firstY = WALL_H + FLOOR_T * 2;

  // ── Ground floor ──
  const groundGroup = new THREE.Group();
  groundGroup.name = 'ground-floor';
  groundGroup.add(buildFloorSlab(GROUND_ROOMS, groundY));
  groundGroup.add(buildOuterWalls(groundY, WALL_H, 3.25 * FT, 0));
  for (const w of GROUND_WALLS) {
    groundGroup.add(buildWallSegment(w.x, w.z, w.len, w.axis, groundY, WALL_H, w.doorAt, w.doorW));
  }
  groundGroup.add(buildCeiling(HOUSE_W, HOUSE_D - 3.25, groundY + WALL_H));
  groundGroup.add(buildPorchRailings(groundY, 26.75, 3.25));
  house.add(groundGroup);

  // ── First floor ──
  const firstGroup = new THREE.Group();
  firstGroup.name = 'first-floor';
  firstGroup.add(buildFloorSlab(FIRST_ROOMS, firstY));
  firstGroup.add(buildOuterWalls(firstY, WALL_H, 0, FIRST_OPEN_DEPTH * FT));
  for (const w of FIRST_WALLS) {
    firstGroup.add(buildWallSegment(w.x, w.z, w.len, w.axis, firstY, WALL_H, w.doorAt, w.doorW));
  }
  firstGroup.add(buildCeiling(HOUSE_W, HOUSE_D, firstY + WALL_H));
  firstGroup.add(buildPorchRailings(firstY, 0, FIRST_OPEN_DEPTH));
  firstGroup.add(buildStairs(groundY));
  house.add(firstGroup);

  house.add(buildFurniture(groundY));

  // Room metadata for labels
  house.userData.rooms = [...GROUND_ROOMS, ...FIRST_ROOMS];

  return house;
}

export function setFloorVisibility(house, mode) {
  const ground = house.getObjectByName('ground-floor');
  const first = house.getObjectByName('first-floor');
  if (!ground || !first) return;

  ground.visible = mode === 'ground' || mode === 'both';
  first.visible = mode === 'first' || mode === 'both';

  // Ghost upper floor in "both" mode
  if (mode === 'both') {
    first.traverse((child) => {
      if (child.isMesh && child.material) {
        child.material = child.material.clone();
        child.material.transparent = true;
        child.material.opacity = 0.55;
      }
    });
  }
}

export function setCeilingVisible(house, visible) {
  house.traverse((child) => {
    if (child.isMesh) {
      const geo = child.geometry;
      if (geo && Math.abs(geo.parameters?.height - FLOOR_T * 0.5) < 0.01) {
        child.visible = visible;
      }
    }
  });
}
