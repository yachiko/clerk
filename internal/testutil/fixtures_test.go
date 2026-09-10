package testutil

import (
	"math/rand"
	"testing"
)

func TestRandomFixtureVersionCountIsBounded(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for range 1000 {
		count := randomFixtureVersionCount(rng)
		if count < minFixtureVersions || count > maxFixtureVersions {
			t.Fatalf("version count %d outside [%d, %d]", count, minFixtureVersions, maxFixtureVersions)
		}
	}
}
