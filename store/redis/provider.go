package nosql

var provider *Provider

type Provider struct {
}

func MustSetup() *Provider {
	provider := &Provider{}
	return provider
}

func (s *Provider) SetConnect() {
	
}
