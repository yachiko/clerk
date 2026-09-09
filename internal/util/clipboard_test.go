package util

import (
	"errors"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeClipboard struct {
	mu                sync.Mutex
	value             string
	readErr, writeErr error
	writes            []string
}

func (f *fakeClipboard) WriteAll(v string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.writeErr != nil {
		return f.writeErr
	}
	f.value = v
	f.writes = append(f.writes, v)
	return nil
}
func (f *fakeClipboard) ReadAll() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.readErr != nil {
		return "", f.readErr
	}
	return f.value, nil
}
func (f *fakeClipboard) Value() string { f.mu.Lock(); defer f.mu.Unlock(); return f.value }

type fakeClock struct{ channels []chan time.Time }

func (c *fakeClock) After(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	c.channels = append(c.channels, ch)
	return ch
}

var _ = Describe("ClipboardManager", func() {
	var backend *fakeClipboard
	var clock *fakeClock
	var cm *ClipboardManager
	BeforeEach(func() {
		backend = &fakeClipboard{}
		clock = &fakeClock{}
		cm = newClipboardManager(time.Second, backend, clock.After)
	})
	It("expires a Clerk-owned value", func() {
		Expect(cm.Copy("secret")).To(Succeed())
		Eventually(func() int { return len(clock.channels) }).Should(Equal(1))
		clock.channels[0] <- time.Now()
		Eventually(backend.Value).Should(BeEmpty())
	})
	It("does not clear a replacement copied by another application", func() {
		Expect(cm.Copy("secret")).To(Succeed())
		Expect(backend.WriteAll("other")).To(Succeed())
		Eventually(func() int { return len(clock.channels) }).Should(Equal(1))
		clock.channels[0] <- time.Now()
		Consistently(backend.Value).Should(Equal("other"))
	})
	It("does not let an old timer clear a newer copy", func() {
		Expect(cm.Copy("one")).To(Succeed())
		Expect(cm.Copy("two")).To(Succeed())
		Eventually(func() int { return len(clock.channels) }).Should(Equal(2))
		clock.channels[0] <- time.Now()
		Consistently(backend.Value).Should(Equal("two"))
		clock.channels[1] <- time.Now()
		Eventually(backend.Value).Should(BeEmpty())
	})
	It("clears owned content on close and rejects later copies", func() {
		Expect(cm.Copy("secret")).To(Succeed())
		Expect(cm.Close()).To(Succeed())
		Expect(backend.Value()).To(BeEmpty())
		Expect(cm.Copy("later")).To(MatchError(ContainSubstring("closed")))
	})
	It("never clears when reading ownership fails", func() {
		Expect(cm.Copy("secret")).To(Succeed())
		backend.readErr = errors.New("read failed")
		Eventually(func() int { return len(clock.channels) }).Should(Equal(1))
		clock.channels[0] <- time.Now()
		Consistently(backend.Value).Should(Equal("secret"))
	})
	It("retains previous ownership when a replacement write fails", func() {
		Expect(cm.Copy("one")).To(Succeed())
		backend.writeErr = errors.New("write failed")
		Expect(cm.Copy("two")).To(HaveOccurred())
		backend.writeErr = nil
		Eventually(func() int { return len(clock.channels) }).Should(Equal(1))
		clock.channels[0] <- time.Now()
		Eventually(backend.Value).Should(BeEmpty())
	})
})
