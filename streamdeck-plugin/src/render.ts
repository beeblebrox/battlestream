import { createCanvas, loadImage } from '@napi-rs/canvas';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

export interface RenderOptions {
  label: string;
  value: string;
  subtitle: string;
  gradient: readonly [string, string];
  offline: boolean;
  iconPath?: string;
}

const imageCache = new Map<string, Awaited<ReturnType<typeof loadImage>>>();

async function getCachedImage(p: string) {
  if (!imageCache.has(p)) {
    imageCache.set(p, await loadImage(p));
  }
  return imageCache.get(p)!;
}

export async function renderButton(opts: RenderOptions): Promise<string> {
  const SIZE = 144;
  const canvas = createCanvas(SIZE, SIZE);
  const ctx = canvas.getContext('2d');

  if (opts.iconPath && !opts.offline) {
    try {
      const img = await getCachedImage(opts.iconPath);
      ctx.drawImage(img, 0, 0, SIZE, SIZE);
      // Semi-transparent overlay so text stays legible
      ctx.fillStyle = 'rgba(0,0,0,0.45)';
      ctx.fillRect(0, 0, SIZE, SIZE);
    } catch {
      drawGradientBg(ctx, opts, SIZE);
    }
  } else {
    drawGradientBg(ctx, opts, SIZE);
  }

  const PAD = 12;
  const maxW = SIZE - PAD * 2;

  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';

  ctx.fillStyle = 'rgba(255,255,255,0.80)';
  fitText(ctx, opts.label.toUpperCase(), maxW, 17, 'bold', 10);
  ctx.fillText(opts.label.toUpperCase(), SIZE / 2, 30);

  ctx.fillStyle = '#ffffff';
  fitText(ctx, opts.value, maxW, 52, 'bold', 14);
  ctx.fillText(opts.value, SIZE / 2, 82);

  if (opts.subtitle) {
    ctx.fillStyle = 'rgba(255,255,255,0.65)';
    fitText(ctx, opts.subtitle, maxW, 14, 'normal', 8);
    ctx.fillText(opts.subtitle, SIZE / 2, 122);
  }

  return `data:image/png;base64,${canvas.toBuffer('image/png').toString('base64')}`;
}

function fitText(
  ctx: ReturnType<ReturnType<typeof createCanvas>['getContext']>,
  text: string,
  maxWidth: number,
  maxSize: number,
  weight: string,
  minSize: number,
): void {
  let size = maxSize;
  ctx.font = `${weight} ${size}px sans-serif`;
  while (ctx.measureText(text).width > maxWidth && size > minSize) {
    size -= 1;
    ctx.font = `${weight} ${size}px sans-serif`;
  }
}

function drawGradientBg(ctx: ReturnType<ReturnType<typeof createCanvas>['getContext']>, opts: RenderOptions, SIZE: number) {
  const [c1, c2] = opts.offline ? (['#2a2a2a', '#444444'] as const) : opts.gradient;
  const grd = ctx.createLinearGradient(0, 0, SIZE, SIZE);
  grd.addColorStop(0, c1);
  grd.addColorStop(1, c2);
  ctx.fillStyle = grd;
  ctx.fillRect(0, 0, SIZE, SIZE);
}
