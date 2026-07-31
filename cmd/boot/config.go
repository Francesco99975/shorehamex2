package boot

import (
	"fmt"
	"net"
	"os"

	"regexp"

	"github.com/Francesco99975/shorehamex2/internal/enums"
)

func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		panic(err)
	}
	defer func() {
		err := conn.Close()
		if err != nil {
			panic(err)
		}
	}()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

var dsnRegex = regexp.MustCompile(`^postgresql:\/\/([a-zA-Z0-9._%+-]+):([^@]+)@([a-zA-Z0-9.-]+):(\d+)\/([a-zA-Z0-9._-]+)\?sslmode=(disable|require|verify-ca|verify-full)$`)

func isValidDSN(dsn string) bool {
	return dsnRegex.MatchString(dsn)
}

type Config struct {
	DevEmail     string
	Port         string
	Host         string
	GoEnv        enums.Environment
	PostgresUser string

	DSN string

	NTFY             string
	NTFYToken        string
	ResendApiKey     string
	URL              string
	MetricSecret     string
	Prometheus       string
	TwofaKey         string
	TwofaKeyOld      string
	PaginationWindow int
}

var Environment = &Config{}

func LoadEnvVariables() error {

	if !enums.IsEnvironmentValid(os.Getenv("GO_ENV")) {
		return fmt.Errorf("invalid environment variable: %s", os.Getenv("GO_ENV"))
	}

	Environment.DevEmail = os.Getenv("DEV_EMAIL")
	Environment.Port = os.Getenv("PORT")
	Environment.Host = os.Getenv("HOST")
	Environment.GoEnv = enums.GetEnvironmentFromString(os.Getenv("GO_ENV"))

	Environment.DSN = os.Getenv("DSN")
	if !isValidDSN(Environment.DSN) {
		return fmt.Errorf("invalid DSN: %s", Environment.DSN)
	}

	Environment.NTFY = os.Getenv("NTFY")
	Environment.NTFYToken = os.Getenv("NTFY_TOKEN")
	Environment.ResendApiKey = os.Getenv("RESEND_API_KEY")
	Environment.MetricSecret = os.Getenv("METRIC_SECRET")
	Environment.Prometheus = os.Getenv("PROMETHEUS")
	Environment.PostgresUser = os.Getenv("POSTGRES_USER")
	if Environment.GoEnv == enums.Environments.DEVELOPMENT {
		localIP := getLocalIP()
		Environment.URL = fmt.Sprintf("http://%s:%s", localIP, Environment.Port)
	} else {
		Environment.URL = fmt.Sprintf("https://%s", Environment.Host)
	}

	Environment.TwofaKey = os.Getenv("TWOFA_KEY")
	Environment.TwofaKeyOld = os.Getenv("TWOFA_KEY_OLD")
	Environment.PaginationWindow = 10

	return nil
}
