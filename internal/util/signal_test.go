package util

import (
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SignalHandler", func() {
	Describe("NewSignalHandler", func() {
		It("returns a handler whose Context is not yet cancelled", func() {
			sh := NewSignalHandler()
			Expect(sh).NotTo(BeNil())

			ctx := sh.Context()
			Expect(ctx).NotTo(BeNil())

			select {
			case <-ctx.Done():
				Fail("context should not be done before any signal or Cleanup")
			default:
			}
		})
	})

	Describe("Cleanup", func() {
		It("runs all registered cleanups in LIFO order and cancels the context", func() {
			sh := NewSignalHandler()

			var calls int32
			var order []int
			sh.RegisterCleanup(func() { atomic.AddInt32(&calls, 1); order = append(order, 1) })
			sh.RegisterCleanup(func() { atomic.AddInt32(&calls, 1); order = append(order, 2) })
			sh.RegisterCleanup(func() { atomic.AddInt32(&calls, 1); order = append(order, 3) })

			sh.Cleanup()

			Expect(atomic.LoadInt32(&calls)).To(Equal(int32(3)))
			Expect(order).To(Equal([]int{3, 2, 1}), "cleanups must run in reverse-registration order")

			select {
			case <-sh.Context().Done():
			default:
				Fail("Cleanup() must cancel the context")
			}
		})
	})
})
