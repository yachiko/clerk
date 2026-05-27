package aws

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Parameter", func() {
	It("carries all SSM parameter fields", func() {
		now := time.Now()
		p := Parameter{
			Name:             "/test/secret",
			Value:            "secret-value",
			Type:             "SecureString",
			Version:          3,
			LastModifiedDate: now,
			ARN:              "arn:aws:ssm:us-east-1:123456789012:parameter/test/secret",
			DataType:         "text",
			Tags:             map[string]string{"env": "test"},
		}

		Expect(p.Name).To(Equal("/test/secret"))
		Expect(p.Value).To(Equal("secret-value"))
		Expect(p.Type).To(Equal("SecureString"))
		Expect(p.Version).To(Equal(int64(3)))
		Expect(p.LastModifiedDate).To(Equal(now))
		Expect(p.ARN).To(ContainSubstring("parameter/test/secret"))
		Expect(p.DataType).To(Equal("text"))
		Expect(p.Tags).To(HaveKeyWithValue("env", "test"))
	})
})

var _ = Describe("ParameterMetadata", func() {
	It("carries name, type, version, modified, tags (no value)", func() {
		now := time.Now()
		m := ParameterMetadata{
			Name:             "/test/p",
			Type:             "String",
			Version:          1,
			LastModifiedDate: now,
			Tags:             map[string]string{"team": "backend"},
		}

		Expect(m.Name).To(Equal("/test/p"))
		Expect(m.Version).To(Equal(int64(1)))
		Expect(m.Tags).To(HaveKeyWithValue("team", "backend"))
	})
})

var _ = Describe("ParameterHistory", func() {
	It("carries the version-specific value and any labels", func() {
		now := time.Now()
		h := ParameterHistory{
			Name:             "/test/p",
			Value:            "old",
			Type:             "SecureString",
			Version:          2,
			LastModifiedDate: now,
			Labels:           []string{"prod", "stable"},
		}

		Expect(h.Version).To(Equal(int64(2)))
		Expect(h.Labels).To(ConsistOf("prod", "stable"))
	})
})

var _ = Describe("PutParameter input/output types", func() {
	It("round-trips PutParameterInput", func() {
		in := PutParameterInput{
			Name:      "/prod/secret",
			Value:     "shhh",
			Type:      "SecureString",
			Overwrite: true,
			KMSKeyID:  "alias/my-key",
			Tags:      map[string]string{"env": "prod"},
		}
		Expect(in.Name).To(Equal("/prod/secret"))
		Expect(in.Overwrite).To(BeTrue())
		Expect(in.KMSKeyID).To(Equal("alias/my-key"))
	})

	It("round-trips PutParameterOutput", func() {
		out := PutParameterOutput{Version: 5}
		Expect(out.Version).To(Equal(int64(5)))
	})
})

var _ = Describe("Label input/output types", func() {
	It("round-trips LabelParameterInput", func() {
		in := LabelParameterInput{Name: "/p", Version: 3, Labels: []string{"prod"}}
		Expect(in.Version).To(Equal(int64(3)))
		Expect(in.Labels).To(Equal([]string{"prod"}))
	})

	It("round-trips LabelParameterOutput including invalid labels", func() {
		out := LabelParameterOutput{InvalidLabels: []string{"aws:bad"}, Version: 3}
		Expect(out.InvalidLabels).To(ConsistOf("aws:bad"))
	})

	It("round-trips UnlabelParameterInput", func() {
		un := UnlabelParameterInput{Name: "/p", Version: 3, Labels: []string{"old"}}
		Expect(un.Labels[0]).To(Equal("old"))
	})
})
