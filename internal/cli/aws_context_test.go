package cli

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/config"
)

var _ = Describe("AWS context resolution", func() {
	var root, cmd *cobra.Command

	BeforeEach(func() {
		globalOpts = GlobalOptions{}
		for _, key := range []string{"AWS_ACCESS_KEY_ID", "AWS_ACCESS_KEY", "AWS_SECRET_ACCESS_KEY", "AWS_SECRET_KEY"} {
			GinkgoT().Setenv(key, "")
		}
		root = &cobra.Command{Use: "clerk"}
		root.PersistentFlags().StringVar(&globalOpts.Region, "region", "", "")
		root.PersistentFlags().StringVar(&globalOpts.Profile, "profile", "", "")
		cmd = &cobra.Command{Use: "get"}
		root.AddCommand(cmd)
	})

	It("leaves an unspecified context for SDK environment and shared config", func() {
		opts, err := resolveAWSOptions(cmd, config.DefaultConfig())
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.Region).To(BeEmpty())
		Expect(opts.ProfileSet).To(BeFalse())
	})

	It("gives explicit flags precedence and preserves explicit default profile", func() {
		Expect(root.PersistentFlags().Set("region", "eu-west-1")).To(Succeed())
		Expect(root.PersistentFlags().Set("profile", "default")).To(Succeed())
		opts, err := resolveAWSOptions(cmd, &config.Config{Region: "us-east-1", Profile: "production", DescribePageSize: 50})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.Region).To(Equal("eu-west-1"))
		Expect(opts.Profile).To(Equal("default"))
		Expect(opts.ProfileSet).To(BeTrue())
	})

	It("applies a saved profile when the environment has no static credentials", func() {
		opts, err := resolveAWSOptions(cmd, &config.Config{Profile: "default"})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.Profile).To(Equal("default"))
		Expect(opts.ProfileSet).To(BeTrue())
	})

	DescribeTable("lets environment credentials override a saved profile",
		func(idKey, secretKey string) {
			GinkgoT().Setenv(idKey, "AKIAEXAMPLE")
			GinkgoT().Setenv(secretKey, "secret")
			opts, err := resolveAWSOptions(cmd, &config.Config{Region: "us-west-2", Profile: "default"})
			Expect(err).NotTo(HaveOccurred())
			Expect(opts.Region).To(Equal("us-west-2"))
			Expect(opts.ProfileSet).To(BeFalse())
		},
		Entry("standard variables", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"),
		Entry("legacy variables", "AWS_ACCESS_KEY", "AWS_SECRET_KEY"),
	)

	It("keeps a saved profile when only part of a key pair is in the environment", func() {
		GinkgoT().Setenv("AWS_ACCESS_KEY_ID", "AKIAEXAMPLE")
		opts, err := resolveAWSOptions(cmd, &config.Config{Profile: "default"})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.ProfileSet).To(BeTrue())
	})

	It("lets an explicit --profile override environment credentials", func() {
		GinkgoT().Setenv("AWS_ACCESS_KEY_ID", "AKIAEXAMPLE")
		GinkgoT().Setenv("AWS_SECRET_ACCESS_KEY", "secret")
		Expect(root.PersistentFlags().Set("profile", "production")).To(Succeed())
		opts, err := resolveAWSOptions(cmd, &config.Config{Profile: "default"})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.Profile).To(Equal("production"))
		Expect(opts.ProfileSet).To(BeTrue())
	})
})
