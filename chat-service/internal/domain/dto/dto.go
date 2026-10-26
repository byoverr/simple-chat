package dto

type CreateChatIn struct {
	Type      int
	Title     string
	MemberIDs []string
}

type SendMessageIn struct {
	ChatID       string
	ClientMsgID  string
	Text         string
	ReplyToMsgID string
}

type GetHistoryIn struct {
	ChatID      string
	Limit       int
	BeforeMsgID string
	BeforeSeq   int64
}
