package vlm

import (
	"fmt"
	"io"
)

const maxHTTPResponseBytes = int64(16 * 1024 * 1024)

func readHTTPResponseBody(body io.Reader, label string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxHTTPResponseBytes+1))
	if err != nil {
		return nil, NewError(TimeoutError, "failed to read "+label+" response", true, err)
	}
	if int64(len(data)) > maxHTTPResponseBytes {
		return nil, NewError(ParseError, fmt.Sprintf("%s response exceeds %d bytes", label, maxHTTPResponseBytes), false, nil)
	}
	return data, nil
}
