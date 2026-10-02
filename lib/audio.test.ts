import {afterEach,expect,it,vi} from 'vitest';
import {audioEngine} from './audio';
afterEach(()=>vi.useRealTimers());
it('obsolete crossfade cleanup cannot stop a reused incoming element',async()=>{
 vi.useFakeTimers();const gain=()=>({cancelScheduledValues:vi.fn(),setValueAtTime:vi.fn(),linearRampToValueAtTime:vi.fn()});
 const first={pause:vi.fn(),play:vi.fn().mockResolvedValue(undefined),currentTime:10} as any;
 const second={pause:vi.fn(),play:vi.fn().mockResolvedValue(undefined),currentTime:10} as any;
 const engine=audioEngine as any;engine.context={currentTime:0,state:'running'};
 engine.sources=new Map([[first,{inputGain:{gain:gain()}}],[second,{inputGain:{gain:gain()}}]]);
 await audioEngine.transition(first,second,1);await vi.advanceTimersByTimeAsync(100);await audioEngine.transition(second,first,1);
 await vi.advanceTimersByTimeAsync(1000);expect(first.pause).not.toHaveBeenCalled();expect(first.currentTime).toBe(10);
 await vi.advanceTimersByTimeAsync(100);expect(second.pause).toHaveBeenCalledOnce();
});
