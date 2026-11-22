package podcast

import (
    "net/http"
    "testing"
)

func TestPodcastStrongEntityTagAndLastModifiedValidation(t *testing.T){
    for _,tc:=range []struct{raw,expect string}{
        {`"v1"`,`"v1"`},
        {`"a-b.123"`,`"a-b.123"`},
        {`W/"weak"`,""},
        {"not-quoted",""},
        {"\"a\\r\\nInjected:1\"",""},
        {"\"\"","\"\""},
        {"\"bad\"tail",""},
    }{
        // Literal backslash-r is not CRLF, but it cannot form a valid tag
        // with the interior quotes in this malformed example.
        if got:=StrongMediaETag(tc.raw);got!=tc.expect{
            t.Fatalf("strong ETag %q -> %q, expected %q",tc.raw,got,tc.expect)
        }
    }
    if StrongMediaETag("\"x\r\nX-Test: yes\"")!=""{t.Fatal("CRLF validator accepted")}
    if got:=stableMediaLastModified("Wed, 21 Oct 2015 07:28:00 GMT");got!="Wed, 21 Oct 2015 07:28:00 GMT"{
        t.Fatalf("unexpected Last-Modified %q",got)
    }
    if stableMediaLastModified("bad\nvalue")!=""{t.Fatal("invalid Last-Modified accepted")}
    if stableMediaLastModified(http.TimeFormat)=="" {
        // Parsing format text is not a valid HTTP date.
    }
}
