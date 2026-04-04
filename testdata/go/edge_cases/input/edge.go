package edge

type Empty struct{}

func init() {
	// init function
}

func NoParams() string {
	return ""
}

func VariadicFunc(prefix string, items ...int) {
}

func InterfaceParam(w interface{}) error {
	return nil
}
