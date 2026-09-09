package cli

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/config"
)

var _ = Describe("AWS context resolution", func() {
	BeforeEach(func() { globalOpts = GlobalOptions{} })

	It("leaves an unspecified context for SDK environment and shared config", func() {
		root := &cobra.Command{Use: "clerk"}
		root.PersistentFlags().StringVar(&globalOpts.Region, "region", "", "")
		root.PersistentFlags().StringVar(&globalOpts.Profile, "profile", "", "")
		cmd := &cobra.Command{Use: "get"}
		root.AddCommand(cmd)
		opts, err := resolveAWSOptions(cmd, config.DefaultConfig())
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.Region).To(BeEmpty())
		Expect(opts.ProfileSet).To(BeFalse())
	})

	It("gives explicit flags precedence and preserves explicit default profile", func() {
		root := &cobra.Command{Use: "clerk"}
		root.PersistentFlags().StringVar(&globalOpts.Region, "region", "", "")
		root.PersistentFlags().StringVar(&globalOpts.Profile, "profile", "", "")
		cmd := &cobra.Command{Use: "get"}
		root.AddCommand(cmd)
		Expect(root.PersistentFlags().Set("region", "eu-west-1")).To(Succeed())
		Expect(root.PersistentFlags().Set("profile", "default")).To(Succeed())
		opts, err := resolveAWSOptions(cmd, &config.Config{Region: "us-east-1", Profile: "production", DescribePageSize: 50})
		Expect(err).NotTo(HaveOccurred())
		Expect(opts.Region).To(Equal("eu-west-1"))
		Expect(opts.Profile).To(Equal("default"))
		Expect(opts.ProfileSet).To(BeTrue())
	})
})
