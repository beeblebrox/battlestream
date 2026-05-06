import { createCanvas } from '@napi-rs/canvas';

export interface RenderOptions {
  label: string;
  value: string;
  subtitle: string;
  gradient: readonly [string, string];
  offline: boolean;
}

export function renderButton(opts: RenderOptions): string {
  const SIZE = 144;
  const canvas = createCanvas(SIZE, SIZE);
  const ctx = canvas.getContext('2d');

  const [c1, c2] = opts.offline ? (['#2a2a2a', '#444444'] as const) : opts.gradient;
  const grd = ctx.createLinearGradient(0, 0, SIZE, SIZE);
  grd.addColorStop(0, c1);
  grd.addColorStop(1, c2);
  ctx.fillStyle = grd;
  ctx.fillRect(0, 0, SIZE, SIZE);

  ctx.fillStyle = 'rgba(255,255,255,0.70)';
  ctx.font = 'bold 17px sans-serif';
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  ctx.fillText(opts.label.toUpperCase(), SIZE / 2, 30);

  ctx.fillStyle = '#ffffff';
  ctx.font = 'bold 52px sans-serif';
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  ctx.fillText(opts.value, SIZE / 2, 82);

  if (opts.subtitle) {
    ctx.fillStyle = 'rgba(255,255,255,0.55)';
    ctx.font = '14px sans-serif';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(opts.subtitle, SIZE / 2, 122);
  }

  return `data:image/png;base64,${canvas.toBuffer('image/png').toString('base64')}`;
}
