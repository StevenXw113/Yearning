package apis

import (
	"Yearning-go/src/handler/personal"
	"Yearning-go/src/lib/factory"
	"github.com/cookieY/yee"
	"net/http"
)

func YearningQueryForGet(y yee.Context) (err error) {
	tp := y.Params("tp")
	switch tp {
	case "tables":
		return personal.FetchQueryTableInfo(y)
	case "schema":
		return personal.FetchQueryDatabaseInfo(y)
	case "results":
		return personal.SocketQueryResults(y)
	}
	return y.JSON(http.StatusOK, "Illegal")
}

func YearningQueryForPost(y yee.Context) (err error) {
	tp := y.Params("tp")
	user := new(factory.Token).JwtParse(y)
	switch tp {
	case "post":
		return personal.ReferQueryOrder(y, user)
	}
	return y.JSON(http.StatusOK, "Illegal")
}

func YearningQueryApis() yee.RestfulAPI {
	return yee.RestfulAPI{
		Get:    YearningQueryForGet,
		Post:   YearningQueryForPost,
		Delete: personal.UndoQueryOrder,
	}
}
