// Synthetic library/audio/analysis browser audit. Optional playlist backend must be an isolated test API.
import assert from 'node:assert/strict';
import { mkdir } from 'node:fs/promises';
import { chromium } from '@playwright/test';
const expiryAudit=process.env.DJ_AUDIT_SCORE_EXPIRY==='1';
const importAudit=process.env.DJ_AUDIT_IMPORTED_BANDS==='1';
const mixExpiry=process.env.DJ_AUDIT_MIX_EXPIRY==='1';
const mixAudit=process.env.DJ_AUDIT_MIX_PLAYLIST==='1';
const loadedBands=importAudit||mixAudit||process.env.DJ_AUDIT_LOADED_BANDS==='1';
const auditWidth=Number(process.env.DJ_AUDIT_WIDTH||1920),auditHeight=Number(process.env.DJ_AUDIT_HEIGHT||1080);
assert.ok(Number.isInteger(auditWidth)&&auditWidth>=1000&&Number.isInteger(auditHeight)&&auditHeight>=600,'Valid DJ audit viewport');
const output=importAudit?'output/playwright/dj-imported-bands':mixAudit?'output/playwright/dj-mix-playlist':expiryAudit?'output/playwright/dj-score-expiry':loadedBands?`output/playwright/dj-loaded-bands-${auditWidth}x${auditHeight}`:'output/playwright/dj-metadata-playlist'; await mkdir(output,{recursive:true});
const browser=await chromium.launch({headless:true});
const page=await browser.newPage({viewport:{width:auditWidth,height:auditHeight}});
if(mixExpiry) await page.clock.install({time:new Date('2099-12-31T23:59:00Z')});
if(expiryAudit) await page.clock.install({time:new Date('2099-12-31T23:59:00Z')});
page.on('pageerror',error=>console.error('BROWSER ERROR:',error.message));
page.on('console',message=>{if(message.type()==='error') console.error('BROWSER CONSOLE:',message.text());});
const songs=['Zero','High','Stale','Unknown'].map((name,i)=>({id:`score-${i}`,title:name,artist:'Fixture',album:'Score audit',duration:12,filePath:`/api/audio/score-${i}`,genre:[]}));
const features=songs.map((song,i)=>({songId:song.id,status:'complete',bpm:120,bpmSource:'measured',keySource:'unknown',syncAllowed:true,energyLevel:7,...(i<3?{providerScores:{spotify_energy_score:{value:i===0?0:0.9,stale:i===2,retrievedAt:'2026-10-05T00:00:00Z',expiresAt:'2100-01-01T00:00:00Z',endpoint:'audio_features'}}}:{})}));
let saves=[]; let persistedPlaylists=[]; let readbackMode='exact'; let recommendationQueries=[]; let awaitExpirySettled=false;
await page.route('https://fonts.googleapis.com/**',r=>r.abort()); await page.route('https://fonts.gstatic.com/**',r=>r.abort());
await page.route('**/api/**',r=>r.fulfill({status:404,json:{error:'Not provided by synthetic audit'}}));
for (const path of ['folders','albums/metadata','artists/metadata']) await page.route(`**/api/${path}`,r=>r.fulfill({json:[]}));
await page.route('**/api/songs/liked',r=>r.fulfill({json:{ids:[]}}));
await page.route('**/api/albums/liked',r=>r.fulfill({json:{albumKeys:[]}}));
await page.route('**/api/v2/jobs/',r=>r.fulfill({json:{jobs:[]}}));
await page.route('**/api/health',r=>r.fulfill({json:{status:'ok',version:'audit'}}));
await page.route('**/api/songs',r=>r.fulfill({json:songs}));
await page.route('**/api/v2/analysis',r=>r.fulfill({json:features}));
if(loadedBands) {
 await page.route('**/api/audio/score-*',async r=>{
  // Serve a deterministic 12-second mono WAV independently of real media.
  const frames=44100*12,tone=Buffer.alloc(44+frames*2);tone.write('RIFF');tone.writeUInt32LE(tone.length-8,4);tone.write('WAVEfmt ',8);tone.writeUInt32LE(16,16);tone.writeUInt16LE(1,20);tone.writeUInt16LE(1,22);tone.writeUInt32LE(44100,24);tone.writeUInt32LE(88200,28);tone.writeUInt16LE(2,32);tone.writeUInt16LE(16,34);tone.write('data',36);tone.writeUInt32LE(frames*2,40);for(let i=0;i<frames;i++)tone.writeInt16LE(Math.round(Math.sin(i*2*Math.PI*220/44100)*500),44+i*2);
  await r.fulfill({contentType:'audio/wav',body:tone});
 });
 await page.route('**/api/v2/analysis/score-0',r=>r.fulfill({json:{...features[0],sourceFingerprint:'fixture-fp'}}));
 if(mixAudit) {
  await page.route('**/api/v2/analysis/score-0/energy',r=>r.fulfill({json:{songId:'score-0',integratedLufs:-10,truePeakDbfs:-1,energy:[{time:0,value:0.5}],sections:[],cueSuggestions:[],algorithmVersion:'fixture'}}));
  await page.route('**/api/v2/analysis/score-0/recommendations?*',r=>{
   const query=new URL(r.request().url()).searchParams;recommendationQueries.push(Object.fromEntries(query));
   const filtered=query.get('spotifyScoreMetric')==='energy';
   const candidates=filtered?(mixExpiry && awaitExpirySettled?[]:[3]):[1,3];
   return r.fulfill({json:{songId:'score-0',intent:'hold',algorithmVersion:'fixture',filters:Object.fromEntries(query),candidatesBeforeFilters:3,candidatesAfterFilters:candidates.length,recommendations:candidates.map((i,n)=>({songId:songs[i].id,title:songs[i].title,artist:'Fixture',score:0.9-n*0.1,intent:'hold',vector:{outgoingTailEnergy:0.5,incomingHeadEnergy:0.5,energyDelta:0,loudnessDeltaLu:0,outgoingMixOutConfidence:0,incomingMixInConfidence:0},components:[],filterEvidence:filtered?{spotifyScoreMetric:'energy',spotifyScore:{value:0,endpoint:'audio_features',retrievedAt:'2026-10-06T00:00:00Z',expiresAt:'2100-01-01T00:00:00Z',stale:false}}:{}}))}});
  });
 }
 if(importAudit) await page.route('**/api/v2/analysis/score-0/provider-analysis/*?*',r=>{const url=new URL(r.request().url()),kind=url.pathname.split('/').at(-1),offset=Number(url.searchParams.get('offset'));if(kind==='segments')return r.fulfill({json:{songId:'score-0',sourceFingerprint:'fixture-fp',recordingId:'TTTTTTTTTTTTTTTTTTTTTT',kind,provenance:'spotify_durable_import',retrievedAt:'2026-10-01T00:00:00Z',stale:false,offset,limit:25,totalItems:1,items:[{start:0,duration:1,confidence:0,loudness_start:-20,loudness_max:-10,loudness_max_time:0.5,pitches:Array(12).fill(0),timbre:Array(12).fill(-1)}]}});if(kind!=='beats')return r.fulfill({status:404,json:{error:'not retained'}});return r.fulfill({json:{songId:'score-0',sourceFingerprint:'fixture-fp',recordingId:'TTTTTTTTTTTTTTTTTTTTTT',kind,provenance:'spotify_durable_import',retrievedAt:'2026-10-01T00:00:00Z',stale:true,offset,limit:25,totalItems:26,items:Array.from({length:Math.min(25,26-offset)},(_,i)=>({start:offset+i,duration:1,confidence:0}))}});});
 if(importAudit) await page.route('**/api/v2/analysis/score-0/waveform/spotify-three-band',r=>r.fulfill({json:{songId:'score-0',sourceFingerprint:'fixture-fp',recordingId:'TTTTTTTTTTTTTTTTTTTTTT',representation:'spotify_three_band',lows:[1,2,1],mids:[0,1,0],highs:[0,0,1],sampleRate:44100,windowMilliseconds:4000,totalSamples:3,displayStride:1,displayAggregation:'signed_absolute_max_v1',durationSeconds:12,localDurationSeconds:12,alignment:'duration_compatible',retrievedAt:'2026-10-01T00:00:00Z',stale:false,readOnly:true,unverified:false,provenance:'spotify_durable_import'}}));
 const low=Array.from({length:120},(_,i)=>0.2+0.1*Math.sin(i/5)),mid=low.map(v=>v/2),high=low.map(v=>v/4);
 await page.route('**/api/v2/analysis/score-0/waveform/local-three-band',r=>r.fulfill({json:{songId:'score-0',sourceFingerprint:'fixture-fp',representation:'local_three_band_estimate',algorithmVersion:'onepole-band-peaks-v1',overview:{sampleRate:44100,frames:529200,resolution:4410,low,mid,high,units:'filtered_pcm_absolute_peak',filter:'one_pole_250_4000_hz_residual_v1',normalization:'none_equal_channel_mono'}}}));
}

