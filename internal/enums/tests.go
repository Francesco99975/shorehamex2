package enums

import "strings"

type test_domain string

const (
	depression  test_domain = "DEPRESSION"
	anxiety     test_domain = "ANXIETY"
	somatic     test_domain = "SOMATIC"
	personality test_domain = "PERSONALITY"
)

type TestDomain test_domain

type TestDomainDef struct {
	DEPRESSION  TestDomain
	ANXIETY     TestDomain
	SOMATIC     TestDomain
	PERSONALITY TestDomain
}

var TestDomains = &TestDomainDef{
	DEPRESSION:  TestDomain(depression),
	ANXIETY:     TestDomain(anxiety),
	SOMATIC:     TestDomain(somatic),
	PERSONALITY: TestDomain(personality),
}

func (r TestDomain) String() string {
	return string(r)
}

func GetTestDomainFromString(TestDomain string) TestDomain {
	switch strings.ToUpper(TestDomain) {
	case string(depression):
		return TestDomains.DEPRESSION
	case string(anxiety):
		return TestDomains.ANXIETY
	case string(somatic):
		return TestDomains.SOMATIC
	case string(personality):
		return TestDomains.PERSONALITY
	default:
		return TestDomains.DEPRESSION
	}
}

func IsTestDomainValid(TestDomain string) bool {
	switch strings.ToUpper(TestDomain) {
	case string(depression):
		return true
	case string(anxiety):
		return true
	case string(somatic):
		return true
	case string(personality):
		return true
	default:
		return false
	}
}
