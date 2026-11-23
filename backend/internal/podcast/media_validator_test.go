package podcast

import "testing"

func TestPodcastStrongMediaValidators(t *testing.T){
    for _,tc:=range []struct{raw,want string}{
        {`"episode-v1"`,`"episode-v1"`},
        {`"a-b.123"`,`"a-b.123"`},
        {`""`,`""`},
        {`W/"weak"`,""},
        {"not-quoted",""},
        {"\"bad\"tail",""},
        {"\"contains\r\nInjected: yes\"",""},
    }{
        if got:=StrongMediaETag(tc.raw);got!=tc.want{
            t.Errorf("ETag %q: got %q, want %q",tc.raw,got,tc.want)
        }
    }
    if got:=stableMediaLastModified("Wed, 21 Oct 2015 07:28:00 GMT");got!="Wed, 21 Oct 2015 07:28:00 GMT"{
        t.Errorf("valid Last-Modified not retained: %q",got)
    }
    for _,bad:=range []string{"garbage","bad\nvalue",""}{
        if got:=stableMediaLastModified(bad);got!=""{t.Errorf("invalid Last-Modified %q accepted as %q",bad,got)}
    }
}