await page.route('**/api/playlists',async r=>{
 if(process.env.DJ_AUDIT_PLAYLIST_BACKEND) {
  const response=await r.fetch({url:process.env.DJ_AUDIT_PLAYLIST_BACKEND});
  assert.equal(response.status(),200,'Persistent playlist endpoint succeeds');
  const body=await response.json();
  if(r.request().method()==='POST') { saves.push(r.request().postDataJSON()); await r.fulfill({response,json:body}); }
  else { persistedPlaylists=body; await r.fulfill({response,json:readbackMode==='missing'?[]:readbackMode==='mismatch'?body.map(p=>({...p,songIds:[...p.songIds].reverse()})):body}); }
  return;
 }
 if(r.request().method()==='POST') {const body=r.request().postDataJSON(); saves.push(body); const saved={id:'created-'+saves.length,...body,createdAt:1}; persistedPlaylists.push(saved); await r.fulfill({json:saved});}
 else await r.fulfill({json:readbackMode==='missing'?[]:readbackMode==='mismatch'?persistedPlaylists.map(p=>({...p,songIds:[...p.songIds].reverse()})):persistedPlaylists});
});
try {
 await page.goto(process.env.DJ_AUDIT_URL||'http://127.0.0.1:4173/dj',{waitUntil:'domcontentloaded'});
 await page.getByRole('button',{name:'Library /',exact:true}).click({timeout:5000});
 const drawer=page.getByRole('region',{name:'DJ library'});
 await drawer.getByRole('button',{name:'Load Zero to Deck A',exact:true}).waitFor();
 await drawer.getByRole('combobox',{name:'Spotify score metric'}).selectOption('energy');
 await drawer.getByText('Spotify energy: 0.00 / 1',{exact:true}).waitFor();
 await drawer.getByRole('combobox',{name:'Spotify score order'}).selectOption('desc');
 // Read names rather than button glyphs for virtualized row order.
 const names=await drawer.getByRole('button',{name:/^Load .* to Deck A$/}).evaluateAll(elements=>elements.map(e=>e.getAttribute('aria-label')));
 assert.deepEqual(names,['High','Zero','Stale','Unknown'].map(n=>`Load ${n} to Deck A`));
 await drawer.getByRole('spinbutton',{name:'Minimum Spotify score'}).fill('0');
 await drawer.getByRole('spinbutton',{name:'Maximum Spotify score'}).fill('0');
 await drawer.getByRole('button',{name:'Save 1 results as playlist',exact:true}).click();
 await drawer.getByRole('textbox',{name:'Playlist name'}).fill('Zero score set');
 await drawer.getByRole('button',{name:'Create playlist',exact:true}).click();
 await drawer.getByText('Saved 1 tracks as “Zero score set”.',{exact:true}).waitFor();
 assert.deepEqual(saves[0].songIds,['score-0']); assert.equal(saves[0].name,'Zero score set');
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
 if(expiryAudit) {
  await page.clock.fastForward(60001);
  await drawer.getByRole('button',{name:'Load Zero to Deck A',exact:true}).waitFor({state:'detached'});
  assert.equal(await drawer.getByRole('button',{name:/^Load .* to Deck A$/}).count(),0);
  await drawer.getByRole('spinbutton',{name:'Minimum Spotify score'}).fill('');
  await drawer.getByRole('spinbutton',{name:'Maximum Spotify score'}).fill('');
  await drawer.getByRole('button',{name:'Load Zero to Deck A',exact:true}).waitFor();
  assert.equal(await drawer.getByText('Spotify energy: 0.00 / 1',{exact:true}).count(),0);
  console.log('PASS: open-library deadline timer excludes expired scores from the active range, then restores unknown rows when the range clears.');
 }
 if(loadedBands) {
  await drawer.getByRole('button',{name:'Load Zero to Deck A',exact:true}).click();
  await page.getByRole('button',{name:'Close library',exact:true}).click();
  await page.getByRole('button',{name:/^Local bands$/i}).first().click();
  await page.getByText('Local bands - Low / Mid / High - Common peak scale',{exact:true}).waitFor({timeout:30000});
  await page.getByRole('button',{name:/^Deck A analysis and editing/}).click();
  const inspector=page.locator('[data-dj-deck-inspector="A"]');
  if(mixAudit) {
   await inspector.getByRole('tab',{name:'Analysis',exact:true}).click();
   await inspector.getByText('Build a playlist from 2 Mix Next candidates',{exact:true}).click();
   await inspector.getByRole('checkbox',{name:'Include High in playlist',exact:true}).uncheck();
   await inspector.getByRole('button',{name:'Save 2 results as playlist',exact:true}).click();
   // The open form keeps its captured selection even if candidates change.
   await inspector.getByRole('checkbox',{name:'Include High in playlist',exact:true}).check();
   await inspector.getByRole('textbox',{name:'Playlist name'}).fill('Reference mix');
   await inspector.getByRole('button',{name:'Create playlist',exact:true}).click();
   await inspector.getByText('Saved 2 tracks as “Reference mix”.',{exact:true}).waitFor();
   assert.deepEqual(saves[1].songIds,['score-0','score-3']);
   assert.equal(saves[1].name,'Reference mix');
   await page.screenshot({path:`${output}/mix-saved.png`});
   console.log('PASS: rendered Mix Next candidate omission, reference-first order and captured selection across later checkbox changes.');
   assert.ok(recommendationQueries.some(query=>!query.spotifyScoreMetric),'Default request has no native preference');
   await inspector.getByRole('combobox',{name:'Mix Next Spotify score',exact:true}).selectOption('energy');
   await inspector.getByRole('spinbutton',{name:'Minimum Spotify score',exact:true}).fill('0');
   await inspector.getByRole('spinbutton',{name:'Maximum Spotify score',exact:true}).fill('0');
   await inspector.getByText('Build a playlist from 1 Mix Next candidates',{exact:true}).waitFor();
   await inspector.getByText('Build a playlist from 1 Mix Next candidates',{exact:true}).click();
   await inspector.getByRole('button',{name:'Save 2 results as playlist',exact:true}).click();
   await inspector.getByRole('textbox',{name:'Playlist name'}).fill('Native zero mix');
   await inspector.getByRole('button',{name:'Create playlist',exact:true}).click();
   await inspector.getByText('Saved 2 tracks as “Native zero mix”.',{exact:true}).waitFor();
   assert.deepEqual(saves[2].songIds,['score-0','score-3']);
   assert.ok(recommendationQueries.some(query=>query.spotifyScoreMetric==='energy'&&query.minSpotifyScore==='0'&&query.maxSpotifyScore==='0'));
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
   await page.screenshot({path:`${output}/native-score-mix-${auditWidth}x${auditHeight}.png`});
   console.log('PASS: optional native score controls serialize inclusive zero range and preserve reference-first filtered playlist selection.');
   if(!mixExpiry) {
    await inspector.getByRole('button',{name:'Save 2 results as playlist',exact:true}).click();
    const draft=inspector.getByRole('textbox',{name:'Playlist name'});
    await draft.fill('  Verified retry mix  ');
    for(const mode of ['missing','mismatch']) {
     readbackMode=mode;
     await inspector.getByRole('button',{name:'Create playlist',exact:true}).click();
     await inspector.getByRole('alert').filter({hasText:'could not be verified'}).waitFor();
     assert.equal(await draft.inputValue(),'  Verified retry mix  ');
     assert.equal(await inspector.getByText('Saved 2 tracks as “Verified retry mix”.',{exact:true}).count(),0);
    }
    readbackMode='exact';
    await inspector.getByRole('button',{name:'Create playlist',exact:true}).click();
    await inspector.getByText('Saved 2 tracks as “Verified retry mix”.',{exact:true}).waitFor();
    const verifiedRetry=persistedPlaylists.find(p=>p.name==='Verified retry mix');
    assert.ok(verifiedRetry);
    assert.deepEqual(verifiedRetry.songIds,['score-0','score-3']);
    console.log('PASS: missing/mismatched synthetic GET retains draft; exact normalized name and ordered IDs permit success.');
   }
   if(mixExpiry) {
    await inspector.getByRole('button',{name:'Save 2 results as playlist',exact:true}).click();
    await inspector.getByRole('textbox',{name:'Playlist name'}).fill('Expired draft');
    awaitExpirySettled=true;
    await page.clock.fastForward(60001);
    await inspector.getByText('Spotify score evidence expired. Updating candidates; review your playlist selection again.',{exact:true}).waitFor();
    await inspector.getByText('No analyzed candidates match these filters (3 checked).',{exact:true}).waitFor();
    assert.equal(await inspector.getByRole('textbox',{name:'Playlist name'}).count(),0);
    assert.equal(await inspector.getByRole('button',{name:'Create playlist',exact:true}).count(),0);
    assert.equal(saves.length,3,'Expired captured draft was not submitted');
    await page.screenshot({path:`${output}/expired-score-mix-${auditWidth}x${auditHeight}.png`});
    console.log('PASS: mounted Mix Next expiry clears score-based candidate and captured playlist draft before refreshing.');
   }
  }
  await inspector.getByRole('tab',{name:'Audio',exact:true}).click();
  await inspector.getByRole('heading',{name:'Local three-band estimate',exact:true}).waitFor();
  assert.equal(await inspector.getByRole('img',{name:/band envelope/}).count(),3);
  if(importAudit) {
   await inspector.getByRole('heading',{name:'Spotify three-band waveform',exact:true}).scrollIntoViewIfNeeded();
   assert.equal(await inspector.getByRole('img',{name:/^Spotify (low|mid|high) band$/}).count(),3);
   await inspector.getByText('Retained with this downloaded file. Available independently of the connected Spotify account.',{exact:true}).waitFor();
   assert.equal(await inspector.getByRole('button',{name:'Refresh Spotify waveform',exact:true}).isDisabled(),true);
   await page.screenshot({path:`${output}/imported.png`});
   await inspector.getByText('Retained Spotify timing and segments',{exact:true}).click();
   const table=inspector.getByRole('table');
   await table.waitFor();
   assert.equal(await table.locator('tbody tr').count(),25);
   assert.equal(await table.getByText('0%',{exact:true}).count(),25);
   await inspector.getByRole('button',{name:'Next',exact:true}).click();
   await table.getByText('25.00',{exact:true}).waitFor();
   assert.equal(await table.locator('tbody tr').count(),1);
   assert.equal(await inspector.getByRole('button',{name:'Next',exact:true}).isDisabled(),true);
   await inspector.getByRole('button',{name:'Previous',exact:true}).click();
   await table.getByText('0.00',{exact:true}).waitFor();
   assert.equal(await table.locator('tbody tr').count(),25);
   await table.scrollIntoViewIfNeeded();
   await page.screenshot({path:`${output}/detailed-page.png`});
   await inspector.getByRole('combobox',{name:'Detailed array'}).selectOption('sections');
   await inspector.getByText('This array was not retained for the current file.',{exact:true}).waitFor();
   await inspector.getByRole('combobox',{name:'Detailed array'}).selectOption('segments');
   await inspector.getByText('Inspect segment 1',{exact:true}).click();
   const vector=inspector.getByText(/^Timbre coefficients 1-12:/);
   await vector.scrollIntoViewIfNeeded();
   assert.equal(await vector.isVisible(),true);
   assert.ok((await vector.innerText()).includes('-1.00'));
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
   await page.screenshot({path:`${output}/segment-details-${auditWidth}x${auditHeight}.png`});
   console.log('PASS: retained beats table, zero confidence, terminal/previous pages and missing-array state.');
   console.log('PASS: waveform-only durable import renders three bands without provider scalar summaries; refresh stays disabled.');
  }
  await inspector.getByText('Local analysis coverage and limits',{exact:true}).click();
  const limits=inspector.getByRole('table',{name:'Audio analysis capability status'});
  await limits.scrollIntoViewIfNeeded();
  assert.equal(await limits.isVisible(),true);
  assert.equal(await inspector.getByText(/Missing Spotify fields do not imply zero/).count(),1);
  await page.screenshot({path:`${output}/coverage-limits.png`});
 }
 await page.screenshot({path:`${output}/saved.png`});
 if(loadedBands) {
  const inspector=page.locator('[data-dj-deck-inspector="A"]');
  await inspector.getByRole('heading',{name:'Spotify observations',exact:true}).scrollIntoViewIfNeeded();
  assert.equal(await inspector.getByRole('heading',{name:'Spotify observations',exact:true}).isVisible(),true);
  const scroll=await inspector.locator('.dj-deck-inspector-body').evaluate(el=>({top:el.scrollTop,overflow:el.scrollHeight>el.clientHeight}));
  assert.ok(scroll.overflow && scroll.top>0,'Long Audio inspector scrolls to remaining metadata');
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'Loaded inspector avoids horizontal page overflow');
  const inspectorBounds=await inspector.boundingBox();
  assert.ok(inspectorBounds && inspectorBounds.x>=0 && inspectorBounds.y>=0 && inspectorBounds.x+inspectorBounds.width<=auditWidth+1 && inspectorBounds.y+inspectorBounds.height<=auditHeight+1,'Loaded inspector stays inside the viewport');
  await page.screenshot({path:`${output}/inspector-scrolled.png`});
  console.log('PASS: loaded 12-second synthetic audio, source-matched local-band overview, three inspector envelopes and scrollable metadata. No DSP/live-provider qualification.');
 }

 console.log('PASS: native zero score, unknown-last order, range exclusion, captured playlist payload and no horizontal page overflow. '+(process.env.DJ_AUDIT_PLAYLIST_BACKEND?'Playlist HTTP/SQLite backend; other APIs synthetic.':'Synthetic API fixtures only.'));
} catch(error) { await page.screenshot({path:`${output}/failure.png`}); console.error((await page.locator('body').innerText()).slice(0,600)); throw error; } finally {await browser.close();}
