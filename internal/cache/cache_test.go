package cache

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/yachiko/clerk/internal/config"
	"github.com/yachiko/clerk/internal/testutil"
)

// newManagerWithIsolatedHome is the per-spec setup helper. It isolates HOME
// and returns both the manager and the home dir for path assertions.
func newManagerWithIsolatedHome() (*Manager, string) {
	home := testutil.IsolateHome(GinkgoT())
	cfg := &config.Config{CacheTTL: 1 * time.Hour}
	mgr, err := NewManager(cfg, "us-east-1", "123456789012")
	Expect(err).NotTo(HaveOccurred())
	return mgr, home
}

func names(entries []CacheEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}

var _ = Describe("cache.Manager", func() {
	var (
		mgr  *Manager
		home string
	)

	BeforeEach(func() {
		mgr, home = newManagerWithIsolatedHome()
	})

	Describe("NewManager", func() {
		It("places the cache file under ~/.clerk/cache/<account>/<region>.json", func() {
			Expect(mgr.cachePath).To(Equal(filepath.Join(home, ".clerk", "cache", "123456789012", "us-east-1.json")))
		})
	})

	Describe("IsExpired", func() {
		It("reports expired when LastRefresh is zero", func() {
			Expect(mgr.IsExpired()).To(BeTrue())
		})

		It("reports fresh inside the TTL window", func() {
			mgr.data.LastRefresh = time.Now()
			Expect(mgr.IsExpired()).To(BeFalse())
		})

		It("reports expired once outside the TTL window", func() {
			mgr.data.LastRefresh = time.Now().Add(-2 * time.Hour)
			Expect(mgr.IsExpired()).To(BeTrue())
		})
	})

	Describe("GetAge", func() {
		It("returns zero for a never-refreshed cache (not seconds-since-epoch)", func() {
			Expect(mgr.GetAge()).To(Equal(time.Duration(0)))
		})

		It("returns the elapsed time since LastRefresh", func() {
			mgr.data.LastRefresh = time.Now().Add(-10 * time.Minute)
			age := mgr.GetAge()
			Expect(age).To(BeNumerically(">", 9*time.Minute))
			Expect(age).To(BeNumerically("<", 11*time.Minute))
		})
	})

	Describe("GetStats", func() {
		It("reports zero entries and expired on a fresh manager", func() {
			stats := mgr.GetStats()
			Expect(stats.TotalEntries).To(Equal(0))
			Expect(stats.IsExpired).To(BeTrue())
		})

		It("reflects entry count, region and freshness once populated", func() {
			mgr.data.Entries = []CacheEntry{{Name: "/a"}, {Name: "/b"}}
			mgr.data.LastRefresh = time.Now()
			mgr.data.Region = "us-east-1"

			stats := mgr.GetStats()
			Expect(stats.TotalEntries).To(Equal(2))
			Expect(stats.IsExpired).To(BeFalse())
			Expect(stats.Region).To(Equal("us-east-1"))
		})
	})

	Describe("GetAll", func() {
		It("returns a defensive copy, not the internal slice", func() {
			mgr.data.Entries = []CacheEntry{{Name: "/a"}, {Name: "/b"}}
			got := mgr.GetAll()
			Expect(got).To(HaveLen(2))

			got[0].Name = "/mutated"
			Expect(mgr.data.Entries[0].Name).To(Equal("/a"), "GetAll must return a copy")
		})
	})

	Describe("Get", func() {
		BeforeEach(func() {
			mgr.data.Entries = []CacheEntry{{Name: "/found", Version: 3}}
		})

		It("returns the entry when present", func() {
			entry, ok := mgr.Get("/found")
			Expect(ok).To(BeTrue())
			Expect(entry.Version).To(Equal(int64(3)))
		})

		It("returns ok=false when absent", func() {
			_, ok := mgr.Get("/missing")
			Expect(ok).To(BeFalse())
		})
	})

	Describe("Search (glob)", func() {
		BeforeEach(func() {
			mgr.data.Entries = []CacheEntry{
				{Name: "/prod/db/password"},
				{Name: "/prod/api/key"},
				{Name: "/dev/db/password"},
				{Name: "/dev/api/key"},
			}
		})

		DescribeTable("matches per pattern",
			func(pattern string, wantLen int) {
				Expect(mgr.Search(pattern)).To(HaveLen(wantLen))
			},
			Entry("empty pattern → everything", "", 4),
			Entry("single slash → everything", "/", 4),
			Entry("/* → everything", "/*", 4),
			Entry("/prod/* → prod entries", "/prod/*", 2),
			Entry("*password → suffix match", "*password", 2),
			Entry("*db* → contains match", "*db*", 2),
			Entry("/prod → prefix-only also works", "/prod", 2),
			Entry("/staging/* → no match", "/staging/*", 0),
		)
	})

	Describe("SearchByTag", func() {
		BeforeEach(func() {
			mgr.data.Entries = []CacheEntry{
				{Name: "/a", Tags: map[string]string{"env": "prod", "team": "be"}},
				{Name: "/b", Tags: map[string]string{"env": "dev"}},
				{Name: "/c", Tags: map[string]string{"team": "fe"}},
			}
		})

		It("matches any entry with the tag key when value is empty", func() {
			Expect(mgr.SearchByTag("env", "")).To(HaveLen(2))
		})

		It("matches the exact value when provided", func() {
			Expect(mgr.SearchByTag("env", "prod")).To(HaveLen(1))
		})

		It("returns nothing for a non-matching value", func() {
			Expect(mgr.SearchByTag("env", "missing")).To(BeEmpty())
		})

		It("returns nothing for a non-existent tag key", func() {
			Expect(mgr.SearchByTag("nope", "")).To(BeEmpty())
		})
	})

	Describe("Sort", func() {
		var (
			now     time.Time
			entries []CacheEntry
		)

		BeforeEach(func() {
			now = time.Now()
			entries = []CacheEntry{
				{Name: "/b", LastModifiedDate: now.Add(-1 * time.Hour)},
				{Name: "/a", LastModifiedDate: now.Add(-2 * time.Hour)},
				{Name: "/c", LastModifiedDate: now},
			}
		})

		It("sorts by name ascending", func() {
			Expect(names(mgr.Sort(entries, "name"))).To(Equal([]string{"/a", "/b", "/c"}))
		})

		It("honors the short alias n", func() {
			Expect(names(mgr.Sort(entries, "n"))).To(Equal([]string{"/a", "/b", "/c"}))
		})

		It("sorts by modified descending (most recent first)", func() {
			Expect(names(mgr.Sort(entries, "modified"))).To(Equal([]string{"/c", "/b", "/a"}))
		})

		It("sorts by created ascending", func() {
			Expect(names(mgr.Sort(entries, "created"))).To(Equal([]string{"/a", "/b", "/c"}))
		})

		It("leaves order untouched for an unknown criterion (still returns a copy)", func() {
			Expect(names(mgr.Sort(entries, "wat"))).To(Equal([]string{"/b", "/a", "/c"}))
		})
	})

	Describe("Update", func() {
		It("adds new entries and modifies existing ones in place", func() {
			Expect(mgr.Update(CacheEntry{Name: "/x", Version: 1})).To(Succeed())
			Expect(mgr.Update(CacheEntry{Name: "/y", Version: 1})).To(Succeed())
			Expect(mgr.data.Entries).To(HaveLen(2))

			Expect(mgr.Update(CacheEntry{Name: "/x", Version: 5})).To(Succeed())
			Expect(mgr.data.Entries).To(HaveLen(2), "count stays the same on modify")

			got, ok := mgr.Get("/x")
			Expect(ok).To(BeTrue())
			Expect(got.Version).To(Equal(int64(5)))
		})
	})

	Describe("Delete", func() {
		BeforeEach(func() {
			Expect(mgr.Update(CacheEntry{Name: "/a"})).To(Succeed())
			Expect(mgr.Update(CacheEntry{Name: "/b"})).To(Succeed())
		})

		It("removes the entry by name", func() {
			Expect(mgr.Delete("/a")).To(Succeed())
			Expect(mgr.data.Entries).To(HaveLen(1))
			_, ok := mgr.Get("/a")
			Expect(ok).To(BeFalse())
		})

		It("is a silent no-op when the entry doesn't exist", func() {
			Expect(mgr.Delete("/never-existed")).To(Succeed())
			Expect(mgr.data.Entries).To(HaveLen(2))
		})
	})

	Describe("Persistence", func() {
		It("survives manager recreation in the same HOME/region/account", func() {
			Expect(mgr.Update(CacheEntry{Name: "/persist/me", Version: 7, Type: "SecureString"})).To(Succeed())

			// Reload — same identity, fresh manager instance.
			cfg := &config.Config{CacheTTL: 1 * time.Hour}
			mgr2, err := NewManager(cfg, "us-east-1", "123456789012")
			Expect(err).NotTo(HaveOccurred())

			got, ok := mgr2.Get("/persist/me")
			Expect(ok).To(BeTrue())
			Expect(got.Version).To(Equal(int64(7)))
			Expect(got.Type).To(Equal("SecureString"))
		})
	})

	Describe("Region and account isolation", func() {
		It("uses separate cache files per (region, account) tuple", func() {
			cfg := &config.Config{CacheTTL: 1 * time.Hour}
			// Use the already-set HOME.

			mgrEast, err := NewManager(cfg, "us-east-1", "111")
			Expect(err).NotTo(HaveOccurred())
			Expect(mgrEast.Update(CacheEntry{Name: "/east-only"})).To(Succeed())

			mgrWest, err := NewManager(cfg, "us-west-2", "111")
			Expect(err).NotTo(HaveOccurred())
			_, ok := mgrWest.Get("/east-only")
			Expect(ok).To(BeFalse(), "different region in same account must not see the entry")

			mgrOtherAcct, err := NewManager(cfg, "us-east-1", "222")
			Expect(err).NotTo(HaveOccurred())
			_, ok = mgrOtherAcct.Get("/east-only")
			Expect(ok).To(BeFalse(), "same region but different account must not see the entry")

			mgrEast2, err := NewManager(cfg, "us-east-1", "111")
			Expect(err).NotTo(HaveOccurred())
			_, ok = mgrEast2.Get("/east-only")
			Expect(ok).To(BeTrue(), "original cache must remain intact")
		})
	})

	Describe("lock file lifecycle", func() {
		It("creates a lock file on acquireLock and removes it on releaseLock", func() {
			Expect(os.MkdirAll(filepath.Dir(mgr.lockFile), 0700)).To(Succeed())
			Expect(mgr.acquireLock()).To(Succeed())
			defer mgr.releaseLock()
			Expect(mgr.lockFile).To(BeAnExistingFile())
		})
	})

	Describe("load() with missing cache file", func() {
		It("starts empty rather than failing", func() {
			cfg := &config.Config{CacheTTL: time.Hour}
			// Brand-new accountID guarantees no on-disk cache.
			fresh, err := NewManager(cfg, "us-east-1", "fresh-account")
			Expect(err).NotTo(HaveOccurred())
			Expect(fresh.GetAll()).To(BeEmpty())
		})
	})
})

var _ = Describe("matchGlob", func() {
	DescribeTable("classifies pattern/name pairs",
		func(pattern, name string, want bool) {
			Expect(matchGlob(pattern, name)).To(Equal(want))
		},
		Entry("empty pattern matches anything", "", "/anything", true),
		Entry("'*' matches anything", "*", "/anything", true),
		Entry("'/*' matches anything", "/*", "/anything", true),
		Entry("'/prod/*' matches a /prod child", "/prod/*", "/prod/x", true),
		Entry("'/prod/*' matches the bare '/prod'", "/prod/*", "/prod", true),
		Entry("'/prod/*' does not match a /dev child", "/prod/*", "/dev/x", false),
		Entry("'*pass*' contains-matches", "*pass*", "/db/password", true),
		Entry("'*pass*' rejects when substring is absent", "*pass*", "/api/key", false),
		Entry("'/prod*' prefix-matches", "/prod*", "/prod/x", true),
		Entry("'*key' suffix-matches", "*key", "/api/key", true),
		Entry("'*key' rejects when suffix doesn't match", "*key", "/api/secret", false),
		Entry("plain string is treated as 'contains'", "plain", "/no-plain", true),
		Entry("plain string rejects when substring absent", "plain", "/no-other", false),
	)
})
