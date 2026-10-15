package audio

import (
	"testing"

	"github.com/ajbergh/viib-mediahub/internal/releasedate"
)

func TestOriginalDateTagPriorityAndPrecision(t *testing.T) {
	tags := map[string][]string{"DATE": {"2024-03-04"}, "originaldate": {"1989-02"}, "ORIGINALYEAR": {"1990"}, "TORY": {"1991"}}
	date, precision, year := releasedate.Parse(getOriginalDateTag(tags))
	if date != "1989-02" || precision != "month" || year != 1989 {
		t.Fatalf("original tag lost: %s %s %d", date, precision, year)
	}
	if getOriginalDateTag(map[string][]string{"DATE": {"2024"}}) != "" {
		t.Fatal("edition date promoted to original")
	}
}
