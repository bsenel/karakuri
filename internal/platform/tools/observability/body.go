package observability

import (
	"fmt"
	"io"
)

// maxResponseBody bounds how much of a backend's response an adapter reads.
const maxResponseBody = 8 << 20

// readBounded reads r up to maxResponseBody. A larger body is an error rather
// than a truncated read, so a partial response is never parsed as a whole one.
func readBounded(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxResponseBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxResponseBody {
		return nil, fmt.Errorf("response body exceeds %d bytes", maxResponseBody)
	}
	return body, nil
}
