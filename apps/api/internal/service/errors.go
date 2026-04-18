package service

import "github.com/bizjs/kratoscarf/response"

// errNotImplemented is a placeholder returned by handlers whose business
// logic lands in a later milestone. Each TODO(Mx) call site below will
// replace this with real code. Callers see HTTP 501 + a clear message.
func errNotImplemented() error {
	return response.NewBizError(501, 50100, "not implemented yet")
}
