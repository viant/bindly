package types

type Option func(p *Type)

func WithName(name string) Option {
	return func(p *Type) {
		p.Name = name
	}
}

func WithPackage(pkg string) Option {
	return func(p *Type) {
		p.Package = pkg
	}
}

func WithEmbedder(e Embedder) Option {
	return func(p *Type) {
		p.Embedder = e
	}
}
