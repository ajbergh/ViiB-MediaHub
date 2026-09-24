// Keep loop boundary checks independent from the UI's requestAnimationFrame
// cycle so rendering pauses do not automatically suspend loop handling.
const timer = setInterval(() => postMessage('tick'), 5);
self.addEventListener('close', () => clearInterval(timer));
