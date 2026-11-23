package podcast

import (
    "encoding/xml"
    "errors"
    "fmt"
)

// OPMLSubscription is a portable, public RSS subscription. OPML export
// deliberately contains neither playback history nor private feed metadata.
type OPMLSubscription struct {
    Title string
    URL   string
}

const MaxOPMLSubscriptions = 5000
const MaxOPMLBytes = 2 << 20

var ErrInvalidOPMLExport = errors.New("invalid podcast OPML export")

type opmlOutline struct {
    XMLName xml.Name `xml:"outline"`
    Type string `xml:"type,attr"`
    Text string `xml:"text,attr"`
    Title string `xml:"title,attr"`
    XMLURL string `xml:"xmlUrl,attr"`
}

type opmlDocument struct {
    XMLName xml.Name `xml:"opml"`
    Version string `xml:"version,attr"`
    Head struct {
        Title string `xml:"title"`
    } `xml:"head"`
    Body struct {
        Outlines []opmlOutline `xml:"outline"`
    } `xml:"body"`
}

// ExportOPML encodes only public HTTPS RSS subscriptions in the OPML 2.0
// format. It fails closed on any URL that might contain a private feed
// credential; never emit a partially redacted or silently truncated list.
func ExportOPML(subscriptions []OPMLSubscription) ([]byte,error) {
    if len(subscriptions)>MaxOPMLSubscriptions {
        return nil,fmt.Errorf("%w: too many subscriptions",ErrInvalidOPMLExport)
    }
    var doc opmlDocument
    doc.Version="2.0"
    doc.Head.Title="ViiB MediaHub Podcasts"
    doc.Body.Outlines=make([]opmlOutline,0,len(subscriptions))
    for _,subscription:=range subscriptions {
        validated,err:=ValidatePublicURL(subscription.URL,false)
        if err!=nil{return nil,fmt.Errorf("%w: unsupported subscription URL",ErrInvalidOPMLExport)}
        title:=subscription.Title
        if title==""{title=validated.Hostname()}
        doc.Body.Outlines=append(doc.Body.Outlines,opmlOutline{
            Type:"rss",Text:title,Title:title,XMLURL:validated.String(),
        })
    }
    data,err:=xml.MarshalIndent(doc,"","  ")
    if err!=nil{return nil,fmt.Errorf("%w: cannot encode subscriptions",ErrInvalidOPMLExport)}
    result:=append([]byte(xml.Header),data...)
    result=append(result,byte(10))
    if len(result)>MaxOPMLBytes{
        return nil,fmt.Errorf("%w: export exceeds size limit",ErrInvalidOPMLExport)
    }
    return result,nil
}
