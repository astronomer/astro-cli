package sql_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestFlowCommandTable(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "flow Suite")
}
