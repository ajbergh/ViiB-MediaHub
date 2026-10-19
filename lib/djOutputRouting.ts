/** Physical output routing within the live DJ engine's AudioContext. */
export type DJOutputMode = 'separate' | 'four-channel';

export function connectFourChannelOutput(context: AudioContext, master: AudioNode, headphones: AudioNode) {
  if (context.destination.maxChannelCount < 4) {
    throw new Error(`This device exposes ${context.destination.maxChannelCount} output channels to this runtime. Four channels are required for Main 1–2 and Headphones 3–4.`);
  }
  context.destination.channelCount = 4;
  context.destination.channelCountMode = 'explicit';
  context.destination.channelInterpretation = 'discrete';
  if (context.destination.channelCount !== 4) throw new Error('The device did not accept four-channel output.');
  const main = context.createChannelSplitter(2);
  const cue = context.createChannelSplitter(2);
  const merger = context.createChannelMerger(4);
  main.channelInterpretation = cue.channelInterpretation = merger.channelInterpretation = 'discrete';
  master.connect(main);
  headphones.connect(cue);
  main.connect(merger, 0, 0);
  main.connect(merger, 1, 1);
  cue.connect(merger, 0, 2);
  cue.connect(merger, 1, 3);
  merger.connect(context.destination);
  let connected = true;
  const tests = new Set<() => void>();
  return {
    /** Bypass cue/fader settings to verify one physical output at a time. */
    testChannel(channel: number): Promise<void> {
      if (!connected) throw new Error('The audio route has been disconnected.');
      if (!Number.isInteger(channel) || channel < 0 || channel > 3) throw new RangeError('Output channel must be 0–3.');
      for (const stop of tests) stop();
      const tone = context.createOscillator();
      const gain = context.createGain();
      tone.frequency.value = 440;
      gain.channelCount = 1;
      gain.channelCountMode = 'explicit';
      const now = context.currentTime;
      gain.gain.setValueAtTime(0, now);
      gain.gain.linearRampToValueAtTime(0.1, now + 0.02);
      gain.gain.setValueAtTime(0.1, now + 0.7);
      gain.gain.linearRampToValueAtTime(0, now + 0.75);
      tone.connect(gain);
      gain.connect(merger, 0, channel);
      return new Promise<void>((resolve, reject) => {
        let finished = false;
        const finish = (error?: unknown) => {
          if (finished) return;
          finished = true;
          tests.delete(stop);
          tone.onended = null;
          tone.disconnect();
          gain.disconnect();
          if (error !== undefined) reject(error);
          else resolve();
        };
        const stop = () => { try { tone.stop(); } catch { /* already stopped */ } finish(); };
        tone.onended = () => finish();
        tests.add(stop);
        try { tone.start(now); tone.stop(now + 0.8); }
        catch (error) { try { tone.stop(); } catch { /* not started */ } finish(error); }
      });
    },
    stopTests() { for (const stop of tests) stop(); },
    disconnect() {
      if (!connected) return;
      connected = false;
      for (const stop of tests) stop();
      master.disconnect(main);
      headphones.disconnect(cue);
      main.disconnect();
      cue.disconnect();
      merger.disconnect();
    },
  };
}
