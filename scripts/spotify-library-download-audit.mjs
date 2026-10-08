// Rendered saved-library navigation -> real isolated download API/SQLite audit.
// Provider browse responses are synthetic; no live credentials or worker runs.
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { chromium } from '@playwright/test';
const base=process.env.SPOTIFY_LIBRARY_AUDIT_URL,backend=process.env.SPOTIFY_LIBRARY_AUDIT_BACKEND;
assert.ok(base && backend,'Requires explicit preview and isolated backend URLs');
for(const value of [base,backend]) assert.ok(['127.0.0.1','localhost','[::1]'].includes(new URL(value).hostname),'Loopback audit endpoints only');
const output='output/playwright/spotify-library-download';await mkdir(output,{recursive:true});
const albumId='B'.repeat(22),playlistId='P'.repeat(22),trackId='U'.repeat(22);
const artwork='data:image/svg+xml,'+encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256"><rect width="256" height="256" fill="#28243b"/></svg>');
const artist={id:'A'.repeat(22),name:'Artist'};
const track={id:trackId,name:'Album Track',track_number:2,disc_number:1,duration_ms:123456,explicit:false,preview_url:null,artists:[artist]};
const album={id:albumId,name:'Saved Album',artists:[artist],images:[{url:artwork}],release_date:'2020-01-01',total_tracks:1,label:'Fixture',copyrights:[],genres:[],popularity:0,external_urls:{},tracks:{items:[track],total:1,offset:0,next:null}};
const playlist={id:playlistId,name:'Saved Playlist',description:'Fixture',images:[{url:artwork}],owner:{id:'owner',display_name:'Owner'},followers:{total:0},public:false,external_urls:{},snapshot_id:'version1',tracks:{total:1,offset:0,next:null,items:[{added_at:null,track:{...track,id:'T'.repeat(22),name:'Playlist Track',album:{name:'Album',images:[]}}}]}};
const browser=await chromium.launch({headless:true});const page=await browser.newPage({viewport:{width:1440,height:1000}});
const failures=[],requests=[];
page.on('pageerror',error=>failures.push(error.message));
await page.route('https://fonts.googleapis.com/**',r=>r.abort());await page.route('https://fonts.gstatic.com/**',r=>r.abort());
await page.route('**/api/**',r=>r.fulfill({status:404,json:{error:'Not provided by isolated audit'}}));
for(const path of ['songs','folders','playlists','albums/metadata','artists/metadata','spotify/downloaded-ids']) await page.route(`**/api/${path}`,r=>r.fulfill({json:[]}));
await page.route('**/api/health',r=>r.fulfill({json:{status:'ok',version:'audit'}}));
await page.route('**/api/log',r=>r.fulfill({json:{status:'logged'}}));
await page.route('**/api/songs/liked',r=>r.fulfill({json:{ids:[]}}));
await page.route('**/api/albums/liked',r=>r.fulfill({json:{albumKeys:[]}}));
await page.route('**/api/v2/analysis',r=>r.fulfill({json:[]}));
await page.route('**/api/spotify/auth/status',r=>r.fulfill({json:{connected:true,authRequired:false,provider:'webplayer',profile:{id:'owner',display_name:'Owner'}}}));
await page.route('**/api/spotify/proxy?*',r=>{
 const path=new URL(r.request().url()).searchParams.get('path');
 const bodies={'me':{id:'owner',display_name:'Owner',images:[]},'me/albums':{items:[{album}],total:1,next:null},'me/playlists':{items:[playlist],total:1,next:null},['albums/'+albumId]:album,['playlists/'+playlistId]:playlist};
 return r.fulfill({json:bodies[path]??{items:[]}});
});
await page.route('**/api/spotify/download/*',async r=>{
 const kind=new URL(r.request().url()).pathname.split('/').at(-1);
 const body=r.request().postDataJSON();
 const response=await r.fetch({url:backend+'/spotify/download/'+kind});
 const result=await response.json();
 requests.push({kind,body,status:response.status(),result});
 await r.fulfill({response,json:result});
});
async function download(name,kind,expectedOrigins){
 const next=page.waitForResponse(response=>new URL(response.url()).pathname==='/api/spotify/download/'+kind);
 await page.getByRole('button',{name,exact:true}).click();
 const response=await next;assert.equal(response.status(),200);
 const request=requests.at(-1);assert.equal(request.result.status,'queued');
 if(expectedOrigins) assert.deepEqual(request.body.origins,expectedOrigins);else assert.equal(Object.hasOwn(request.body,'origins'),false);
}
const savedAlbum=[{kind:'library',id:'saved_albums',entityId:albumId,position:-1}];
const savedPlaylist=[{kind:'library',id:'saved_playlists',entityId:playlistId,position:-1}];
try {
 await page.goto(base+'/spotify',{waitUntil:'domcontentloaded'});
 await page.getByRole('button',{name:'Saved Albums',exact:true}).click();
 const albumCard=page.getByRole('link',{name:'Open Spotify album Saved Album',exact:true});
 await albumCard.focus();await albumCard.press('Enter');
 await download('Download album','album',savedAlbum);
 await download('Download','track',[...savedAlbum,{kind:'album',id:albumId,position:1}]);
 await page.screenshot({animations:'disabled',path:output+'/saved-album.png'});
 await page.goto(base+'/spotify');
 await page.getByRole('button',{name:'Saved Playlists',exact:true}).click();
 await page.getByRole('link',{name:'Open Spotify playlist Saved Playlist',exact:true}).click();
 const navigationOrigin=await page.evaluate(()=>history.state.usr.downloadLibraryOrigin);
 await download('Download playlist','playlist',savedPlaylist);
 await page.screenshot({animations:'disabled',path:output+'/saved-playlist.png'});
 // Fresh direct navigation carries no saved-library claim.
 await page.goto(base+'/spotify/album/'+albumId);await download('Download album','album');
 await page.goto(base+'/spotify/playlist/'+playlistId);await download('Download playlist','playlist');
 // Stale session and wrong-entity history state cannot add a library claim.
 for(const origin of [{...navigationOrigin,sessionGeneration:999999},{...navigationOrigin,entityId:'X'.repeat(22)},navigationOrigin]) {
  await page.evaluate(origin=>history.replaceState({...history.state,usr:{downloadLibraryOrigin:origin}},'',location.href),origin);
  await page.reload();await download('Download playlist','playlist',origin===navigationOrigin?savedPlaylist:undefined);
 }
 await page.setViewportSize({width:480,height:900});
 await page.getByRole('button',{name:'Download playlist',exact:true}).waitFor();
 const fits=()=>page.getByRole('heading',{level:1}).evaluate(heading=>{
  const box=heading.parentElement.getBoundingClientRect();
  return box.left>=0 && box.right<=innerWidth && heading.parentElement.scrollWidth<=heading.parentElement.clientWidth;
 });
 assert.ok(await fits(),'Narrow playlist header fits');
 await page.screenshot({animations:'disabled',path:output+'/playlist-narrow.png'});
 await page.goto(base+'/spotify/album/'+albumId);await page.getByRole('button',{name:'Download album',exact:true}).waitFor();
 assert.ok(await fits(),'Narrow album header fits');
 await page.screenshot({animations:'disabled',path:output+'/album-narrow.png'});
 assert.deepEqual(failures,[],'No uncaught rendered application errors');
 await writeFile(output+'/requests.json',JSON.stringify(requests,null,2));
 console.log('PASS: saved album/track/playlist origins reach real local download routes; direct/stale/entity-mismatched navigation omits library claims. Provider browsing synthetic; SQLite checked by Go parent.');
} catch(error) {await page.screenshot({animations:'disabled',path:output+'/failure.png'});console.error((await page.locator('body').innerText()).slice(0,1200));throw error;} finally {await browser.close();}
