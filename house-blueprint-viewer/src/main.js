import * as THREE from 'three';
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js';
import { buildHouse, setFloorVisibility } from './houseBuilder.js';
import { GROUND_ROOMS, FIRST_ROOMS, FT, roomCenter } from './floorPlan.js';

// ── Scene ────────────────────────────────────────────────────────
const canvas = document.getElementById('canvas');
const renderer = new THREE.WebGLRenderer({ canvas, antialias: true });
renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
renderer.setSize(window.innerWidth, window.innerHeight);
renderer.shadowMap.enabled = true;
renderer.setClearColor(0x87b8d8);

const scene = new THREE.Scene();
scene.fog = new THREE.Fog(0x87b8d8, 30, 80);

const camera = new THREE.PerspectiveCamera(65, window.innerWidth / window.innerHeight, 0.05, 200);
const EYE = 5.5 * FT;

const hemi = new THREE.HemisphereLight(0xfff8f0, 0x8a9ab0, 0.7);
scene.add(hemi);
const sun = new THREE.DirectionalLight(0xfff5e8, 1.0);
sun.position.set(12, 20, 8);
scene.add(sun);

const yard = new THREE.Mesh(
  new THREE.PlaneGeometry(60, 60),
  new THREE.MeshStandardMaterial({ color: 0x7aab6a }),
);
yard.rotation.x = -Math.PI / 2;
yard.position.set(9 * FT, -0.02, 15 * FT);
scene.add(yard);

const house = buildHouse();
scene.add(house);
setFloorVisibility(house, 'ground');

// Simple orbit controls — drag to look, no pointer-lock needed
const controls = new OrbitControls(camera, canvas);
controls.enableDamping = true;
controls.dampingFactor = 0.1;
controls.maxPolarAngle = Math.PI / 2.05;
controls.minDistance = 2;
controls.maxDistance = 30;
controls.enablePan = false;

let floorLevel = 0; // 0 ground, 1 first
let floorMode = 'ground';

function eyeY() {
  return (floorLevel === 0 ? 0.33 * FT : 8.33 * FT) + EYE;
}

function setCamera(x, z, lookX, lookZ) {
  camera.position.set(x, eyeY(), z);
  controls.target.set(lookX ?? x, eyeY() - 1, lookZ ?? z - 2);
  controls.update();
}

setCamera(9 * FT, 22 * FT);

// ── Movement via d-pad ───────────────────────────────────────────
const keys = {};
const MOVE = 2.5;

function bindBtn(id, code) {
  const btn = document.getElementById(id);
  if (!btn) return;
  const down = () => { keys[code] = true; btn.classList.add('pressed'); };
  const up = () => { keys[code] = false; btn.classList.remove('pressed'); };
  btn.addEventListener('mousedown', down);
  btn.addEventListener('mouseup', up);
  btn.addEventListener('mouseleave', up);
  btn.addEventListener('touchstart', (e) => { e.preventDefault(); down(); });
  btn.addEventListener('touchend', up);
}

bindBtn('btn-up', 'fwd');
bindBtn('btn-down', 'back');
bindBtn('btn-left', 'left');
bindBtn('btn-right', 'right');

function movePlayer(dt) {
  if (!keys.fwd && !keys.back && !keys.left && !keys.right) return;
  const speed = MOVE * dt;
  const dir = new THREE.Vector3();
  camera.getWorldDirection(dir);
  dir.y = 0;
  dir.normalize();
  const right = new THREE.Vector3().crossVectors(dir, new THREE.Vector3(0, 1, 0));

  if (keys.fwd) camera.position.addScaledVector(dir, speed);
  if (keys.back) camera.position.addScaledVector(dir, -speed);
  if (keys.left) camera.position.addScaledVector(right, -speed);
  if (keys.right) camera.position.addScaledVector(right, speed);

  // Keep inside house
  camera.position.x = THREE.MathUtils.clamp(camera.position.x, 0.5, 17.5 * FT);
  camera.position.z = THREE.MathUtils.clamp(camera.position.z, 0.5, 29.5 * FT);
  camera.position.y = eyeY();
  controls.target.y = eyeY() - 1;
}

