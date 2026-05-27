package cache

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CacheEntry", func() {
	It("stores all fields including version history and tags", func() {
		now := time.Now()
		entry := CacheEntry{
			Name:             "/test/param",
			Type:             "SecureString",
			Version:          1,
			LastModifiedDate: now,
			Tags:             map[string]string{"env": "test"},
			VersionHistory: []VersionHistoryEntry{
				{Version: 1, Modified: now},
			},
		}

		Expect(entry.Name).To(Equal("/test/param"))
		Expect(entry.Type).To(Equal("SecureString"))
		Expect(entry.Version).To(Equal(int64(1)))
		Expect(entry.LastModifiedDate).To(Equal(now))
		Expect(entry.Tags).To(HaveKeyWithValue("env", "test"))
		Expect(entry.VersionHistory).To(HaveLen(1))
	})
})

var _ = Describe("CacheData", func() {
	It("carries refresh timestamp, region and entries", func() {
		now := time.Now()
		data := CacheData{
			LastRefresh: now,
			Region:      "us-east-1",
			Entries: []CacheEntry{
				{Name: "/test/p1"},
				{Name: "/test/p2"},
			},
		}

		Expect(data.LastRefresh).To(Equal(now))
		Expect(data.Region).To(Equal("us-east-1"))
		Expect(data.Entries).To(HaveLen(2))
	})
})

var _ = Describe("CacheStats", func() {
	It("reports counts, freshness and region", func() {
		now := time.Now()
		stats := CacheStats{
			TotalEntries: 100,
			LastRefresh:  now,
			IsExpired:    false,
			Region:       "us-west-2",
		}

		Expect(stats.TotalEntries).To(Equal(100))
		Expect(stats.LastRefresh).To(Equal(now))
		Expect(stats.IsExpired).To(BeFalse())
		Expect(stats.Region).To(Equal("us-west-2"))
	})
})
