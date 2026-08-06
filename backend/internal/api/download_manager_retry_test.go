package api

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestIsRetriableDownloadError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{fmt.Errorf("read: unexpected EOF"), true},
		{fmt.Errorf("dial: connection reset by peer"), true},
		{fmt.Errorf("request timeout"), true},
		{fmt.Errorf("permission denied"), false},
		{context.Canceled, false},
		{errors.New("invalid key size"), true},
	}
	for _, test := range tests {
		if got := isRetriableDownloadError(test.err); got != test.want {
			t.Errorf("isRetriableDownloadError(%q) = %v, want %v", test.err, got, test.want)
		}
	}
}
