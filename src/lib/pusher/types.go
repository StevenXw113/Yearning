package pusher

import "Yearning-go/src/model"


type messageToUserList struct {
	ToUser  []model.CoreAccount
	Message model.Message
}

type StatusType int

type Msg struct {
	orderInfo model.CoreSqlOrder
	queryInfo model.CoreQueryOrder
	orderId   string
	ll        messageToUserList
}

type OrderTPL struct {
	pushTpl string
	mailTpl string
	ll      messageToUserList
}

type QueryTPL struct {
	pushTpl string
	mailTpl string
	ll      messageToUserList
}

const (
	RejectStatus StatusType = iota
	AgreeStatus
	ExecuteStatus
	SummitStatus
	FailedStatus
	NextStepStatus
	UndoStatus
)
