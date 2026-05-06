jest.mock('@elgato/streamdeck', () => ({
  action: () => (cls: unknown) => cls,
  SingletonAction: class {},
  streamDeck: {
    profiles: { switchToProfile: jest.fn().mockResolvedValue(undefined) },
  },
}));

import { streamDeck } from '@elgato/streamdeck';
import { AutoLayoutAction } from '../../actions/auto-layout.js';

const cases: Array<[number, string]> = [
  [0, 'Battlestream Standard'],
  [1, 'Battlestream Mini'],
  [2, 'Battlestream XL'],
  [7, 'Battlestream Plus'],
  [99, 'Battlestream Standard'],
];

test.each(cases)('device type %i → profile "%s"', async (deviceType, expectedProfile) => {
  const action = new AutoLayoutAction();
  const mockEv = {
    action: { device: { type: deviceType, id: 'dev-1' } },
  };
  await action.onKeyDown(mockEv as never);

  expect(streamDeck.profiles.switchToProfile).toHaveBeenCalledWith(
    'dev-1',
    expectedProfile,
  );
  jest.clearAllMocks();
});
