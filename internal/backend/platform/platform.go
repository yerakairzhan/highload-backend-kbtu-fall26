package platform

const apiPrefix = "/api/v2/"

type Platform interface {
	Start() error
	Stop() error
	IsRunning() bool
}
