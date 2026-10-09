package enums

import "strings"

type test_code string

const (
	asq    test_code = "ASQ"
	bai    test_code = "BAI"
	bdi    test_code = "BDI"
	p3     test_code = "P3"
	dass42 test_code = "DASS-42"
	bdi2   test_code = "BDI-II"
	ptsd   test_code = "PCL-C"
	mmpi2  test_code = "MMPI-2"
)

type TestCode test_code

func (r TestCode) String() string {
	return string(r)
}

type TestCodeRef struct {
	ASQ    TestCode
	BAI    TestCode
	BDI    TestCode
	P3     TestCode
	DASS42 TestCode
	BDI2   TestCode
	PSTD   TestCode
	MMPI2  TestCode
}

var TestCodes = &TestCodeRef{
	ASQ:    TestCode(asq),
	BAI:    TestCode(bai),
	BDI:    TestCode(bdi),
	P3:     TestCode(p3),
	DASS42: TestCode(dass42),
	BDI2:   TestCode(bdi2),
	PSTD:   TestCode(ptsd),
	MMPI2:  TestCode(mmpi2),
}

func GetTestCodeFromString(code string) TestCode {
	switch strings.ToUpper(code) {
	case string(asq):
		return TestCodes.ASQ
	case string(bai):
		return TestCodes.BAI
	case string(bdi):
		return TestCodes.BDI
	case string(p3):
		return TestCodes.P3
	case string(dass42):
		return TestCodes.DASS42
	case string(bdi2):
		return TestCodes.BDI2
	case string(ptsd):
		return TestCodes.PSTD
	case string(mmpi2):
		return TestCodes.MMPI2
	default:
		return ""
	}
}

func isTestCodeValid(code string) bool {
	switch strings.ToUpper(code) {
	case string(asq), string(bai), string(bdi), string(p3), string(dass42), string(bdi2), string(ptsd), string(mmpi2):
		return true
	default:
		return false
	}
}

type test_domain string

const (
	depression  test_domain = "DEPRESSION"
	anxiety     test_domain = "ANXIETY"
	somatic     test_domain = "SOMATIC"
	personality test_domain = "PERSONALITY"
	stress      test_domain = "STRESS"
	trauma      test_domain = "TRAUMA"
	multi       test_domain = "MULTI"
)

type TestDomain test_domain

type TestDomainDef struct {
	DEPRESSION  TestDomain
	ANXIETY     TestDomain
	SOMATIC     TestDomain
	PERSONALITY TestDomain
	STRESS      TestDomain
	TRAUMA      TestDomain
	MULTI       TestDomain
}

var TestDomains = &TestDomainDef{
	DEPRESSION:  TestDomain(depression),
	ANXIETY:     TestDomain(anxiety),
	SOMATIC:     TestDomain(somatic),
	PERSONALITY: TestDomain(personality),
	STRESS:      TestDomain(stress),
	TRAUMA:      TestDomain(trauma),
	MULTI:       TestDomain(multi),
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
	case string(stress):
		return TestDomains.STRESS
	case string(trauma):
		return TestDomains.TRAUMA
	case string(multi):
		return TestDomains.MULTI
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
	case string(stress):
		return true
	case string(trauma):
		return true
	case string(multi):
		return true
	default:
		return false
	}
}
