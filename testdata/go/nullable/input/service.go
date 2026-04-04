package service

type Config struct {
	Host    string
	Port    int
	Timeout *int
}

func NewService(cfg *Config, name string) (*Service, error) {
	return nil, nil
}

func Close(s *Service) error {
	return nil
}
