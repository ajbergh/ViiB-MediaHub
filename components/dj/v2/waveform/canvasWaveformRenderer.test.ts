import {expect,it,vi} from 'vitest';
import {drawMainWaveform,timeAtLaneX} from './canvasWaveformRenderer';
it('draws three common-scale lanes while preserving local timeline markers',()=>{
 const methods=new Map<string,ReturnType<typeof vi.fn>>();
 const ctx=new Proxy({},{get:(_,key)=>{const name=String(key);if(!methods.has(name))methods.set(name,vi.fn());return methods.get(name);},set:()=>true}) as CanvasRenderingContext2D;
 drawMainWaveform(ctx,20,30,{deck:'A',peaks:null,position:1,duration:2,beatGrid:[1],beatGridOffset:0,cuePoint:0,hotCues:[],loop:{start:0,end:0,enabled:false} as any,visibleSeconds:2,colorMode:'rgb',hasTrack:true,threeBands:{peak:1,data:{source:'local',sourceFingerprint:'fp',durationSeconds:2,low:[1,1],mid:[0.5,0.5],high:[0.25,0.25]}}});
 const rectangles=methods.get('rect')!.mock.calls;
 expect(new Set(rectangles.map(args=>args[3]))).toEqual(new Set([6,3,1.5]));
 expect(methods.get('fillText')!.mock.calls.map(args=>args[0])).toEqual(['Local Low','Local Mid','Local High']);
 expect(methods.get('moveTo')!.mock.calls).toContainEqual([10,0]);
 expect(timeAtLaneX(15,20,1,2,2)).toBe(1.5);
});
it('draws vertical scrolling waveform with horizontal playhead when orientation is vertical',()=>{
 const methods=new Map<string,ReturnType<typeof vi.fn>>();
 const ctx=new Proxy({},{get:(_,key)=>{const name=String(key);if(!methods.has(name))methods.set(name,vi.fn());return methods.get(name);},set:()=>true}) as CanvasRenderingContext2D;
 drawMainWaveform(ctx,30,20,{deck:'B',peaks:[0.8,0.5,0.9,0.4],position:1,duration:2,beatGrid:[1],beatGridOffset:0,cuePoint:0.5,hotCues:[{slot:1,position:1.2,color:'green',origin:'user'}],loop:{start:0.5,end:1.5,enabled:true} as any,visibleSeconds:2,colorMode:'single',hasTrack:true,orientation:'vertical'});
 const rectangles=methods.get('rect')!.mock.calls;
 expect(rectangles.length).toBeGreaterThan(0);
 expect(rectangles.every(args=>args[3]===1)).toBe(true);
 expect(methods.get('moveTo')!.mock.calls).toContainEqual([0,10]);
});
