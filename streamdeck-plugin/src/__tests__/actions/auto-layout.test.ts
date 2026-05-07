const mockSwitchToProfile = jest.fn().mockResolvedValue(undefined);
const mockShowAlert = jest.fn().mockResolvedValue(undefined);

jest.mock('@elgato/streamdeck', () => ({
  action: () => (cls: unknown) => cls,
  SingletonAction: class {},
  streamDeck: {
    profiles: { switchToProfile: mockSwitchToProfile },
  },
}));

import { AutoLayoutAction } from '../../actions/auto-layout.js';

const cases: Array<[number, string]> = [
  [0, 'Battlestream Standard'],
  [1, 'Battlestream Mini'],
  [2, 'Battlestream XL'],
  [7, 'Battlestream Plus'],
  [99, 'Battlestream Standard'],
];

function makeEvent(deviceType: number) {
  return {
    action: {
      device: { type: deviceType, id: 'dev-1' },
      showAlert: mockShowAlert,
    },
  };
}

describe('on non-Linux (official Stream Deck)', () => {
  const originalPlatform = process.platform;
  beforeAll(() => { Object.defineProperty(process, 'platform', { value: 'win32', configurable: true }); });
  afterAll(() => { Object.defineProperty(process, 'platform', { value: originalPlatform, configurable: true }); });
  afterEach(() => jest.clearAllMocks());

  test.each(cases)('device type %i → switchToProfile("%s")', async (deviceType, expectedProfile) => {
    const instance = new AutoLayoutAction();
    await instance.onKeyDown(makeEvent(deviceType) as never);

    expect(mockSwitchToProfile).toHaveBeenCalledWith('dev-1', expectedProfile);
    expect(mockShowAlert).not.toHaveBeenCalled();
  });
});

describe('on Linux (OpenDeck — profile switching restricted to internal plugins)', () => {
  const originalPlatform = process.platform;
  beforeAll(() => { Object.defineProperty(process, 'platform', { value: 'linux', configurable: true }); });
  afterAll(() => { Object.defineProperty(process, 'platform', { value: originalPlatform, configurable: true }); });
  afterEach(() => jest.clearAllMocks());

  test('shows alert and does not call switchToProfile', async () => {
    const instance = new AutoLayoutAction();
    await instance.onKeyDown(makeEvent(0) as never);

    expect(mockShowAlert).toHaveBeenCalled();
    expect(mockSwitchToProfile).not.toHaveBeenCalled();
  });
});
