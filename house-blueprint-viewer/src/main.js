import * as THREE from 'three';
import { buildHouse, setFloorVisibility, setCeilingVisible } from './houseBuilder.js';
import { ViewControls } from './controls.js';
import { initShare, parseUrlState } from './share.js';
import { GROUND_ROOMS, FIRST_ROOMS, PRESETS, FT, roomCenter } from './floorPlan.js';

// ── Scene setup ──────────────────────────────────────────────────
const canvas = document.getElementById('canvas');
const renderer = new THREE.WebGLRenderer({ canvas, antialias: true });
renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
renderer.setSize(window.innerWidth, window.innerHeight);
renderer.shadowMap.enabled = true;
renderer.shadowMap.type = THREE.PCFSoftShadowMap;
renderer.setClearColor(0x87b8d8);

const scene = new THREE.Scene();
scene.fog = new THREE.Fog(0x87b8d8, 30, 80);

const camera = new THREE.PerspectiveCamera(70, window.innerWidth / window.innerHeight, 0.05, 200);
camera.position.set(9 * FT, 5.5 * FT + 0.33 * FT, 21 * FT);

// Lighting
const hemi = new THREE.HemisphereLight(0xfff8f0, 0x8a9ab0, 0.65);
scene.add(hemi);

const sun = new THREE.DirectionalLight(0xfff5e8, 1.1);
sun.position.set(15, 25, 10);
sun.castShadow = true;
sun.shadow.mapSize.set(2048, 2048);
sun.shadow.camera.near = 1;
sun.shadow.camera.far = 60;
sun.shadow.camera.left = -15;
sun.shadow.camera.right = 15;
sun.shadow.camera.top = 15;
sun.shadow.camera.bottom = -15;
scene.add(sun);

// Ground plane (yard)
const yardGeo = new THREE.PlaneGeometry(60, 60);
const yardMat = new THREE.MeshStandardMaterial({ color: 0x7aab6a, roughness: 1 });
const yard = new THREE.Mesh(yardGeo, yardMat);
yard.rotation.x = -Math.PI / 2;
yard.position.set(9 * FT, -0.02, 15 * FT);
yard.receiveShadow = true;
scene.add(yard);

// House
const house = buildHouse();
scene.add(house);

// Controls
const viewControls = new ViewControls(camera, canvas, house);
initShare(viewControls);

let floorMode = 'ground';
let ceilingVisible = false;

// ── UI wiring ────────────────────────────────────────────────────
function populateRoomList() {
  const list = document.getElementById('room-list');
  const rooms = floorMode === 'first' ? FIRST_ROOMS : floorMode === 'both' ? [...GROUND_ROOMS, ...FIRST_ROOMS] : GROUND_ROOMS;
  list.innerHTML = rooms.map((r) =>
    `<li><button class="room-btn" data-room="${r.id}">${r.name}</button></li>`
  ).join('');

  list.querySelectorAll('.room-btn').forEach((btn) => {
    btn.addEventListener('click', () => {
      const room = rooms.find((r) => r.id === btn.dataset.room);
      if (!room) return;
      const c = roomCenter(room);
      const floor = room.floor === 'first' ? 1 : 0;
      viewControls.setMode('walk');
      viewControls.teleport(c.x, c.z, 0, floor);
      highlightRoom(room.id);
    });
  });
}

function highlightRoom(id) {
  document.querySelectorAll('.room-btn').forEach((b) => {
    b.classList.toggle('active', b.dataset.room === id);
  });
  const room = [...GROUND_ROOMS, ...FIRST_ROOMS].find((r) => r.id === id);
  const label = document.getElementById('room-label');
  if (room) {
    label.textContent = room.name;
    label.classList.remove('hidden');
    setTimeout(() => label.classList.add('hidden'), 2500);
  }
}

document.querySelectorAll('.floor-tab').forEach((tab) => {
  tab.addEventListener('click', () => {
    document.querySelectorAll('.floor-tab').forEach((t) => t.classList.remove('active'));
    tab.classList.add('active');
    floorMode = tab.dataset.floor;
    setFloorVisibility(house, floorMode);
    populateRoomList();
  });
});

document.querySelectorAll('.mode-tab').forEach((tab) => {
  tab.addEventListener('click', () => {
    document.querySelectorAll('.mode-tab').forEach((t) => t.classList.remove('active'));
    tab.classList.add('active');
    const mode = tab.dataset.mode;
    viewControls.setMode(mode);
    document.getElementById('controls-walk').classList.toggle('hidden', mode !== 'walk');
    document.getElementById('controls-orbit').classList.toggle('hidden', mode === 'walk');
    document.getElementById('crosshair').classList.toggle('hidden', mode !== 'walk');
  });
});

canvas.addEventListener('click', () => {
  if (viewControls.mode === 'walk') viewControls.requestLock();
});

document.getElementById('btn-fullscreen')?.addEventListener('click', () => {
  if (!document.fullscreenElement) {
    document.documentElement.requestFullscreen();
  } else {
    document.exitFullscreen();
  }
});

document.addEventListener('keydown', (e) => {
  if (e.code === 'KeyC') {
    ceilingVisible = !ceilingVisible;
    setCeilingVisible(house, !ceilingVisible);
  }
});

// ── URL state on load ────────────────────────────────────────────
const urlState = parseUrlState();
if (urlState) {
  viewControls.applyState(urlState);
  if (urlState.mode) {
    document.querySelectorAll('.mode-tab').forEach((t) => {
      t.classList.toggle('active', t.dataset.mode === urlState.mode);
    });
    document.getElementById('controls-walk').classList.toggle('hidden', urlState.mode !== 'walk');
    document.getElementById('controls-orbit').classList.toggle('hidden', urlState.mode === 'walk');
  }
  if (urlState.floor != null) {
    floorMode = urlState.floor === 1 ? 'first' : 'ground';
    document.querySelectorAll('.floor-tab').forEach((t) => {
      t.classList.toggle('active', t.dataset.floor === floorMode);
    });
    setFloorVisibility(house, floorMode);
  }
} else {
  setFloorVisibility(house, 'ground');
}

populateRoomList();

// ── Render loop ──────────────────────────────────────────────────
const clock = new THREE.Clock();

function animate() {
  requestAnimationFrame(animate);
  const delta = Math.min(clock.getDelta(), 0.05);
  viewControls.update(delta);
  renderer.render(scene, camera);
}
animate();

window.addEventListener('resize', () => {
  camera.aspect = window.innerWidth / window.innerHeight;
  camera.updateProjectionMatrix();
  renderer.setSize(window.innerWidth, window.innerHeight);
});

// Expose for debugging
window.__houseViewer = { scene, house, viewControls, PRESETS, FT };
