package api

import (
    "bytes"
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestPodcastPlaybackSettingsRoutesAreScopedAndStrict(t *testing.T) {
    d, _ := seedPodcastDownload(t)
    defer d.Close()
    subs, err := d.ListSubscribedPodcasts(context.Background())
    if err != nil || len(subs) != 1 {
        t.Fatalf("subscriptions: %+v %v", subs, err)
    }
    feedID := subs[0].ID
    a := &API{db: d, dataDir: t.TempDir()}
    router := a.PodcastRoutes()

    req := func(method, path, body string) *httptest.ResponseRecorder {
        t.Helper()
        rec := httptest.NewRecorder()
        request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
        if body != "" {
            request.Header.Set("Content-Type", "application/json")
        }
        router.ServeHTTP(rec, request)
        return rec
    }

    // Global defaults
    gotGlobal := req(http.MethodGet, "/settings", "")
    if gotGlobal.Code != 200 || !bytes.Contains(gotGlobal.Body.Bytes(), []byte(`"defaultSpeed":1`)) ||
        !bytes.Contains(gotGlobal.Body.Bytes(), []byte(`"skipBackSeconds":15`)) {
        t.Fatalf("default global settings: %d %s", gotGlobal.Code, gotGlobal.Body.String())
    }
    for _, bad := range []string{
        `{"defaultSpeed":0,"skipBackSeconds":15,"skipForwardSeconds":30}`,
        `{"defaultSpeed":2.5,"skipBackSeconds":15,"skipForwardSeconds":30}`,
        `{"defaultSpeed":1,"skipBackSeconds":4,"skipForwardSeconds":30}`,
        `{"defaultSpeed":1,"skipBackSeconds":15,"skipForwardSeconds":121}`,
        `{"defaultSpeed":1,"skipBackSeconds":15,"skipForwardSeconds":30,"extra":true}`,
        `{"defaultSpeed":1,"skipBackSeconds":15,"skipForwardSeconds":30}{}`,
    } {
        res := req(http.MethodPut, "/settings", bad)
        if res.Code != 400 {
            t.Fatalf("accepted invalid global settings %s: %d %s", bad, res.Code, res.Body.String())
        }
    }
    putGlobal := req(http.MethodPut, "/settings", `{"defaultSpeed":1.5,"skipBackSeconds":10,"skipForwardSeconds":45}`)
    if putGlobal.Code != 200 || !bytes.Contains(putGlobal.Body.Bytes(), []byte(`"skipForwardSeconds":45`)) {
        t.Fatalf("save global failed: %d %s", putGlobal.Code, putGlobal.Body.String())
    }

    // Per-show defaults
    showURL := "/feeds/" + feedID + "/playback-settings"
    gotShow := req(http.MethodGet, showURL, "")
    if gotShow.Code != 200 || !bytes.Contains(gotShow.Body.Bytes(), []byte(`"defaultSpeed":0`)) {
        t.Fatalf("default show settings: %d %s", gotShow.Code, gotShow.Body.String())
    }
    for _, bad := range []string{
        `{"defaultSpeed":3,"skipIntroSeconds":0,"skipOutroSeconds":0}`,
        `{"defaultSpeed":1,"skipIntroSeconds":-1,"skipOutroSeconds":0}`,
        `{"defaultSpeed":1,"skipIntroSeconds":0,"skipOutroSeconds":601}`,
        `{"defaultSpeed":1,"skipIntroSeconds":0,"skipOutroSeconds":0,"unknown":"x"}`,
    } {
        res := req(http.MethodPut, showURL, bad)
        if res.Code != 400 {
            t.Fatalf("accepted invalid show settings %s: %d %s", bad, res.Code, res.Body.String())
        }
    }
    putShow := req(http.MethodPut, showURL, `{"defaultSpeed":1.25,"skipIntroSeconds":25,"skipOutroSeconds":40}`)
    if putShow.Code != 200 || !bytes.Contains(putShow.Body.Bytes(), []byte(`"skipIntroSeconds":25`)) {
        t.Fatalf("save show failed: %d %s", putShow.Code, putShow.Body.String())
    }
    feedResp := req(http.MethodGet, "/feeds/"+feedID, "")
    if feedResp.Code != 200 || !bytes.Contains(feedResp.Body.Bytes(), []byte(`"defaultSpeed":1.25`)) ||
        !bytes.Contains(feedResp.Body.Bytes(), []byte(`"skipOutroSeconds":40`)) {
        t.Fatalf("feed detail missing playback settings: %d %s", feedResp.Code, feedResp.Body.String())
    }

    missing := req(http.MethodPut, "/feeds/unknown/playback-settings", `{"defaultSpeed":1,"skipIntroSeconds":0,"skipOutroSeconds":0}`)
    if missing.Code != 404 {
        t.Fatalf("missing show writable: %d", missing.Code)
    }
    if err := d.UnsubscribePodcast(context.Background(), feedID); err != nil {
        t.Fatal(err)
    }
    if gone := req(http.MethodGet, showURL, ""); gone.Code != 404 {
        t.Fatalf("unfollowed show exposed settings: %d", gone.Code)
    }
}
