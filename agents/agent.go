package agents

type Agent interface {
	Start()
	Stop() error
}
