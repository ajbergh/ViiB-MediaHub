package api

import (
    "bytes"
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestPodcastAutoDownloadPolicyRoutesAreScopedAndStrict(t *testing.T){
    d,_:=seedPodcastDownload(t)
    defer d.Close()
    subs,err:=d.ListSubscribedPodcasts(context.Background())
    if err!=nil||len(subs)!=1{t.Fatalf("subscriptions: %+v %v",subs,err)}
    feedID:=subs[0].ID
    a:=&API{db:d,dataDir:t.TempDir()}
    router:=a.PodcastRoutes()
    req:=func(method,path,body string)*httptest.ResponseRecorder{
        t.Helper()
        record:=httptest.NewRecorder()
        request:=httptest.NewRequest(method,path,bytes.NewBufferString(body))
        if body!=""{request.Header.Set("Content-Type","application/json")}
        router.ServeHTTP(record,request)
        return record
    }
    url:="/feeds/"+feedID+"/auto-download"
    got:=req(http.MethodGet,url,"")
    if got.Code!=200||!bytes.Contains(got.Body.Bytes(),[]byte(`"enabled":false`))||
        !bytes.Contains(got.Body.Bytes(),[]byte(`"newestCount":1`)){
        t.Fatalf("default policy: %d %s",got.Code,got.Body.String())
    }
    for _,body:=range []string{
        `{"enabled":true,"newestCount":0}`,
        `{"enabled":true,"newestCount":4}`,
        `{"enabled":true,"newestCount":1,"rawUrl":"https://private.example"}`,
        `{"enabled":true,"newestCount":2}{}`,
    }{
        response:=req(http.MethodPut,url,body)
        if response.Code!=400{t.Fatalf("accepted policy %q: %d %s",body,response.Code,response.Body.String())}
    }
    put:=req(http.MethodPut,url,`{"enabled":true,"newestCount":2}`)
    if put.Code!=200||!bytes.Contains(put.Body.Bytes(),[]byte(`"enabled":true`))||
       !bytes.Contains(put.Body.Bytes(),[]byte(`"newestCount":2`)){
        t.Fatalf("save policy: %d %s",put.Code,put.Body.String())
    }
    get:=req(http.MethodGet,url,"")
    if get.Code!=200||!bytes.Contains(get.Body.Bytes(),[]byte(`"enabled":true`)){
        t.Fatalf("persisted policy: %d %s",get.Code,get.Body.String())
    }
    missing:=req(http.MethodPut,"/feeds/unknown/auto-download",`{"enabled":true,"newestCount":1}`)
    if missing.Code!=404{t.Fatalf("missing show was writable: %d",missing.Code)}
    if err:=d.UnsubscribePodcast(context.Background(),feedID);err!=nil{t.Fatal(err)}
    gone:=req(http.MethodGet,url,"")
    if gone.Code!=404{t.Fatalf("unfollowed show exposed policy: %d",gone.Code)}
}
