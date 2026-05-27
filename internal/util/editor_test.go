package util

import (
	"os"
	"os/exec"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/yachiko/clerk/internal/testutil"
)

var _ = Describe("Editor", func() {
	Describe("NewEditor", func() {
		It("stores the supplied config", func() {
			e := NewEditor(EditorConfig{PreferredEditor: "nvim"})
			Expect(e).NotTo(BeNil())
			Expect(e.config.PreferredEditor).To(Equal("nvim"))
		})
	})

	Describe("getEditor preference order", func() {
		BeforeEach(func() {
			// Start every test from a known empty state.
			_ = os.Unsetenv("EDITOR")
			_ = os.Unsetenv("VISUAL")
		})

		It("uses PreferredEditor when set, even if EDITOR/VISUAL exist", func() {
			testutil.SetEnv(GinkgoT(), "EDITOR", "vim")
			testutil.SetEnv(GinkgoT(), "VISUAL", "code")
			e := NewEditor(EditorConfig{PreferredEditor: "nano"})
			Expect(e.getEditor()).To(Equal("nano"))
		})

		It("falls back to EDITOR when PreferredEditor is empty", func() {
			testutil.SetEnv(GinkgoT(), "EDITOR", "vim")
			_ = os.Unsetenv("VISUAL")
			e := NewEditor(EditorConfig{})
			Expect(e.getEditor()).To(Equal("vim"))
		})

		It("falls back to VISUAL when EDITOR is unset", func() {
			_ = os.Unsetenv("EDITOR")
			testutil.SetEnv(GinkgoT(), "VISUAL", "code --wait")
			e := NewEditor(EditorConfig{})
			Expect(e.getEditor()).To(Equal("code --wait"))
		})
	})

	Describe("createSecureTempFile + secureDelete", func() {
		It("creates a 0600 file and removes it on secureDelete", func() {
			tmp := testutil.TempDir(GinkgoT())
			e := NewEditor(EditorConfig{TempDir: tmp})

			path, err := e.createSecureTempFile("hello world", ".txt")
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(BeAnExistingFile())

			info, err := os.Stat(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))

			Expect(e.secureDelete(path)).To(Succeed())
			_, err = os.Stat(path)
			Expect(os.IsNotExist(err)).To(BeTrue())
		})
	})

	Describe("Edit (round-trip with a no-op editor)", func() {
		// Use PATH lookup rather than a hardcoded /bin/true so the test runs on
		// any unix-y system that ships a `true` binary somewhere on PATH.
		It("returns the original content when the editor exits 0 without writing", func() {
			if runtime.GOOS == "windows" {
				Skip("relies on a unix 'true' binary on PATH")
			}
			truePath, err := exec.LookPath("true")
			if err != nil {
				Skip("no 'true' binary on PATH")
			}

			tmp := testutil.TempDir(GinkgoT())
			e := NewEditor(EditorConfig{PreferredEditor: truePath, TempDir: tmp})

			got, err := e.Edit("payload", ".txt")
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal("payload"))
		})
	})

	Describe("GetEditorName", func() {
		BeforeEach(func() {
			_ = os.Unsetenv("EDITOR")
			_ = os.Unsetenv("VISUAL")
		})

		It("strips path components and command-line args", func() {
			e := NewEditor(EditorConfig{PreferredEditor: "/usr/bin/nvim --headless"})
			Expect(e.GetEditorName()).To(Equal("nvim"))
		})

		It("reports 'none' when no editor is configured and PATH yields nothing", func() {
			if runtime.GOOS != "linux" {
				Skip("linux-only: fallback logic looks up nano/vi on PATH")
			}
			testutil.SetEnv(GinkgoT(), "PATH", "")
			e := NewEditor(EditorConfig{})
			Expect(e.GetEditorName()).To(Equal("none"))
		})
	})
})
