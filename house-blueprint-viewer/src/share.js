import { PRESETS, FT } from './floorPlan.js';

function encodeState(state) {
  const payload = {
    m: state.mode?.[0] ?? 'w',
    f: state.floor ?? 0,
    x: Math.round((state.x ?? 0) / FT * 10) / 10,
    z: Math.round((state.z ?? 0) / FT * 10) / 10,
    r: Math.round((state.ry ?? 0) * 100) / 100,
  };
  return btoa(JSON.stringify(payload));
}

function decodeState(hash) {
  try {
    const payload = JSON.parse(atob(hash));
    return {
      mode: { w: 'walk', o: 'orbit', t: 'top' }[payload.m] ?? 'walk',
      floor: payload.f ?? 0,
      x: (payload.x ?? 9) * FT,
      z: (payload.z ?? 21) * FT,
      ry: payload.r ?? 0,
    };
  } catch {
    return null;
  }
}

export function getShareUrl(state) {
  const base = window.location.href.split('#')[0];
  return `${base}#v=${encodeState(state)}`;
}

export function parseUrlState() {
  const hash = window.location.hash.slice(1);
  if (!hash.startsWith('v=')) return null;
  return decodeState(hash.slice(2));
}

export function getPresetUrl(presetKey) {
  const preset = PRESETS[presetKey];
  if (!preset) return window.location.href.split('#')[0];
  return getShareUrl({
    mode: 'walk',
    floor: preset.floor,
    x: preset.x * FT,
    z: preset.z * FT,
    ry: preset.ry,
  });
}

import QRCode from 'qrcode';

export async function drawQR(canvas, text) {
  const size = 160;
  canvas.width = size;
  canvas.height = size;
  try {
    await QRCode.toCanvas(canvas, text, { width: size, margin: 1 });
  } catch {
    const ctx = canvas.getContext('2d');
    ctx.fillStyle = '#f5f0e8';
    ctx.fillRect(0, 0, size, size);
    ctx.fillStyle = '#3d3530';
    ctx.font = '11px sans-serif';
    ctx.textAlign = 'center';
    ctx.fillText('Copy link above', size / 2, size / 2);
  }
}

export function initShare(viewControls) {
  const modal = document.getElementById('share-modal');
  const btnShare = document.getElementById('btn-share');
  const btnCopy = document.getElementById('btn-copy');
  const shareUrl = document.getElementById('share-url');
  const shareClose = document.getElementById('share-close');
  const toast = document.getElementById('copy-toast');
  const backdrop = modal?.querySelector('.modal-backdrop');

  function openShare() {
    const url = getShareUrl(viewControls.getState());
    shareUrl.value = url;
    modal.classList.remove('hidden');
    drawQR(document.getElementById('qr-canvas'), url);
    history.replaceState(null, '', url);
  }

  function closeShare() {
    modal.classList.add('hidden');
  }

  btnShare?.addEventListener('click', openShare);
  shareClose?.addEventListener('click', closeShare);
  backdrop?.addEventListener('click', closeShare);

  btnCopy?.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(shareUrl.value);
      toast.classList.remove('hidden');
      setTimeout(() => toast.classList.add('hidden'), 2000);
    } catch {
      shareUrl.select();
      document.execCommand('copy');
      toast.classList.remove('hidden');
      setTimeout(() => toast.classList.add('hidden'), 2000);
    }
  });

  document.querySelectorAll('.preset-btn').forEach((btn) => {
    btn.addEventListener('click', () => {
      const key = btn.dataset.preset;
      const url = getPresetUrl(key);
      shareUrl.value = url;
      drawQR(document.getElementById('qr-canvas'), url);
      history.replaceState(null, '', url);
    });
  });

  return { openShare };
}
