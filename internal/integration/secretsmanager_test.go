//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/yachiko/clerk/internal/testutil"
)

const secretsManagerBackend = "--backend=secretsmanager"

func secretVersionID(output string) string {
	var result struct {
		VersionID string `json:"version_id"`
	}
	Expect(json.Unmarshal([]byte(output), &result)).To(Succeed())
	Expect(result.VersionID).NotTo(BeEmpty())
	return result.VersionID
}

func motoUnsupportedOperation(stderr string) bool {
	message := strings.ToLower(stderr)
	return strings.Contains(message, "not implemented") ||
		strings.Contains(message, "notimplemented") ||
		strings.Contains(message, "status code: 501")
}

var _ = Describe("clerk Secrets Manager against moto", func() {
	var home string

	BeforeEach(func() {
		Expect(testutil.ResetMoto(integrationCfg.MotoEndpoint)).To(Succeed())
		home = GinkgoT().TempDir()
	})

	It("creates, updates, and reads text versions by ID and stage", func() {
		stdout, stderr, err := run30s(home, "put", "integration/text", "first", secretsManagerBackend, "--output=json")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		firstVersion := secretVersionID(stdout)

		stdout, stderr, err = run30s(home, "put", "integration/text", "second", secretsManagerBackend, "--output=json")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		Expect(secretVersionID(stdout)).NotTo(Equal(firstVersion))

		stdout, stderr, err = run30s(home, "get", "integration/text", secretsManagerBackend, "--version-id", firstVersion, "--value")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		Expect(stdout).To(Equal("first"))

		stdout, stderr, err = run30s(home, "get", "integration/text", secretsManagerBackend, "--stage", "AWSCURRENT", "--value")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		Expect(stdout).To(Equal("second"))
	})

	It("accepts file:// text and fileb:// binary values", func() {
		textPath := filepath.Join(home, "secret.txt")
		binaryPath := filepath.Join(home, "secret.bin")
		Expect(os.WriteFile(textPath, []byte("text from file\n"), 0600)).To(Succeed())
		binary := []byte{0x00, 0x01, 0xfe, 0xff, 'b', 'i', 'n'}
		Expect(os.WriteFile(binaryPath, binary, 0600)).To(Succeed())

		_, stderr, err := run30s(home, "put", "integration/file-text", "file://"+textPath, secretsManagerBackend)
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		stdout, stderr, err := run30s(home, "get", "integration/file-text", secretsManagerBackend, "--value")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		Expect(stdout).To(Equal("text from file\n"))

		_, stderr, err = run30s(home, "put", "integration/file-binary", "fileb://"+binaryPath, secretsManagerBackend)
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		stdout, stderr, err = run30s(home, "get", "integration/file-binary", secretsManagerBackend, "--value", "--raw")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		Expect([]byte(stdout)).To(Equal(binary))
	})

	It("tags and untags a secret", func() {
		_, stderr, err := run30s(home, "put", "integration/tags", "value", secretsManagerBackend, "--tags", "env=test,team=backend")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		_, stderr, err = run30s(home, "tag", "integration/tags", "owner=clerk", secretsManagerBackend)
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		_, stderr, err = run30s(home, "untag", "integration/tags", "team", secretsManagerBackend)
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		stdout, stderr, err := run30s(home, "list", "integration/tags", secretsManagerBackend, "--tags", "--output=json")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		Expect(stdout).To(SatisfyAll(
			ContainSubstring(`"env": "test"`),
			ContainSubstring(`"owner": "clerk"`),
			Not(ContainSubstring(`"team"`)),
		))
	})

	It("schedules deletion with a recovery window and restores the secret", func() {
		_, stderr, err := run30s(home, "put", "integration/recovery", "recoverable", secretsManagerBackend)
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		stdout, stderr, err := run30s(home, "delete", "integration/recovery", secretsManagerBackend, "--recovery-window", "7", "--force")
		if err != nil && motoUnsupportedOperation(stderr) {
			Skip("Moto does not implement Secrets Manager scheduled deletion: " + stderr)
		}
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		Expect(stdout).To(ContainSubstring("Scheduled deletion"))

		_, _, err = run30s(home, "get", "integration/recovery", secretsManagerBackend, "--value")
		Expect(err).To(HaveOccurred(), "scheduled secrets must not be readable")

		stdout, stderr, err = run30s(home, "restore", "integration/recovery", secretsManagerBackend)
		if err != nil && motoUnsupportedOperation(stderr) {
			Skip("Moto does not implement Secrets Manager restore: " + stderr)
		}
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		Expect(stdout).To(ContainSubstring("Restored secret"))

		stdout, stderr, err = run30s(home, "get", "integration/recovery", secretsManagerBackend, "--value")
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		Expect(stdout).To(Equal("recoverable"))
	})

	It("lists and refreshes only Secrets Manager metadata", func() {
		_, stderr, err := run30s(home, "put", "integration/listed", "value", secretsManagerBackend)
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)

		stdout, stderr, err := run30s(home, "list", "integration/*", secretsManagerBackend)
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		Expect(stdout).To(ContainSubstring("integration/listed"))

		stdout, stderr, err = run30s(home, "refresh", secretsManagerBackend)
		Expect(err).NotTo(HaveOccurred(), "stderr: %s", stderr)
		Expect(strings.ToLower(stdout)).To(ContainSubstring("refresh"))
	})
})
