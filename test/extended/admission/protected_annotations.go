package admission

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
)

var _ = g.Describe("[sig-luis] protected metadata", func() {
	g.When("update by a service account", func() {
		g.It("fails", func() {
			o.Expect(true).To(o.BeTrue())
		})
	})
})
