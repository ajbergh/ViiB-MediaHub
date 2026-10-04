/** Tests search cursor advancement independently of fallback items and unequal result buckets. */

import {describe,it,expect} from 'vitest';
import {nextSpotifySearchOffset} from './spotifySearchPaging';
describe('Spotify search paging',()=>{
 it('uses server offset when fallback adds items and buckets have unequal lengths',()=>{
  expect(nextSpotifySearchOffset({
   albums:{items:[1],offset:0,limit:20,next:'https://api.spotify.com/v1/search?offset=20'},
   playlists:{items:Array(25),offset:0,limit:20,next:'https://api.spotify.com/v1/search?offset=20'}
  })).toBe(20);
 });
 it('ends paging when requested buckets are empty or have null next links',()=>{
  expect(nextSpotifySearchOffset({artists:{items:[],next:null},tracks:{items:[1],next:null}})).toBeUndefined();
  expect(nextSpotifySearchOffset({})).toBeUndefined();
 });
 it('retains page-metadata support for opaque links without inferring item counts',()=>{
  expect(nextSpotifySearchOffset({tracks:{items:[1],offset:20,limit:20,next:'cursor'}})).toBe(40);
  expect(nextSpotifySearchOffset({tracks:{items:[1],next:'cursor'}})).toBeUndefined();
 });
});
