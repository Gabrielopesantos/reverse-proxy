/*
Source: https://stackoverflow.com/questions/53272536/how-do-i-get-response-statuscode-in-golang-middleware
This implementation has some issues as `loggingResponseWriter` doesn't implement
multiple interfaces (`CloseNotifier`, Flusher`, etc) that might be used.
Better example: https://github.com/urfave/negroni/blob/master/response_writer.go
*/
package middleware

import (
	"net/http"
)

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func NewLoggingResponseWriter(w http.ResponseWriter) *loggingResponseWriter {
	return &loggingResponseWriter{
		ResponseWriter: w,
	}
}

func (lrw *loggingResponseWriter) WriteHeader(statusCode int) {
	lrw.statusCode = statusCode
	lrw.ResponseWriter.WriteHeader(statusCode)
}
