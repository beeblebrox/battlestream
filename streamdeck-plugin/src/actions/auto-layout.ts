import { action, SingletonAction, streamDeck, type KeyDownEvent } from '@elgato/streamdeck';

const PROFILES: Record<number, string> = {
  0: 'Battlestream Standard',
  1: 'Battlestream Mini',
  2: 'Battlestream XL',
  7: 'Battlestream Plus',
};

@action({ UUID: 'com.battlestream.streamdeck.auto-layout' })
export class AutoLayoutAction extends SingletonAction<Record<string, never>> {
  override async onKeyDown({ action }: KeyDownEvent<Record<string, never>>): Promise<void> {
    // OpenDeck (Linux) restricts profile switching to internal plugins via UUID allowlist;
    // 3rd-party plugins cannot switch profiles there. Show alert so the user knows.
    if (process.platform === 'linux') {
      await action.showAlert();
      return;
    }

    const { type, id } = action.device;
    const profile = PROFILES[type] ?? 'Battlestream Standard';
    await streamDeck.profiles.switchToProfile(id, profile);
  }
}
