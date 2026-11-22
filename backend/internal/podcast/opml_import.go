package podcast

import (
    "bytes"
    "encoding/xml"
    "errors"
    "io"
    "strings"
)

const MaxOPMLImportBytes = 512 << 10
const MaxOPMLImportFeeds = 100
const MaxOPMLImportDepth = 24

var ErrInvalidOPMLImport = errors.New("unsupported OPML import")

// OPMLImportCandidate contains only validated public URLs: never return
// private/token-bearing outlines to the renderer or retain untrusted XML.
type OPMLImportCandidate struct {
    Title string `json:"title"`
    URL string `json:"url"`
    AlreadyFollowed bool `json:"alreadyFollowed"`
}
type OPMLImportPreview struct {
    Feeds []OPMLImportCandidate `json:"feeds"`
    InvalidCount int `json:"invalidCount"`
    DuplicateCount int `json:"duplicateCount"`
}

// ParseOPMLImport extracts nested outline RSS feeds without fetching URLs.
// It bounds document length, nesting depth, feed count and individual values.
// Unsupported/private URLs are counted, never echoed or stored.
func ParseOPMLImport(data []byte) (OPMLImportPreview,error) {
    result:=OPMLImportPreview{Feeds:make([]OPMLImportCandidate,0)}
    data=bytes.TrimPrefix(data,[]byte{0xef,0xbb,0xbf})
    if len(data)==0 || len(data)>MaxOPMLImportBytes {
        return result,ErrInvalidOPMLImport
    }
    upper:=bytes.ToUpper(data)
    if bytes.Contains(upper,[]byte("<!DOCTYPE")) || bytes.Contains(upper,[]byte("<!ENTITY")) {
        return result,ErrInvalidOPMLImport
    }
    dec:=xml.NewDecoder(bytes.NewReader(data))
    dec.Strict=true
    depth:=0
    bodyDepth:=0
    rootFound:=false
    bodyFound:=false
    feedCount:=0
    seen:=map[string]bool{}
    for {
        token,err:=dec.Token()
        if errors.Is(err,io.EOF){break}
        if err!=nil{return OPMLImportPreview{},ErrInvalidOPMLImport}
        switch item:=token.(type){
        case xml.StartElement:
            depth++
            if depth>MaxOPMLImportDepth{return OPMLImportPreview{},ErrInvalidOPMLImport}
            if depth==1 {
                if item.Name.Local!="opml" || item.Name.Space!=""{
                    return OPMLImportPreview{},ErrInvalidOPMLImport
                }
                rootFound=true
                version:=""
                for _,a:=range item.Attr{if a.Name.Local=="version"{version=a.Value}}
                if version!="2.0"{return OPMLImportPreview{},ErrInvalidOPMLImport}
            }
            if depth==2 && item.Name.Local=="body" {
                bodyFound=true
                bodyDepth=depth
            }
            if bodyDepth>0 && depth>bodyDepth && item.Name.Local=="outline" {
                var title,urlStr string
                for _,a:=range item.Attr{
                    switch a.Name.Local {
                    case "xmlUrl":urlStr=a.Value
                    case "title":if title==""{title=a.Value}
                    case "text":if title==""{title=a.Value}
                    }
                }
                if urlStr==""{continue}
                feedCount++
                if feedCount>MaxOPMLImportFeeds{return OPMLImportPreview{},ErrInvalidOPMLImport}
                if len(urlStr)>2048 {
                    result.InvalidCount++
                    continue
                }
                parsed,err:=ValidatePublicURL(urlStr,false)
                if err!=nil{
                    result.InvalidCount++
                    continue
                }
                normalized:=parsed.String()
                if seen[normalized]{
                    result.DuplicateCount++
                    continue
                }
                seen[normalized]=true
                title=plainText(title,250)
                if title==""{title=parsed.Hostname()}
                result.Feeds=append(result.Feeds,OPMLImportCandidate{Title:title,URL:normalized})
            }
        case xml.EndElement:
            if depth==bodyDepth{bodyDepth=0}
            depth--
            if depth<0{return OPMLImportPreview{},ErrInvalidOPMLImport}
        case xml.Directive:
            return OPMLImportPreview{},ErrInvalidOPMLImport
        }
    }
    if !rootFound||!bodyFound||depth!=0{return OPMLImportPreview{},ErrInvalidOPMLImport}
    return result,nil
}
