import * as THREE from 'three';
import { PointerLockControls } from 'three/examples/jsm/controls/PointerLockControls.js';
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js';
import { WALL_H, FLOOR_T, FT, HOUSE_W, HOUSE_D } from './floorPlan.js';

const EYE_H = 5.5 * FT;
const MOVE_SPEED = 4;
const RUN_MULT = 2.2;
const GRAVITY = 20;

export class ViewControls {
  constructor(camera, canvas, house) {
    this.camera = camera;
    this.canvas = canvas;
    this.house = house;
    this.mode = 'walk';
    this.floorLevel = 0; // 0 = ground, 1 = first
    this.velocity = new THREE.Vector3();
    this.direction = new THREE.Vector3();
    this.keys = {};
    this.canJump = false;

    this.pointerLock = new PointerLockControls(camera, canvas);
    this.orbit = new OrbitControls(camera, canvas);
    this.orbit.enableDamping = true;
    this.orbit.dampingFactor = 0.08;
    this.orbit.maxPolarAngle = Math.PI / 2.1;
    this.orbit.target.set((HOUSE_W * FT) / 2, EYE_H, (HOUSE_D * FT) / 2);
    this.orbit.enabled = false;

    this._bindKeys();
    this._bindPointerLock();
  }

  _bindKeys() {
    const setKey = (e, down) => {
      this.keys[e.code] = down;
      if (e.code === 'Space' && down) this._toggleFloor();
    };
    document.addEventListener('keydown', (e) => setKey(e, true));
    document.addEventListener('keyup', (e) => setKey(e, false));
  }

  _bindPointerLock() {
    this.pointerLock.addEventListener('lock', () => {
      document.getElementById('crosshair')?.classList.remove('hidden');
      document.getElementById('status').textContent = 'Walking — Esc to release mouse';
    });
    this.pointerLock.addEventListener('unlock', () => {
      document.getElementById('crosshair')?.classList.add('hidden');
      document.getElementById('status').textContent = 'Click canvas to start walking';
    });
  }

  _toggleFloor() {
    if (this.mode !== 'walk') return;
    this.floorLevel = this.floorLevel === 0 ? 1 : 0;
    const y = this._eyeY();
    this.camera.position.y = y;
    this._updateStatus();
  }

  _eyeY() {
    const base = this.floorLevel === 0 ? FLOOR_T : WALL_H + FLOOR_T * 2;
    return base + EYE_H;
  }

  setMode(mode) {
    this.mode = mode;
    this.pointerLock.unlock();
    this.orbit.enabled = mode === 'orbit' || mode === 'top';

    if (mode === 'walk') {
      this._snapToFloor();
    } else if (mode === 'top') {
      this.camera.position.set(HOUSE_W * FT / 2, 25, HOUSE_D * FT / 2);
      this.orbit.target.set(HOUSE_W * FT / 2, 0, HOUSE_D * FT / 2);
      this.camera.lookAt(this.orbit.target);
    } else if (mode === 'orbit') {
      this.camera.position.set(HOUSE_W * FT * 1.2, 12, HOUSE_D * FT * 1.4);
      this.orbit.target.set(HOUSE_W * FT / 2, WALL_H / 2, HOUSE_D * FT / 2);
    }
    this._updateStatus();
  }

  _snapToFloor() {
    this.camera.position.y = this._eyeY();
  }

  _updateStatus() {
    const el = document.getElementById('status');
    if (!el) return;
    const floorName = this.floorLevel === 0 ? 'Ground' : 'First';
    if (this.mode === 'walk') {
      el.textContent = `${floorName} floor — WASD to move, Space to switch floor`;
    } else if (this.mode === 'orbit') {
      el.textContent = 'Orbit view — drag to rotate, scroll to zoom';
    } else {
      el.textContent = 'Top-down view';
    }
  }

  teleport(x, z, ry, floor = 0) {
    this.floorLevel = floor;
    this.camera.position.set(x, this._eyeY(), z);
    if (ry != null) {
      this.camera.rotation.set(0, ry, 0);
      if (this.mode === 'walk') {
        this.pointerLock.object.rotation.set(0, ry, 0);
      }
    }
    this._updateStatus();
  }

  requestLock() {
    if (this.mode === 'walk') this.pointerLock.lock();
  }

  get isLocked() {
    return this.pointerLock.isLocked;
  }

  update(delta) {
    if (this.mode === 'walk' && this.pointerLock.isLocked) {
      this._updateWalk(delta);
    } else if (this.mode === 'orbit' || this.mode === 'top') {
      this.orbit.update();
    }
  }

  _updateWalk(delta) {
    const speed = (this.keys['ShiftLeft'] || this.keys['ShiftRight'] ? RUN_MULT : 1) * MOVE_SPEED;

    this.velocity.x -= this.velocity.x * 10 * delta;
    this.velocity.z -= this.velocity.z * 10 * delta;
    this.velocity.y -= GRAVITY * delta;

    this.direction.set(0, 0, 0);
    if (this.keys['KeyW']) this.direction.z -= 1;
    if (this.keys['KeyS']) this.direction.z += 1;
    if (this.keys['KeyA']) this.direction.x -= 1;
    if (this.keys['KeyD']) this.direction.x += 1;
    this.direction.normalize();

    if (this.pointerLock.isLocked) {
      const move = new THREE.Vector3();
      move.copy(this.direction);
      move.applyQuaternion(this.camera.quaternion);
      move.y = 0;
      move.normalize();
      this.velocity.x += move.x * speed * delta * 10;
      this.velocity.z += move.z * speed * delta * 10;
    }

    this.pointerLock.moveRight(this.velocity.x * delta);
    this.pointerLock.moveForward(-this.velocity.z * delta);

    // Clamp to house bounds
    const margin = 0.5;
    const px = this.camera.position.x;
    const pz = this.camera.position.z;
    this.camera.position.x = THREE.MathUtils.clamp(px, margin, HOUSE_W * FT - margin);
    this.camera.position.z = THREE.MathUtils.clamp(pz, margin, HOUSE_D * FT - margin);
    this.camera.position.y = this._eyeY();
  }

  getState() {
    return {
      mode: this.mode,
      floor: this.floorLevel,
      x: this.camera.position.x,
      z: this.camera.position.z,
      ry: this.camera.rotation.y,
    };
  }

  applyState(state) {
    if (!state) return;
    if (state.mode) this.setMode(state.mode);
    if (state.floor != null) this.floorLevel = state.floor;
    if (state.x != null) this.teleport(state.x, state.z, state.ry, state.floor ?? this.floorLevel);
  }
}
