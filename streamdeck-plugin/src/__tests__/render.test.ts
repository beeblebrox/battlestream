import { renderButton } from '../render.js';

test('returns a valid base64 PNG data URL', async () => {
  const result = await renderButton({
    label: 'HEALTH',
    value: '32',
    subtitle: '/ 40',
    gradient: ['#7b0000', '#c0392b'],
    offline: false,
  });
  expect(result).toMatch(/^data:image\/png;base64,/);
  expect(result.length).toBeGreaterThan(100);
});

test('offline flag uses desaturated gradient', async () => {
  const online = await renderButton({ label: 'HEALTH', value: '32', subtitle: '', gradient: ['#7b0000', '#c0392b'], offline: false });
  const offline = await renderButton({ label: 'HEALTH', value: '32', subtitle: '', gradient: ['#7b0000', '#c0392b'], offline: true });
  expect(online).not.toEqual(offline);
});

test('empty subtitle produces no subtitle text region', async () => {
  await expect(
    renderButton({ label: 'TURN', value: '8', subtitle: '', gradient: ['#1a1a3a', '#5d6d7e'], offline: false })
  ).resolves.not.toThrow();
});
