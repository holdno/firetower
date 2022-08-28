package store 

type ClusterConnStore interface {
	OneClientInc(clientIP string) error 
	OneClientDec(clientIP string) error
	GetAllConnNum() (uint64, error)
	RemoveClient(clientIP string) error
}