// ── Room navigation ──────────────────────────────────────────────
function showRoom(name) {
  const banner = document.getElementById('room-banner');
  banner.textContent = name;
  banner.classList.remove('hidden');
  banner.style.opacity = '1';
  clearTimeout(showRoom._t);
  showRoom._t = setTimeout(() => { banner.style.opacity = '0'; }, 3000);
}

function goToRoom(room) {
  const c = roomCenter(room);
  floorLevel = room.floor === 'first' ? 1 : 0;
  floorMode = floorLevel === 0 ? 'ground' : 'first';
  setFloorVisibility(house, floorMode);
  updateFloorBtn();
  setCamera(c.x, c.z + 1 * FT, c.x, c.z - 1 * FT);
  showRoom(room.name);
  document.querySelectorAll('.room-chip').forEach((b) => {
    b.classList.toggle('active', b.dataset.room === room.id);
  });
}

function buildRoomButtons() {
  const row = document.getElementById('room-buttons');
  const rooms = floorLevel === 0 ? GROUND_ROOMS.filter((r) => !r.open) : FIRST_ROOMS.filter((r) => !r.open && !r.balcony);
  row.innerHTML = rooms.map((r) =>
    `<button class="room-chip" data-room="${r.id}">${r.name}</button>`
  ).join('');
  row.querySelectorAll('.room-chip').forEach((btn) => {
    btn.addEventListener('click', () => {
      const room = rooms.find((r) => r.id === btn.dataset.room);
      if (room) goToRoom(room);
    });
  });
}

function updateFloorBtn() {
  const btn = document.getElementById('btn-floor');
  btn.textContent = floorLevel === 0 ? 'Ground Floor' : 'Upstairs';
}

document.getElementById('btn-floor').addEventListener('click', () => {
  floorLevel = floorLevel === 0 ? 1 : 0;
  floorMode = floorLevel === 0 ? 'ground' : 'first';
  setFloorVisibility(house, floorMode);
  updateFloorBtn();
  buildRoomButtons();
  const rooms = floorLevel === 0 ? GROUND_ROOMS : FIRST_ROOMS;
  const porch = rooms.find((r) => r.open);
  if (porch) goToRoom(porch);
  else goToRoom(rooms.find((r) => !r.open));
});

// ── Guided tour ──────────────────────────────────────────────────
const TOUR = [
  ...GROUND_ROOMS.filter((r) => !r.open).map((r) => ({ room: r, pause: 4 })),
  ...FIRST_ROOMS.filter((r) => !r.open && !r.balcony).map((r) => ({ room: r, pause: 4 })),
];

let tourRunning = false;
let tourIdx = 0;
let tourTimer = 0;

function startTour() {
  tourRunning = !tourRunning;
  const btn = document.getElementById('btn-tour');
  if (tourRunning) {
    btn.textContent = '⏸ Pause Tour';
    tourIdx = 0;
    tourTimer = 0;
    visitTourStop();
  } else {
    btn.textContent = '▶ Guided Tour';
  }
}

function visitTourStop() {
  if (!tourRunning || tourIdx >= TOUR.length) {
    tourRunning = false;
    document.getElementById('btn-tour').textContent = '▶ Guided Tour';
    return;
  }
  const { room } = TOUR[tourIdx];
  goToRoom(room);
  tourTimer = 0;
}

document.getElementById('btn-tour').addEventListener('click', startTour);

// ── Start ────────────────────────────────────────────────────────
document.getElementById('btn-start').addEventListener('click', () => {
  document.getElementById('welcome').classList.add('hidden');
  document.getElementById('simple-ui').classList.remove('hidden');
  buildRoomButtons();
  goToRoom(GROUND_ROOMS.find((r) => r.id === 'sitting'));
});

// ── Render loop ──────────────────────────────────────────────────
const clock = new THREE.Clock();

function animate() {
  requestAnimationFrame(animate);
  const dt = Math.min(clock.getDelta(), 0.05);
  movePlayer(dt);
  controls.update();

  if (tourRunning) {
    tourTimer += dt;
    if (tourTimer >= TOUR[tourIdx].pause) {
      tourIdx++;
      visitTourStop();
    }
  }

  renderer.render(scene, camera);
}
animate();

window.addEventListener('resize', () => {
  camera.aspect = window.innerWidth / window.innerHeight;
  camera.updateProjectionMatrix();
  renderer.setSize(window.innerWidth, window.innerHeight);
});
