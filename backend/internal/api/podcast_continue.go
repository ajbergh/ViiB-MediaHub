package api

import (
    "net/http"
    "strconv"
)

func (a *API) listContinueListening(w http.ResponseWriter,r *http.Request){
    limit:=20
    if raw:=r.URL.Query().Get("limit");raw!=""{
        n,err:=strconv.Atoi(raw)
        if err!=nil||n<1||n>50{
            respondV2Error(w,r,http.StatusBadRequest,"invalid_pagination","Continue Listening limit must be between 1 and 50",false,nil)
            return
        }
        limit=n
    }
    items,err:=a.db.ListContinueListening(r.Context(),limit)
    if err!=nil{
        respondV2Error(w,r,http.StatusInternalServerError,"podcast_library_unavailable",
            "Unable to read listening history",true,nil)
        return
    }
    respondV2JSON(w,http.StatusOK,map[string]any{"episodes":items})
}
