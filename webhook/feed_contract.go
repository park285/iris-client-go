package webhook

const (
	MessageTypeText        = "1"
	EventTypeKakaoFeed     = "kakao_feed"
	KakaoFeedSchemaVersion = 1
)

const (
	KakaoFeedStatusRecognized  = "recognized"
	KakaoFeedStatusUnknown     = "unknown"
	KakaoFeedStatusSchemaError = "schema_error"
	KakaoFeedStatusDecodeError = "decode_error"
)

const (
	KakaoFeedKindUserJoined         = "user_joined"
	KakaoFeedKindUserLeft           = "user_left"
	KakaoFeedKindOpenLinkUserJoined = "open_link_user_joined"
	KakaoFeedKindOpenLinkDeleted    = "open_link_deleted"
	KakaoFeedKindUserKicked         = "user_kicked"
	KakaoFeedKindModeratorAdded     = "moderator_added"
	KakaoFeedKindModeratorRemoved   = "moderator_removed"
	KakaoFeedKindMessageDeleted     = "message_deleted"
	KakaoFeedKindHostChanged        = "host_changed"
	KakaoFeedKindMessageChanged     = "message_changed"
	KakaoFeedKindMessageHidden      = "message_hidden"
	KakaoFeedKindManagerSpeakerMode = "manager_speaker_mode"
	KakaoFeedKindUnknown            = "unknown"
)
