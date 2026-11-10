package archery

import "errors"

// Kind 是跨 RPC 边界的错误分类，UI 依赖它决定提示与重登流程。
type Kind string

const (
	KindInvalidConfig Kind = "invalid_config"
	KindAuthRequired  Kind = "auth_required"
	KindTwoFARequired Kind = "twofa_required"
	KindForbidden     Kind = "forbidden"
	KindNotFound      Kind = "not_found"
	KindHTTP          Kind = "http_error"
	KindNetwork       Kind = "network"
	KindTimeout       Kind = "timeout"
	KindServer        Kind = "server"
	KindUnsupported   Kind = "unsupported"
)

// APIError 携带分类与可展示的消息。
type APIError struct {
	Kind    Kind
	Status  int
	Message string
}

func (e *APIError) Error() string { return e.Message }

func apiError(kind Kind, message string) *APIError {
	return &APIError{Kind: kind, Message: message}
}

func statusError(status int, message string) *APIError {
	return &APIError{Kind: KindHTTP, Status: status, Message: message}
}

// IsKind 判断错误（含包装）是否属于指定分类。
func IsKind(err error, kind Kind) bool {
	var target *APIError
	if errors.As(err, &target) {
		return target.Kind == kind
	}
	return false
}

// KindOf 返回错误分类，未知错误归为 network。
func KindOf(err error) Kind {
	var target *APIError
	if errors.As(err, &target) {
		return target.Kind
	}
	if err != nil {
		return KindNetwork
	}
	return ""
}
