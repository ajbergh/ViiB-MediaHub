type Peaks = ArrayLike<number> & object;
const pyramids = new WeakMap<Peaks, PeakPyramid>();

/** Exact range maxima over immutable waveform arrays. Aligned power-of-two
 * blocks preserve transients and reduce wide pixel buckets to O(log n) reads. */
export class PeakPyramid {
  readonly levels: ArrayLike<number>[];
  constructor(readonly source: Peaks) {
    this.levels = [source];
    while (this.levels.at(-1)!.length > 1) {
      const previous = this.levels.at(-1)!;
      const next = new Float64Array(Math.ceil(previous.length / 2));
      for (let i = 0; i < next.length; i++) next[i] = Math.max(0, previous[i * 2] || 0, previous[i * 2 + 1] || 0);
      this.levels.push(next);
    }
  }
  max(first: number, end: number): number {
    first = Math.max(0, Math.floor(first));
    end = Math.min(this.source.length, Math.ceil(end));
    let result = 0;
    while (first < end) {
      let level = Math.min(this.levels.length - 1, Math.floor(Math.log2(end - first)));
      while (level > 0 && first % (2 ** level) !== 0) level--;
      result = Math.max(result, this.levels[level][Math.floor(first / (2 ** level))] || 0);
      first += 2 ** level;
    }
    return result;
  }
}

export function peakRangeMax(source: Peaks, first: number, end: number): number {
  let pyramid = pyramids.get(source);
  if (!pyramid) { pyramid = new PeakPyramid(source); pyramids.set(source, pyramid); }
  return pyramid.max(first, end);
}
