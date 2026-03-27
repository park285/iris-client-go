package client

type ReplyAcceptedResponse struct {
	Success   bool   `json:"success"`
	Delivery  string `json:"delivery"`
	RequestID string `json:"requestId"`
	Room      string `json:"room"`
	Type      string `json:"type"`
}

type ReplyStatusSnapshot struct {
	RequestID        string  `json:"requestId"`
	State            string  `json:"state"`
	UpdatedAtEpochMs int64   `json:"updatedAtEpochMs"`
	Detail           *string `json:"detail,omitempty"`
}
