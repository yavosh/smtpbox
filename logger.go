package smtpbox

// Logger is an abstract logger to support multiple logging frameworks
type Logger interface {
	Print(v ...interface{})
	Printf(format string, v ...interface{})
	Fatalf(format string, v ...interface{})
}
