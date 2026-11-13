package core_http_response

type ErrorResponse struct {
	Error   string `json:"error,omitempty"`
	Message string `json:"message"`
}
