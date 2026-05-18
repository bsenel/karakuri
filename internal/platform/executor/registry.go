package executor

func NewRegistry(name string) Executor {
	switch name {
	case "celery":
		return NewCeleryExecutor()
	case "restate":
		return NewRestateExecutor()
	default:
		return NewLocalExecutor()
	}
}
