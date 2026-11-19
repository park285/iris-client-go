package iris

import "time"

const (
	ReplyAdmissionRetention     = 168 * time.Hour
	ReplyReplayHorizonMargin    = 24 * time.Hour
	ReplyAutomaticReplayHorizon = ReplyAdmissionRetention - ReplyReplayHorizonMargin
)
