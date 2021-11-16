package service

type Service struct {
	PeerListBackup bool
	SnListBackup bool
}

func InitService() *Service {
	return &Service{
		PeerListBackup: false,
		SnListBackup: false,
	}
}
